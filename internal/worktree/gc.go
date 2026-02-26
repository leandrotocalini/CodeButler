package worktree

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ThreadChecker checks Slack thread activity.
type ThreadChecker interface {
	// LastActivity returns the timestamp of the last message in the thread.
	// Returns zero time if the thread has no messages or doesn't exist.
	LastActivity(ctx context.Context, channelID, threadTS string) (time.Time, error)
	// IsThreadActive returns true if the thread exists and has recent activity.
	IsThreadActive(ctx context.Context, channelID, threadTS string) (bool, error)
}

// PRChecker checks if a PR exists for a branch.
type PRChecker interface {
	// HasOpenPR returns true if an open PR exists for the given head branch.
	HasOpenPR(ctx context.Context, head string) (bool, error)
}

// ThreadPhase represents the current phase of a thread.
type ThreadPhase string

const (
	PhaseUnknown  ThreadPhase = ""
	PhasePlanning ThreadPhase = "planning"
	PhaseCoding   ThreadPhase = "coder"
	PhaseReview   ThreadPhase = "review"
	PhaseDone     ThreadPhase = "done"
)

// PhaseChecker checks the current phase of a thread.
type PhaseChecker interface {
	// GetPhase returns the current phase of the thread.
	GetPhase(ctx context.Context, threadID string) (ThreadPhase, error)
}

// GCNotifier sends GC-related notifications to Slack threads.
type GCNotifier interface {
	// WarnInactive posts a warning message in the thread about pending cleanup.
	WarnInactive(ctx context.Context, channelID, threadTS string) error
}

// WorktreeMapping maps a worktree branch to its Slack thread.
type WorktreeMapping struct {
	Branch    string
	ChannelID string
	ThreadTS  string
}

// MappingStore loads and persists worktree-to-thread mappings.
type MappingStore interface {
	// ListMappings returns all known worktree-to-thread mappings.
	ListMappings(ctx context.Context) ([]WorktreeMapping, error)
	// RemoveMapping removes a mapping for a branch.
	RemoveMapping(ctx context.Context, branch string) error
}

// GCConfig holds garbage collection configuration.
type GCConfig struct {
	// Interval between GC runs. Default: 6 hours.
	Interval time.Duration
	// InactivityTimeout is how long a thread must be idle before it's orphaned.
	// Default: 48 hours.
	InactivityTimeout time.Duration
	// GracePeriod is how long to wait after warning before cleaning up.
	// Default: 24 hours.
	GracePeriod time.Duration
}

// DefaultGCConfig returns the default GC configuration.
func DefaultGCConfig() GCConfig {
	return GCConfig{
		Interval:          6 * time.Hour,
		InactivityTimeout: 48 * time.Hour,
		GracePeriod:       24 * time.Hour,
	}
}

// GCState tracks warned worktrees so we can enforce the grace period.
type GCState struct {
	// WarnedAt tracks when each branch was warned. Key is branch name.
	WarnedAt map[string]time.Time
}

// GarbageCollector detects and cleans up orphaned worktrees.
type GarbageCollector struct {
	manager  *Manager
	threads  ThreadChecker
	prs      PRChecker
	phases   PhaseChecker
	notifier GCNotifier
	mappings MappingStore
	config   GCConfig
	logger   *slog.Logger
	now      func() time.Time // injectable clock for testing

	mu    sync.Mutex
	state GCState
}

// GCOption configures the garbage collector.
type GCOption func(*GarbageCollector)

// WithGCLogger sets the logger.
func WithGCLogger(l *slog.Logger) GCOption {
	return func(gc *GarbageCollector) {
		gc.logger = l
	}
}

// WithGCConfig sets the GC configuration.
func WithGCConfig(cfg GCConfig) GCOption {
	return func(gc *GarbageCollector) {
		gc.config = cfg
	}
}

// WithGCClock sets a custom clock (for testing).
func WithGCClock(now func() time.Time) GCOption {
	return func(gc *GarbageCollector) {
		gc.now = now
	}
}

// NewGarbageCollector creates a new garbage collector.
func NewGarbageCollector(
	manager *Manager,
	threads ThreadChecker,
	prs PRChecker,
	phases PhaseChecker,
	notifier GCNotifier,
	mappings MappingStore,
	opts ...GCOption,
) *GarbageCollector {
	gc := &GarbageCollector{
		manager:  manager,
		threads:  threads,
		prs:      prs,
		phases:   phases,
		notifier: notifier,
		mappings: mappings,
		config:   DefaultGCConfig(),
		logger:   slog.Default(),
		now:      time.Now,
		state:    GCState{WarnedAt: make(map[string]time.Time)},
	}
	for _, opt := range opts {
		opt(gc)
	}
	return gc
}

// RunOnce performs a single GC pass: detect orphans, warn or clean.
func (gc *GarbageCollector) RunOnce(ctx context.Context) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	mappings, err := gc.mappings.ListMappings(ctx)
	if err != nil {
		return fmt.Errorf("list mappings: %w", err)
	}

	worktrees, err := gc.manager.List(ctx)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}

	// Build set of existing worktree branches for quick lookup
	wtSet := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		wtSet[wt.Branch] = true
	}

	now := gc.now()

	for _, m := range mappings {
		// Skip if worktree doesn't exist locally
		if !wtSet[m.Branch] {
			gc.logger.Info("mapping has no local worktree, cleaning mapping", "branch", m.Branch)
			gc.mappings.RemoveMapping(ctx, m.Branch)
			delete(gc.state.WarnedAt, m.Branch)
			continue
		}

		orphaned, err := gc.isOrphaned(ctx, m, now)
		if err != nil {
			gc.logger.Warn("error checking orphan status", "branch", m.Branch, "err", err)
			continue
		}

		if !orphaned {
			// Reset warning if thread became active again
			delete(gc.state.WarnedAt, m.Branch)
			continue
		}

		// Check if we already warned
		warnedAt, warned := gc.state.WarnedAt[m.Branch]
		if !warned {
			// First detection — warn and record
			gc.logger.Info("orphan detected, warning", "branch", m.Branch)
			if err := gc.notifier.WarnInactive(ctx, m.ChannelID, m.ThreadTS); err != nil {
				gc.logger.Warn("failed to warn thread", "branch", m.Branch, "err", err)
			}
			gc.state.WarnedAt[m.Branch] = now
			continue
		}

		// Check if grace period has elapsed
		if now.Sub(warnedAt) < gc.config.GracePeriod {
			gc.logger.Info("orphan in grace period", "branch", m.Branch,
				"warned_at", warnedAt, "remaining", gc.config.GracePeriod-now.Sub(warnedAt))
			continue
		}

		// Grace period elapsed — clean up
		gc.logger.Info("cleaning orphan worktree", "branch", m.Branch)
		if err := gc.manager.Remove(ctx, m.Branch, true); err != nil {
			gc.logger.Warn("failed to remove worktree", "branch", m.Branch, "err", err)
		}
		if err := gc.mappings.RemoveMapping(ctx, m.Branch); err != nil {
			gc.logger.Warn("failed to remove mapping", "branch", m.Branch, "err", err)
		}
		delete(gc.state.WarnedAt, m.Branch)
	}

	return nil
}

// isOrphaned checks if a worktree is orphaned based on the three criteria:
// 1. No activity for > inactivityTimeout
// 2. Thread is not in coder phase
// 3. No open PR for the branch
func (gc *GarbageCollector) isOrphaned(ctx context.Context, m WorktreeMapping, now time.Time) (bool, error) {
	// Check thread activity
	lastActivity, err := gc.threads.LastActivity(ctx, m.ChannelID, m.ThreadTS)
	if err != nil {
		return false, fmt.Errorf("check last activity: %w", err)
	}

	if !lastActivity.IsZero() && now.Sub(lastActivity) < gc.config.InactivityTimeout {
		return false, nil // Still active
	}

	// Check thread phase
	phase, err := gc.phases.GetPhase(ctx, m.ThreadTS)
	if err != nil {
		return false, fmt.Errorf("check phase: %w", err)
	}
	if phase == PhaseCoding {
		return false, nil // Don't GC during coding
	}

	// Check PR status
	hasOpenPR, err := gc.prs.HasOpenPR(ctx, m.Branch)
	if err != nil {
		return false, fmt.Errorf("check PR: %w", err)
	}
	if hasOpenPR {
		return false, nil // Active PR exists
	}

	return true, nil
}

// Run starts the periodic GC loop. Blocks until context is cancelled.
func (gc *GarbageCollector) Run(ctx context.Context) error {
	// Run once immediately
	if err := gc.RunOnce(ctx); err != nil {
		gc.logger.Warn("initial GC run failed", "err", err)
	}

	ticker := time.NewTicker(gc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := gc.RunOnce(ctx); err != nil {
				gc.logger.Warn("GC run failed", "err", err)
			}
		}
	}
}

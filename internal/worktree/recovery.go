package worktree

import (
	"context"
	"fmt"
	"log/slog"
)

// RecoveryHandler reconciles local worktrees with Slack threads on startup.
type RecoveryHandler struct {
	manager  *Manager
	threads  ThreadChecker
	mappings MappingStore
	logger   *slog.Logger
}

// RecoveryOption configures the recovery handler.
type RecoveryOption func(*RecoveryHandler)

// WithRecoveryLogger sets the logger.
func WithRecoveryLogger(l *slog.Logger) RecoveryOption {
	return func(r *RecoveryHandler) {
		r.logger = l
	}
}

// NewRecoveryHandler creates a new recovery handler.
func NewRecoveryHandler(
	manager *Manager,
	threads ThreadChecker,
	mappings MappingStore,
	opts ...RecoveryOption,
) *RecoveryHandler {
	r := &RecoveryHandler{
		manager:  manager,
		threads:  threads,
		mappings: mappings,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// RecoveryResult holds the outcome of a recovery run.
type RecoveryResult struct {
	// WorktreesFound is the number of local worktrees found.
	WorktreesFound int
	// MappingsFound is the number of known mappings.
	MappingsFound int
	// CleanedUp is the number of worktrees cleaned up (thread gone).
	CleanedUp int
	// Orphaned is the number of worktrees with no mapping (left for GC).
	Orphaned int
}

// Reconcile checks all local worktrees against Slack threads.
// - If the thread is gone (deleted, archived) → clean up immediately.
// - If the thread exists but no pending work → leave for GC.
// - Worktrees with no mapping are logged as orphaned (left for GC).
func (r *RecoveryHandler) Reconcile(ctx context.Context) (*RecoveryResult, error) {
	worktrees, err := r.manager.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	mappings, err := r.mappings.ListMappings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list mappings: %w", err)
	}

	// Build mapping lookup by branch
	mappingByBranch := make(map[string]WorktreeMapping, len(mappings))
	for _, m := range mappings {
		mappingByBranch[m.Branch] = m
	}

	result := &RecoveryResult{
		WorktreesFound: len(worktrees),
		MappingsFound:  len(mappings),
	}

	for _, wt := range worktrees {
		m, hasMappings := mappingByBranch[wt.Branch]
		if !hasMappings {
			r.logger.Warn("worktree has no mapping, leaving for GC", "branch", wt.Branch)
			result.Orphaned++
			continue
		}

		// Check if thread still exists
		active, err := r.threads.IsThreadActive(ctx, m.ChannelID, m.ThreadTS)
		if err != nil {
			r.logger.Warn("error checking thread", "branch", wt.Branch, "err", err)
			continue
		}

		if !active {
			// Thread is gone — clean up immediately
			r.logger.Info("thread gone, cleaning worktree", "branch", wt.Branch,
				"channel", m.ChannelID, "thread", m.ThreadTS)
			if err := r.manager.Remove(ctx, wt.Branch, true); err != nil {
				r.logger.Warn("failed to remove worktree", "branch", wt.Branch, "err", err)
				continue
			}
			if err := r.mappings.RemoveMapping(ctx, wt.Branch); err != nil {
				r.logger.Warn("failed to remove mapping", "branch", wt.Branch, "err", err)
			}
			result.CleanedUp++
		}
	}

	r.logger.Info("recovery complete",
		"worktrees", result.WorktreesFound,
		"mappings", result.MappingsFound,
		"cleaned", result.CleanedUp,
		"orphaned", result.Orphaned,
	)

	return result, nil
}

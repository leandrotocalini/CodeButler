// Package lifecycle manages graceful shutdown and crash recovery for
// CodeButler agent processes. It handles signal interception, context
// cancellation, goroutine wind-down, and work resumption after restart.
package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ShutdownConfig configures the shutdown behavior.
type ShutdownConfig struct {
	GracePeriod  time.Duration // time to wait for goroutines before force exit
	ForceTimeout time.Duration // max time before os.Exit regardless
}

// DefaultShutdownConfig returns sensible defaults.
func DefaultShutdownConfig() ShutdownConfig {
	return ShutdownConfig{
		GracePeriod:  10 * time.Second,
		ForceTimeout: 15 * time.Second,
	}
}

// Manager coordinates shutdown and recovery for an agent process.
type Manager struct {
	config   ShutdownConfig
	logger   *slog.Logger
	cancel   context.CancelFunc
	mu       sync.Mutex
	hooks    []ShutdownHook
	started  time.Time
	shutdown bool
}

// ShutdownHook is called during graceful shutdown. Name is for logging.
type ShutdownHook struct {
	Name string
	Fn   func(ctx context.Context) error
}

// NewManager creates a lifecycle manager.
func NewManager(config ShutdownConfig, logger *slog.Logger) *Manager {
	return &Manager{
		config:  config,
		logger:  logger,
		started: time.Now(),
	}
}

// OnShutdown registers a hook to run during graceful shutdown.
// Hooks run in registration order.
func (m *Manager) OnShutdown(name string, fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, ShutdownHook{Name: name, Fn: fn})
}

// Run starts the agent lifecycle: installs signal handlers, runs the main
// function, and handles shutdown. Returns exit code.
func (m *Manager) Run(mainFn func(ctx context.Context) error) int {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Install signal handlers
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Run main function
	errCh := make(chan error, 1)
	go func() {
		errCh <- mainFn(ctx)
	}()

	// Wait for signal or main completion
	select {
	case sig := <-sigCh:
		m.logger.Info("received signal, starting graceful shutdown",
			"signal", sig.String(),
			"uptime", time.Since(m.started).String(),
		)
		return m.gracefulShutdown()

	case err := <-errCh:
		if err != nil {
			m.logger.Error("main function error", "error", err)
			m.runHooksQuick()
			return 1
		}
		m.runHooksQuick()
		return 0
	}
}

// gracefulShutdown cancels the root context and runs hooks with a deadline.
func (m *Manager) gracefulShutdown() int {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return 1
	}
	m.shutdown = true
	hooks := make([]ShutdownHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	// Cancel root context — all goroutines should start winding down
	m.cancel()

	// Run hooks with grace period deadline
	ctx, cancel := context.WithTimeout(context.Background(), m.config.GracePeriod)
	defer cancel()

	for _, hook := range hooks {
		m.logger.Info("running shutdown hook", "name", hook.Name)
		if err := hook.Fn(ctx); err != nil {
			m.logger.Error("shutdown hook failed", "name", hook.Name, "error", err)
		}
	}

	m.logger.Info("graceful shutdown complete",
		"uptime", time.Since(m.started).String(),
	)
	return 0
}

// runHooksQuick runs hooks with a short timeout (for normal exit).
func (m *Manager) runHooksQuick() {
	m.mu.Lock()
	hooks := make([]ShutdownHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, hook := range hooks {
		hook.Fn(ctx)
	}
}

// Uptime returns how long the process has been running.
func (m *Manager) Uptime() time.Duration {
	return time.Since(m.started)
}

// --- Recovery ---

// RecoveryState represents the state of work that was in progress when
// the agent crashed or was stopped.
type RecoveryState struct {
	Role          string       `json:"role"`
	ActiveThreads []ThreadInfo `json:"active_threads"`
	PendingWork   []PendingItem `json:"pending_work"`
	Timestamp     time.Time    `json:"timestamp"`
}

// ThreadInfo describes an active thread that needs to be resumed.
type ThreadInfo struct {
	ThreadID        string `json:"thread_id"`
	Channel         string `json:"channel"`
	Branch          string `json:"branch"`
	HasConversation bool   `json:"has_conversation"` // true if conversation JSON exists
	LastActivity    string `json:"last_activity"`     // ISO timestamp
}

// PendingItem describes work that was not completed before shutdown.
type PendingItem struct {
	Type      string `json:"type"`       // "mention", "thread", "task"
	ThreadID  string `json:"thread_id"`
	Channel   string `json:"channel"`
	MessageTS string `json:"message_ts"` // Slack message timestamp
	Text      string `json:"text"`       // preview text
}

// WorktreeReconciler compares local worktrees with known thread state.
type WorktreeReconciler interface {
	// ListWorktrees returns all active worktrees.
	ListWorktrees(ctx context.Context) ([]string, error)
	// ThreadExists checks if a Slack thread is still active.
	ThreadExists(ctx context.Context, channel, threadID string) (bool, error)
}

// ConversationLoader loads conversation state for recovery.
type ConversationLoader interface {
	// HasConversation checks if a conversation file exists for a thread/role.
	HasConversation(threadID, role string) bool
}

// MentionScanner finds unresponded @mentions in thread history.
type MentionScanner interface {
	// ScanUnresponded returns @mentions of this role that were not replied to.
	ScanUnresponded(ctx context.Context, channel, threadID, role string) ([]PendingItem, error)
}

// RecoverAgent builds a recovery state for the agent to resume from.
func RecoverAgent(
	ctx context.Context,
	role string,
	worktrees []string,
	convLoader ConversationLoader,
) *RecoveryState {
	state := &RecoveryState{
		Role:      role,
		Timestamp: time.Now(),
	}

	for _, wt := range worktrees {
		ti := ThreadInfo{
			Branch:          wt,
			HasConversation: convLoader.HasConversation(wt, role),
		}
		state.ActiveThreads = append(state.ActiveThreads, ti)
	}

	return state
}

// FormatRecoveryReport creates a human-readable recovery report.
func FormatRecoveryReport(state *RecoveryState) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Recovery Report — %s\n\n", state.Role))
	b.WriteString(fmt.Sprintf("**Recovered at:** %s\n\n", state.Timestamp.Format(time.RFC3339)))

	if len(state.ActiveThreads) > 0 {
		b.WriteString("### Active Threads\n\n")
		for _, t := range state.ActiveThreads {
			conv := "no"
			if t.HasConversation {
				conv = "yes"
			}
			b.WriteString(fmt.Sprintf("- Branch: %s (conversation: %s)\n", t.Branch, conv))
		}
		b.WriteString("\n")
	}

	if len(state.PendingWork) > 0 {
		b.WriteString("### Pending Work\n\n")
		for _, p := range state.PendingWork {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", p.Type, p.Text))
		}
		b.WriteString("\n")
	}

	if len(state.ActiveThreads) == 0 && len(state.PendingWork) == 0 {
		b.WriteString("No pending work found.\n")
	}

	return b.String()
}

package roadmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ItemWorker executes a single roadmap item.
// Returns the branch name on success, or an error if blocked.
type ItemWorker func(ctx context.Context, item Item) (branch string, err error)

// StatusReporter posts progress updates to the orchestration thread.
type StatusReporter func(ctx context.Context, message string) error

// Orchestrator manages unattended execution of all roadmap items.
type Orchestrator struct {
	roadmap  *Roadmap
	worker   ItemWorker
	reporter StatusReporter
	logger   *slog.Logger

	maxConcurrent int

	mu       sync.Mutex
	active   map[int]bool    // items currently executing
	results  map[int]string  // item number → branch (completed)
	errors   map[int]string  // item number → error message (failed)
}

// OrchestratorConfig configures the orchestrator.
type OrchestratorConfig struct {
	MaxConcurrent int
	Logger        *slog.Logger
}

// NewOrchestrator creates an orchestrator for the given roadmap.
func NewOrchestrator(r *Roadmap, worker ItemWorker, reporter StatusReporter, cfg OrchestratorConfig) *Orchestrator {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 3
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Orchestrator{
		roadmap:       r,
		worker:        worker,
		reporter:      reporter,
		logger:        cfg.Logger,
		maxConcurrent: cfg.MaxConcurrent,
		active:        make(map[int]bool),
		results:       make(map[int]string),
		errors:        make(map[int]string),
	}
}

// Run executes all pending roadmap items respecting dependencies.
// It launches unblocked items in parallel (up to maxConcurrent),
// and cascades when items complete.
func (o *Orchestrator) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Find items ready to launch
		g := BuildGraph(o.roadmap)
		ready := o.findReady(g)

		if len(ready) == 0 {
			// Check if we're waiting for active items
			o.mu.Lock()
			activeCount := len(o.active)
			o.mu.Unlock()

			if activeCount == 0 {
				// Nothing active, nothing ready — we're done
				break
			}

			// Wait for active items to finish
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Launch ready items (respect concurrency limit)
		o.mu.Lock()
		slots := o.maxConcurrent - len(o.active)
		o.mu.Unlock()

		if slots <= 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		launched := 0
		for _, item := range ready {
			if launched >= slots {
				break
			}

			o.mu.Lock()
			if o.active[item.Number] {
				o.mu.Unlock()
				continue
			}
			o.active[item.Number] = true
			o.mu.Unlock()

			o.roadmap.SetStatus(item.Number, StatusInProgress)
			launched++

			go o.executeItem(ctx, item)
		}

		if launched > 0 {
			o.reportProgress(ctx)
		}

		time.Sleep(100 * time.Millisecond)
	}

	o.reportProgress(ctx)
	return nil
}

// findReady returns pending items with all deps satisfied that are not active.
func (o *Orchestrator) findReady(g *DependencyGraph) []Item {
	unblocked := g.Unblocked()

	o.mu.Lock()
	defer o.mu.Unlock()

	var ready []Item
	for _, item := range unblocked {
		if !o.active[item.Number] {
			ready = append(ready, item)
		}
	}
	return ready
}

// executeItem runs a single roadmap item via the worker.
func (o *Orchestrator) executeItem(ctx context.Context, item Item) {
	o.logger.Info("starting roadmap item",
		"item", item.Number,
		"title", item.Title,
	)

	branch, err := o.worker(ctx, item)

	o.mu.Lock()
	delete(o.active, item.Number)

	if err != nil {
		o.errors[item.Number] = err.Error()
		o.mu.Unlock()

		o.roadmap.SetStatus(item.Number, StatusBlocked)
		o.logger.Error("roadmap item failed",
			"item", item.Number,
			"title", item.Title,
			"error", err,
		)
		o.reportProgress(ctx)
		return
	}

	o.results[item.Number] = branch
	o.mu.Unlock()

	o.roadmap.SetStatus(item.Number, StatusDone)
	if branch != "" {
		o.roadmap.SetBranch(item.Number, branch)
	}

	o.logger.Info("roadmap item completed",
		"item", item.Number,
		"title", item.Title,
		"branch", branch,
	)

	o.reportProgress(ctx)
}

// reportProgress posts a status update to the orchestration thread.
func (o *Orchestrator) reportProgress(ctx context.Context) {
	if o.reporter == nil {
		return
	}

	msg := o.FormatProgress()
	if err := o.reporter(ctx, msg); err != nil {
		o.logger.Warn("failed to report progress", "error", err)
	}
}

// FormatProgress returns a formatted progress message.
func (o *Orchestrator) FormatProgress() string {
	var b strings.Builder
	b.WriteString("Roadmap progress:\n")

	for _, item := range o.roadmap.Items {
		var icon string
		switch item.Status {
		case StatusDone:
			icon = "done"
		case StatusInProgress:
			icon = "in_progress"
		case StatusBlocked:
			icon = "blocked"
		case StatusPending:
			icon = "pending"
		}

		line := fmt.Sprintf("%s %d. %s — %s", icon, item.Number, item.Title, item.Status)
		if item.Branch != "" {
			line += fmt.Sprintf(" (%s)", item.Branch)
		}

		o.mu.Lock()
		if errMsg, ok := o.errors[item.Number]; ok {
			line += fmt.Sprintf(" [error: %s]", errMsg)
		}
		o.mu.Unlock()

		b.WriteString(line + "\n")
	}

	g := BuildGraph(o.roadmap)
	stats := g.Stats()
	b.WriteString("\n" + stats.FormatProgress())

	return b.String()
}

// CompletedCount returns how many items have completed.
func (o *Orchestrator) CompletedCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.results)
}

// FailedCount returns how many items have failed.
func (o *Orchestrator) FailedCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.errors)
}

// ActiveCount returns how many items are currently executing.
func (o *Orchestrator) ActiveCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.active)
}

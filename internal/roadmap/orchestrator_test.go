package roadmap

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrchestrator_SimpleExecution(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. First
- Status: pending
- Depends on: —
- Acceptance criteria: first thing

## 2. Second
- Status: pending
- Depends on: —
- Acceptance criteria: second thing
`)

	var completed atomic.Int32
	worker := func(ctx context.Context, item Item) (string, error) {
		completed.Add(1)
		return fmt.Sprintf("branch-%d", item.Number), nil
	}

	var messages []string
	var msgMu sync.Mutex
	reporter := func(ctx context.Context, msg string) error {
		msgMu.Lock()
		messages = append(messages, msg)
		msgMu.Unlock()
		return nil
	}

	orch := NewOrchestrator(r, worker, reporter, OrchestratorConfig{
		MaxConcurrent: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := orch.Run(ctx); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if completed.Load() != 2 {
		t.Errorf("expected 2 completed, got %d", completed.Load())
	}
	if orch.CompletedCount() != 2 {
		t.Errorf("expected 2 completed count, got %d", orch.CompletedCount())
	}
}

func TestOrchestrator_DependencyOrder(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. Base
- Status: pending
- Depends on: —
- Acceptance criteria: base

## 2. Depends on base
- Status: pending
- Depends on: 1
- Acceptance criteria: depends
`)

	var order []int
	var mu sync.Mutex
	worker := func(ctx context.Context, item Item) (string, error) {
		mu.Lock()
		order = append(order, item.Number)
		mu.Unlock()
		return "", nil
	}

	orch := NewOrchestrator(r, worker, nil, OrchestratorConfig{
		MaxConcurrent: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch.Run(ctx)

	if len(order) != 2 {
		t.Fatalf("expected 2 items executed, got %d", len(order))
	}
	if order[0] != 1 {
		t.Errorf("expected item 1 first, got %d", order[0])
	}
	if order[1] != 2 {
		t.Errorf("expected item 2 second, got %d", order[1])
	}
}

func TestOrchestrator_PartialFailure(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. Works
- Status: pending
- Depends on: —
- Acceptance criteria: ok

## 2. Fails
- Status: pending
- Depends on: —
- Acceptance criteria: nope

## 3. Depends on failed
- Status: pending
- Depends on: 2
- Acceptance criteria: blocked
`)

	worker := func(ctx context.Context, item Item) (string, error) {
		if item.Number == 2 {
			return "", fmt.Errorf("compilation failed")
		}
		return "ok-branch", nil
	}

	orch := NewOrchestrator(r, worker, nil, OrchestratorConfig{
		MaxConcurrent: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch.Run(ctx)

	if orch.CompletedCount() != 1 {
		t.Errorf("expected 1 completed, got %d", orch.CompletedCount())
	}
	if orch.FailedCount() != 1 {
		t.Errorf("expected 1 failed, got %d", orch.FailedCount())
	}

	// Item 2 should be blocked
	if r.GetItem(2).Status != StatusBlocked {
		t.Errorf("item 2 should be blocked, got %q", r.GetItem(2).Status)
	}
	// Item 3 should still be pending (dep not met)
	if r.GetItem(3).Status != StatusPending {
		t.Errorf("item 3 should be pending, got %q", r.GetItem(3).Status)
	}
}

func TestOrchestrator_ConcurrencyLimit(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. A
- Status: pending
- Depends on: —
- Acceptance criteria: a

## 2. B
- Status: pending
- Depends on: —
- Acceptance criteria: b

## 3. C
- Status: pending
- Depends on: —
- Acceptance criteria: c
`)

	var maxConcurrent atomic.Int32
	var current atomic.Int32

	worker := func(ctx context.Context, item Item) (string, error) {
		cur := current.Add(1)
		// Track max concurrent
		for {
			old := maxConcurrent.Load()
			if cur <= old {
				break
			}
			if maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		return "", nil
	}

	orch := NewOrchestrator(r, worker, nil, OrchestratorConfig{
		MaxConcurrent: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch.Run(ctx)

	if maxConcurrent.Load() > 2 {
		t.Errorf("max concurrent should be <= 2, got %d", maxConcurrent.Load())
	}
}

func TestOrchestrator_SkipsDone(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. Already done
- Status: done
- Depends on: —
- Acceptance criteria: done

## 2. Pending
- Status: pending
- Depends on: 1
- Acceptance criteria: pending
`)

	var executed []int
	var mu sync.Mutex
	worker := func(ctx context.Context, item Item) (string, error) {
		mu.Lock()
		executed = append(executed, item.Number)
		mu.Unlock()
		return "", nil
	}

	orch := NewOrchestrator(r, worker, nil, OrchestratorConfig{
		MaxConcurrent: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch.Run(ctx)

	// Only item 2 should execute (item 1 already done)
	if len(executed) != 1 {
		t.Fatalf("expected 1 executed, got %d: %v", len(executed), executed)
	}
	if executed[0] != 2 {
		t.Errorf("expected item 2, got %d", executed[0])
	}
}

func TestOrchestrator_FormatProgress(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. Done item
- Status: done
- Branch: branch-1
- Depends on: —
- Acceptance criteria: done

## 2. In progress
- Status: in_progress
- Depends on: —
- Acceptance criteria: wip

## 3. Pending
- Status: pending
- Depends on: 1
- Acceptance criteria: todo
`)

	orch := NewOrchestrator(r, nil, nil, OrchestratorConfig{})
	progress := orch.FormatProgress()

	if progress == "" {
		t.Error("expected non-empty progress")
	}

	// Check it mentions all items
	for _, item := range r.Items {
		if !containsString(progress, item.Title) {
			t.Errorf("progress should mention %q", item.Title)
		}
	}
}

func TestOrchestrator_EmptyRoadmap(t *testing.T) {
	r := &Roadmap{}

	orch := NewOrchestrator(r, nil, nil, OrchestratorConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := orch.Run(ctx); err != nil {
		t.Fatalf("empty roadmap should not error: %v", err)
	}
}

func TestOrchestrator_ContextCancellation(t *testing.T) {
	r, _ := ParseString(`# Roadmap: Test

## 1. Slow
- Status: pending
- Depends on: —
- Acceptance criteria: slow
`)

	worker := func(ctx context.Context, item Item) (string, error) {
		select {
		case <-time.After(10 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	orch := NewOrchestrator(r, worker, nil, OrchestratorConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

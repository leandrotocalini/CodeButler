package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestManager_RunNormal(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	code := m.Run(func(ctx context.Context) error {
		return nil
	})

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestManager_RunError(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	code := m.Run(func(ctx context.Context) error {
		return fmt.Errorf("something broke")
	})

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestManager_ShutdownHooksRun(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	var mu sync.Mutex
	var order []string

	m.OnShutdown("first", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, "first")
		mu.Unlock()
		return nil
	})
	m.OnShutdown("second", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, "second")
		mu.Unlock()
		return nil
	})

	m.Run(func(ctx context.Context) error {
		return nil
	})

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" {
		t.Errorf("hooks ran in wrong order: %v", order)
	}
}

func TestManager_ShutdownHookError(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	var secondRan bool
	m.OnShutdown("failing", func(ctx context.Context) error {
		return fmt.Errorf("hook failed")
	})
	m.OnShutdown("succeeding", func(ctx context.Context) error {
		secondRan = true
		return nil
	})

	// Normal exit still runs hooks
	m.Run(func(ctx context.Context) error {
		return nil
	})

	if !secondRan {
		t.Error("second hook should still run after first hook fails")
	}
}

func TestManager_ContextCancellation(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	var ctxCancelled bool
	code := m.Run(func(ctx context.Context) error {
		// Cancel immediately via context
		go func() {
			time.Sleep(10 * time.Millisecond)
			m.cancel()
		}()

		<-ctx.Done()
		ctxCancelled = true
		return ctx.Err()
	})

	if !ctxCancelled {
		t.Error("context should have been cancelled")
	}
	// Error exit code since main returned ctx.Err
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestManager_Uptime(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	time.Sleep(10 * time.Millisecond)
	uptime := m.Uptime()

	if uptime < 10*time.Millisecond {
		t.Errorf("uptime too short: %v", uptime)
	}
}

func TestDefaultShutdownConfig(t *testing.T) {
	cfg := DefaultShutdownConfig()

	if cfg.GracePeriod != 10*time.Second {
		t.Errorf("grace period: %v", cfg.GracePeriod)
	}
	if cfg.ForceTimeout != 15*time.Second {
		t.Errorf("force timeout: %v", cfg.ForceTimeout)
	}
}

// --- Recovery Tests ---

type mockConvLoader struct {
	existing map[string]bool // "branch:role" → exists
}

func (m *mockConvLoader) HasConversation(threadID, role string) bool {
	return m.existing[threadID+":"+role]
}

func TestRecoverAgent_WithWorktrees(t *testing.T) {
	loader := &mockConvLoader{
		existing: map[string]bool{
			"codebutler/add-login:coder":    true,
			"codebutler/fix-bug:coder":      false,
			"codebutler/add-login:reviewer": true,
		},
	}

	state := RecoverAgent(
		context.Background(),
		"coder",
		[]string{"codebutler/add-login", "codebutler/fix-bug"},
		loader,
	)

	if state.Role != "coder" {
		t.Errorf("role: got %s", state.Role)
	}
	if len(state.ActiveThreads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(state.ActiveThreads))
	}

	// First thread should have conversation
	if !state.ActiveThreads[0].HasConversation {
		t.Error("add-login should have conversation for coder")
	}
	// Second thread should not
	if state.ActiveThreads[1].HasConversation {
		t.Error("fix-bug should not have conversation for coder")
	}
}

func TestRecoverAgent_Empty(t *testing.T) {
	loader := &mockConvLoader{existing: map[string]bool{}}

	state := RecoverAgent(
		context.Background(),
		"pm",
		nil,
		loader,
	)

	if len(state.ActiveThreads) != 0 {
		t.Error("should have no active threads")
	}
}

func TestFormatRecoveryReport_WithWork(t *testing.T) {
	state := &RecoveryState{
		Role:      "coder",
		Timestamp: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
		ActiveThreads: []ThreadInfo{
			{Branch: "codebutler/add-login", HasConversation: true},
			{Branch: "codebutler/fix-bug", HasConversation: false},
		},
		PendingWork: []PendingItem{
			{Type: "mention", Text: "@codebutler.coder implement the login form"},
		},
	}

	report := FormatRecoveryReport(state)

	if !strings.Contains(report, "coder") {
		t.Error("should contain role")
	}
	if !strings.Contains(report, "codebutler/add-login") {
		t.Error("should contain branch")
	}
	if !strings.Contains(report, "Pending Work") {
		t.Error("should contain pending work section")
	}
	if !strings.Contains(report, "login form") {
		t.Error("should contain pending work text")
	}
}

func TestFormatRecoveryReport_Empty(t *testing.T) {
	state := &RecoveryState{
		Role:      "pm",
		Timestamp: time.Now(),
	}

	report := FormatRecoveryReport(state)
	if !strings.Contains(report, "No pending work") {
		t.Error("should indicate no pending work")
	}
}

func TestRecoveryState_Fields(t *testing.T) {
	state := &RecoveryState{
		Role: "reviewer",
		ActiveThreads: []ThreadInfo{
			{
				ThreadID:        "T123",
				Channel:         "C456",
				Branch:          "codebutler/review-1",
				HasConversation: true,
				LastActivity:    "2026-02-26T12:00:00Z",
			},
		},
		PendingWork: []PendingItem{
			{
				Type:      "mention",
				ThreadID:  "T123",
				Channel:   "C456",
				MessageTS: "1234567.890",
				Text:      "@codebutler.reviewer review this PR",
			},
		},
		Timestamp: time.Now(),
	}

	if state.ActiveThreads[0].ThreadID != "T123" {
		t.Error("thread ID should be preserved")
	}
	if state.PendingWork[0].MessageTS != "1234567.890" {
		t.Error("message TS should be preserved")
	}
}

func TestManager_GracefulShutdownIdempotent(t *testing.T) {
	m := NewManager(DefaultShutdownConfig(), testLogger())

	callCount := 0
	m.OnShutdown("counter", func(ctx context.Context) error {
		callCount++
		return nil
	})

	// Call gracefulShutdown directly twice
	m.cancel = func() {} // no-op cancel
	m.gracefulShutdown()
	m.gracefulShutdown()

	// Second call should be no-op because m.shutdown is already true
	if callCount != 1 {
		t.Errorf("hooks should run only once, ran %d times", callCount)
	}
}

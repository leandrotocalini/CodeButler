package budget

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedClock returns a fixed time for testing.
type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time { return c.now }

func TestTracker_Record(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, "")

	err := tr.Record("T1", "coder", "openai/gpt-4o", TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	})
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}

	cost := tr.ThreadCost("T1")
	if cost <= 0 {
		t.Error("expected positive cost")
	}

	daily := tr.DailyCost()
	if daily <= 0 {
		t.Error("expected positive daily cost")
	}
}

func TestTracker_ThreadBudgetExceeded(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerThreadUSD: 0.001}, "")

	// Record enough to exceed $0.001
	err := tr.Record("T1", "coder", "anthropic/claude-opus-4-6", TokenUsage{
		PromptTokens:     10000,
		CompletionTokens: 5000,
		TotalTokens:      15000,
	})

	if err == nil {
		t.Fatal("expected budget exceeded error")
	}

	be, ok := err.(*BudgetExceeded)
	if !ok {
		t.Fatalf("expected *BudgetExceeded, got %T", err)
	}
	if be.Scope != "thread" {
		t.Errorf("expected thread scope, got %s", be.Scope)
	}
	if be.ThreadID != "T1" {
		t.Errorf("expected thread T1, got %s", be.ThreadID)
	}

	// Thread should be paused
	_, paused := tr.CheckThread("T1")
	if !paused {
		t.Error("thread should be paused")
	}
}

func TestTracker_DailyBudgetExceeded(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerDayUSD: 0.001}, "")

	err := tr.Record("T1", "pm", "anthropic/claude-opus-4-6", TokenUsage{
		PromptTokens:     10000,
		CompletionTokens: 5000,
		TotalTokens:      15000,
	})

	if err == nil {
		t.Fatal("expected daily budget exceeded error")
	}

	be := err.(*BudgetExceeded)
	if be.Scope != "day" {
		t.Errorf("expected day scope, got %s", be.Scope)
	}

	_, exhausted := tr.CheckDaily()
	if !exhausted {
		t.Error("daily budget should be exhausted")
	}
}

func TestTracker_UnlimitedBudget(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, "") // no limits

	for i := 0; i < 100; i++ {
		err := tr.Record("T1", "coder", "anthropic/claude-opus-4-6", TokenUsage{
			PromptTokens:     100000,
			CompletionTokens: 50000,
			TotalTokens:      150000,
		})
		if err != nil {
			t.Fatalf("unlimited budget should not error: %v", err)
		}
	}
}

func TestTracker_ResumeThread(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerThreadUSD: 0.001}, "")

	// Exceed budget
	tr.Record("T1", "coder", "anthropic/claude-opus-4-6", TokenUsage{
		PromptTokens: 10000, CompletionTokens: 5000, TotalTokens: 15000,
	})

	_, paused := tr.CheckThread("T1")
	if !paused {
		t.Error("should be paused")
	}

	// Resume
	tr.ResumeThread("T1")

	_, paused = tr.CheckThread("T1")
	if paused {
		t.Error("should not be paused after resume")
	}
}

func TestTracker_CheckThread_NoRecord(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerThreadUSD: 10.0}, "")

	remaining, paused := tr.CheckThread("T_NEW")
	if paused {
		t.Error("new thread should not be paused")
	}
	if remaining != 10.0 {
		t.Errorf("expected 10.0 remaining, got %f", remaining)
	}
}

func TestTracker_CheckDaily_NoRecord(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerDayUSD: 50.0}, "")

	remaining, exhausted := tr.CheckDaily()
	if exhausted {
		t.Error("should not be exhausted")
	}
	if remaining != 50.0 {
		t.Errorf("expected 50.0 remaining, got %f", remaining)
	}
}

func TestTracker_MultipleThreads(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerThreadUSD: 1.0}, "")

	tr.Record("T1", "coder", "openai/gpt-4o-mini", TokenUsage{
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
	})
	tr.Record("T2", "pm", "openai/gpt-4o", TokenUsage{
		PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300,
	})

	c1 := tr.ThreadCost("T1")
	c2 := tr.ThreadCost("T2")

	if c1 >= c2 {
		t.Error("gpt-4o should cost more than gpt-4o-mini for same tokens")
	}

	// Both should be tracked in daily
	daily := tr.DailyCost()
	if daily != c1+c2 {
		t.Errorf("daily cost should be sum: %f != %f + %f", daily, c1, c2)
	}
}

func TestTracker_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker(BudgetConfig{PerThreadUSD: 5.0}, dir)

	tr.Record("T1", "coder", "openai/gpt-4o", TokenUsage{
		PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500,
	})

	if err := tr.Save("T1"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "budgets", "T1.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("budget file not created")
	}

	// Load into new tracker
	tr2 := NewTracker(BudgetConfig{PerThreadUSD: 5.0}, dir)
	if err := tr2.Load("T1"); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	cost := tr2.ThreadCost("T1")
	if cost != tr.ThreadCost("T1") {
		t.Errorf("loaded cost %f != original %f", cost, tr.ThreadCost("T1"))
	}
}

func TestTracker_LoadMissing(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, t.TempDir())

	err := tr.Load("NONEXISTENT")
	if err != nil {
		t.Errorf("load missing should not error: %v", err)
	}
}

func TestTracker_GetThreadBudget(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerThreadUSD: 10.0}, "")

	tr.Record("T1", "pm", "openai/gpt-4o", TokenUsage{
		PromptTokens: 500, CompletionTokens: 200, TotalTokens: 700,
	})
	tr.Record("T1", "coder", "openai/gpt-4o", TokenUsage{
		PromptTokens: 800, CompletionTokens: 400, TotalTokens: 1200,
	})

	tb := tr.GetThreadBudget("T1")
	if tb == nil {
		t.Fatal("expected thread budget")
	}
	if len(tb.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(tb.Entries))
	}
	if tb.ThreadID != "T1" {
		t.Errorf("expected thread T1, got %s", tb.ThreadID)
	}
}

func TestTracker_GetThreadBudget_Missing(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, "")

	tb := tr.GetThreadBudget("T_NONE")
	if tb != nil {
		t.Error("expected nil for missing thread")
	}
}

func TestTracker_GetDailyBudget(t *testing.T) {
	tr := NewTracker(BudgetConfig{PerDayUSD: 100.0}, "")

	tr.Record("T1", "pm", "openai/gpt-4o", TokenUsage{
		PromptTokens: 500, CompletionTokens: 200, TotalTokens: 700,
	})

	db := tr.GetDailyBudget()
	if db == nil {
		t.Fatal("expected daily budget")
	}
	if len(db.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(db.Entries))
	}
}

func TestTracker_GetDailyBudget_Empty(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, "")

	db := tr.GetDailyBudget()
	if db != nil {
		t.Error("expected nil for empty daily budget")
	}
}

func TestTracker_Concurrent(t *testing.T) {
	tr := NewTracker(BudgetConfig{}, "")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Record("T1", "coder", "openai/gpt-4o", TokenUsage{
				PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
			})
		}()
	}
	wg.Wait()

	tb := tr.GetThreadBudget("T1")
	if tb == nil {
		t.Fatal("expected thread budget")
	}
	if len(tb.Entries) != 50 {
		t.Errorf("expected 50 entries, got %d", len(tb.Entries))
	}
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		tokens TokenUsage
		minUSD float64
		maxUSD float64
	}{
		{
			"opus expensive",
			"anthropic/claude-opus-4-6",
			TokenUsage{PromptTokens: 1000, CompletionTokens: 500},
			0.05, // 1000/1M*15 + 500/1M*75 = 0.015 + 0.0375 = 0.0525
			0.06,
		},
		{
			"mini cheap",
			"openai/gpt-4o-mini",
			TokenUsage{PromptTokens: 1000, CompletionTokens: 500},
			0.0, // very small
			0.001,
		},
		{
			"unknown model uses defaults",
			"unknown/model",
			TokenUsage{PromptTokens: 1000, CompletionTokens: 500},
			0.005, // defaults: $3/$15 per Mtokens
			0.015,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := CalculateCost(tt.model, tt.tokens)
			if cost < tt.minUSD || cost > tt.maxUSD {
				t.Errorf("cost $%.6f not in [%.6f, %.6f]", cost, tt.minUSD, tt.maxUSD)
			}
		})
	}
}

func TestEstimateCost(t *testing.T) {
	cost := EstimateCost("openai/gpt-4o", 10000, 2000)
	if cost <= 0 {
		t.Error("estimate should be positive")
	}

	// gpt-4o: $2.5/M input, $10/M output
	// 10K input: 0.025, 2K output: 0.02, total: 0.045
	expected := 0.045
	if cost < expected*0.9 || cost > expected*1.1 {
		t.Errorf("cost $%.6f not near expected $%.6f", cost, expected)
	}
}

func TestEstimatePlanCost(t *testing.T) {
	steps := []CostEstimate{
		{Model: "openai/gpt-4o", EstimatedInput: 5000, EstimatedOutput: 1000, EstimatedCostUSD: 0.0225},
		{Model: "openai/gpt-4o", EstimatedInput: 3000, EstimatedOutput: 2000, EstimatedCostUSD: 0.0275},
	}

	total := EstimatePlanCost(steps)
	if total != 0.05 {
		t.Errorf("expected $0.05, got $%.4f", total)
	}
}

func TestFormatCostSummary(t *testing.T) {
	tb := &ThreadBudget{
		ThreadID:    "T123",
		TotalCost:   0.45,
		TotalTokens: 50000,
		LimitUSD:    1.0,
		Entries: []UsageEntry{
			{Agent: "pm", CostUSD: 0.15, Tokens: TokenUsage{TotalTokens: 15000}},
			{Agent: "coder", CostUSD: 0.30, Tokens: TokenUsage{TotalTokens: 35000}},
		},
	}

	output := FormatCostSummary(tb)
	if !strings.Contains(output, "T123") {
		t.Error("should contain thread ID")
	}
	if !strings.Contains(output, "$0.4500") {
		t.Error("should contain total cost")
	}
	if !strings.Contains(output, "45.0%") {
		t.Error("should contain percentage")
	}
	if !strings.Contains(output, "pm") {
		t.Error("should contain agent name")
	}
}

func TestFormatCostSummary_Paused(t *testing.T) {
	tb := &ThreadBudget{
		ThreadID:    "T1",
		TotalCost:   1.5,
		TotalTokens: 100000,
		LimitUSD:    1.0,
		Paused:      true,
	}

	output := FormatCostSummary(tb)
	if !strings.Contains(output, "Paused") {
		t.Error("should show paused status")
	}
}

func TestFormatDailySummary(t *testing.T) {
	db := &DailyBudget{
		Date:        "2026-02-26",
		TotalCost:   5.25,
		TotalTokens: 500000,
		LimitUSD:    10.0,
		Entries:     make([]UsageEntry, 42),
	}

	output := FormatDailySummary(db)
	if !strings.Contains(output, "2026-02-26") {
		t.Error("should contain date")
	}
	if !strings.Contains(output, "$5.2500") {
		t.Error("should contain total cost")
	}
	if !strings.Contains(output, "42") {
		t.Error("should contain API call count")
	}
}

func TestFormatDailySummary_Exhausted(t *testing.T) {
	db := &DailyBudget{
		Date:      "2026-02-26",
		TotalCost: 10.5,
		LimitUSD:  10.0,
		Exhausted: true,
	}

	output := FormatDailySummary(db)
	if !strings.Contains(output, "exhausted") {
		t.Error("should show exhausted status")
	}
}

func TestFormatCostEstimate(t *testing.T) {
	steps := []CostEstimate{
		{Model: "openai/gpt-4o", EstimatedInput: 5000, EstimatedOutput: 1000, EstimatedCostUSD: 0.0225},
		{Model: "openai/gpt-4o-mini", EstimatedInput: 3000, EstimatedOutput: 500, EstimatedCostUSD: 0.0008},
	}

	output := FormatCostEstimate(steps)
	if !strings.Contains(output, "Estimated Cost") {
		t.Error("should contain header")
	}
	if !strings.Contains(output, "gpt-4o") {
		t.Error("should contain model name")
	}
	if !strings.Contains(output, "Total estimated") {
		t.Error("should contain total")
	}
}

func TestBudgetExceeded_Error(t *testing.T) {
	threadErr := &BudgetExceeded{
		Scope:     "thread",
		LimitUSD:  1.0,
		ActualUSD: 1.5,
		ThreadID:  "T1",
	}
	if !strings.Contains(threadErr.Error(), "thread T1") {
		t.Error("thread error should mention thread ID")
	}

	dayErr := &BudgetExceeded{
		Scope:     "day",
		LimitUSD:  10.0,
		ActualUSD: 12.0,
	}
	if !strings.Contains(dayErr.Error(), "daily") {
		t.Error("daily error should mention daily")
	}
}

func TestTracker_WithClock(t *testing.T) {
	clk := &fixedClock{now: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)}
	tr := NewTrackerWithClock(BudgetConfig{PerDayUSD: 50.0}, "", clk)

	tr.Record("T1", "pm", "openai/gpt-4o", TokenUsage{
		PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500,
	})

	db := tr.GetDailyBudget()
	if db == nil {
		t.Fatal("expected daily budget")
	}
	if db.Date != "2026-02-26" {
		t.Errorf("expected date 2026-02-26, got %s", db.Date)
	}
}

func TestTracker_ThreadBudgetThenDailyExceeded(t *testing.T) {
	// Thread limit is higher than daily — daily should trigger
	tr := NewTracker(BudgetConfig{
		PerThreadUSD: 100.0,
		PerDayUSD:    0.001,
	}, "")

	err := tr.Record("T1", "coder", "anthropic/claude-opus-4-6", TokenUsage{
		PromptTokens: 10000, CompletionTokens: 5000, TotalTokens: 15000,
	})

	if err == nil {
		t.Fatal("expected error")
	}

	be := err.(*BudgetExceeded)
	if be.Scope != "day" {
		t.Errorf("expected day scope (lower limit), got %s", be.Scope)
	}
}

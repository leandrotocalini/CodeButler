package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultLeadConfig(t *testing.T) {
	cfg := DefaultLeadConfig()
	if cfg.MaxTurns != 20 {
		t.Errorf("expected 20 max turns, got %d", cfg.MaxTurns)
	}
	if cfg.Model == "" {
		t.Error("expected non-empty model")
	}
}

func TestLeadRunner_RunRetrospective(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			// Turn 1: Lead reads thread context
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "l1", Name: "SendMessage", Arguments: `{"text":"@codebutler.coder What was the hardest part of this implementation?"}`},
					},
				},
			},
			// Turn 2: Lead produces retrospective
			{
				Message: Message{
					Role:    "assistant",
					Content: "## Retrospective\n\nWent well: fast implementation. Friction: PM missed test utils.",
				},
			},
		},
	}

	executor := &mockExecutor{
		results: map[string]ToolResult{
			"SendMessage": {Content: "message sent"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"},
			{Name: "Grep"},
			{Name: "SendMessage"},
		},
	}

	lead := NewLeadRunner(
		provider,
		&discardSender{},
		executor,
		DefaultLeadConfig(),
		"You are the Lead agent.",
	)

	results := map[string]*Result{
		"pm":       {TurnsUsed: 3, ToolCalls: 5, TokenUsage: TokenUsage{TotalTokens: 5000}},
		"coder":    {TurnsUsed: 15, ToolCalls: 30, TokenUsage: TokenUsage{TotalTokens: 50000}},
		"reviewer": {TurnsUsed: 2, ToolCalls: 3, TokenUsage: TokenUsage{TotalTokens: 3000}},
	}

	result, err := lead.RunRetrospective(ctx, "User requested login feature. PM planned, Coder implemented, Reviewer approved.", results, "C-test", "T-test")
	if err != nil {
		t.Fatalf("retrospective failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected retrospective response")
	}
}

func TestLeadRunner_Mediate(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role:    "assistant",
					Content: "Decision: The Coder's approach is more maintainable. Proceed with option B.",
				},
			},
		},
	}

	lead := NewLeadRunner(
		provider,
		&discardSender{},
		&mockExecutor{},
		DefaultLeadConfig(),
		"You are the Lead.",
	)

	dispute := FormatMediationContext(
		"coder", "We should use a map for O(1) lookups",
		"reviewer", "A sorted slice is more readable and the dataset is small",
	)

	result, err := lead.Mediate(ctx, dispute, "C-test", "T-test")
	if err != nil {
		t.Fatalf("mediation failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected mediation decision")
	}
}

func TestFormatRetroPrompt(t *testing.T) {
	results := map[string]*Result{
		"pm":    {TurnsUsed: 3, ToolCalls: 5, TokenUsage: TokenUsage{TotalTokens: 5000}},
		"coder": {TurnsUsed: 15, ToolCalls: 30, TokenUsage: TokenUsage{TotalTokens: 50000}, LoopsDetected: 1},
	}

	prompt := FormatRetroPrompt("User wanted login feature.", results)

	if !strings.Contains(prompt, "Retrospective") {
		t.Error("missing retrospective header")
	}
	if !strings.Contains(prompt, "login feature") {
		t.Error("missing thread summary")
	}
	if !strings.Contains(prompt, "pm") {
		t.Error("missing PM metrics")
	}
	if !strings.Contains(prompt, "coder") {
		t.Error("missing Coder metrics")
	}
	if !strings.Contains(prompt, "loops detected") {
		t.Error("missing loop detection info")
	}
	if !strings.Contains(prompt, "3 things that went well") {
		t.Error("missing went-well instruction")
	}
	if !strings.Contains(prompt, "3 friction points") {
		t.Error("missing friction instruction")
	}
}

func TestFormatLearning(t *testing.T) {
	l := Learning{
		When:       "When reviewing auth code",
		Rule:       "Always check for SQL injection in user input",
		Example:    "handler.go:42 was vulnerable",
		Confidence: 0.9,
		Source:     "T-123",
	}

	formatted := FormatLearning(l)

	if !strings.Contains(formatted, "When reviewing auth code") {
		t.Error("missing when clause")
	}
	if !strings.Contains(formatted, "SQL injection") {
		t.Error("missing rule")
	}
	if !strings.Contains(formatted, "handler.go:42") {
		t.Error("missing example")
	}
	if !strings.Contains(formatted, "90%") {
		t.Error("missing confidence")
	}
	if !strings.Contains(formatted, "T-123") {
		t.Error("missing source")
	}
}

func TestFormatLearning_NoExample(t *testing.T) {
	l := Learning{
		When:       "Always",
		Rule:       "Use structured logging",
		Confidence: 0.8,
		Source:     "T-456",
	}

	formatted := FormatLearning(l)
	if strings.Contains(formatted, "Example") {
		t.Error("should not include example section when empty")
	}
}

func TestPruneLearnings(t *testing.T) {
	learnings := []Learning{
		{Rule: "low confidence", Confidence: 0.2, Source: "T-1"},
		{Rule: "medium confidence", Confidence: 0.5, Source: "T-2"},
		{Rule: "high confidence", Confidence: 0.9, Source: "T-3"},
		{Rule: "very high confidence", Confidence: 1.0, Source: "T-4"},
	}

	pruned, reasons := PruneLearnings(learnings, 0)
	if len(pruned) != 3 {
		t.Fatalf("expected 3 after removing low-confidence, got %d", len(pruned))
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(reasons))
	}
	if !strings.Contains(reasons[0], "low-confidence") {
		t.Errorf("reason should mention low-confidence: %s", reasons[0])
	}
}

func TestPruneLearnings_CapEnforced(t *testing.T) {
	learnings := []Learning{
		{Rule: "a", Confidence: 0.5},
		{Rule: "b", Confidence: 0.6},
		{Rule: "c", Confidence: 0.7},
		{Rule: "d", Confidence: 0.8},
		{Rule: "e", Confidence: 0.9},
	}

	pruned, reasons := PruneLearnings(learnings, 3)
	if len(pruned) != 3 {
		t.Errorf("expected 3 after cap, got %d", len(pruned))
	}
	// Should keep the newest (last) 3
	if pruned[0].Rule != "c" {
		t.Errorf("expected 'c' as first after pruning, got %q", pruned[0].Rule)
	}

	hasCapReason := false
	for _, r := range reasons {
		if strings.Contains(r, "cap") {
			hasCapReason = true
		}
	}
	if !hasCapReason {
		t.Error("expected cap enforcement reason")
	}
}

func TestNewThreadReport(t *testing.T) {
	results := map[string]*Result{
		"pm": {
			TurnsUsed:  3,
			ToolCalls:  5,
			TokenUsage: TokenUsage{TotalTokens: 5000},
		},
		"coder": {
			TurnsUsed:     15,
			ToolCalls:     30,
			TokenUsage:    TokenUsage{TotalTokens: 50000},
			LoopsDetected: 1,
		},
	}

	report := NewThreadReport("T-test-001", results)

	if report.ThreadID != "T-test-001" {
		t.Errorf("thread ID: got %q", report.ThreadID)
	}
	if len(report.AgentMetrics) != 2 {
		t.Errorf("expected 2 agent metrics, got %d", len(report.AgentMetrics))
	}
	if report.AgentMetrics["coder"].LoopsDetected != 1 {
		t.Error("coder loops should be 1")
	}
	if report.TotalCost <= 0 {
		t.Error("expected non-zero cost estimate")
	}
}

func TestNewThreadReport_NilResults(t *testing.T) {
	results := map[string]*Result{
		"pm":   nil,
		"coder": {TurnsUsed: 5, TokenUsage: TokenUsage{TotalTokens: 1000}},
	}

	report := NewThreadReport("T-nil", results)
	if len(report.AgentMetrics) != 1 {
		t.Errorf("expected 1 metric (nil PM skipped), got %d", len(report.AgentMetrics))
	}
}

func TestMarshalReport(t *testing.T) {
	report := ThreadReport{
		ThreadID: "T-test",
		Outcome:  "success",
		AgentMetrics: map[string]AgentMetrics{
			"pm": {TurnsUsed: 3},
		},
	}

	data, err := MarshalReport(report)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed["thread_id"] != "T-test" {
		t.Error("missing thread_id in JSON")
	}
}

func TestFormatUsageReport(t *testing.T) {
	report := ThreadReport{
		ThreadID: "T-test",
		Outcome:  "success",
		AgentMetrics: map[string]AgentMetrics{
			"pm":    {TurnsUsed: 3, ToolCalls: 5, TokensUsed: 5000, LoopsDetected: 0},
			"coder": {TurnsUsed: 15, ToolCalls: 30, TokensUsed: 50000, LoopsDetected: 1},
		},
		TotalCost: 0.495,
	}

	text := FormatUsageReport(report)

	if !strings.Contains(text, "Usage Report") {
		t.Error("missing header")
	}
	if !strings.Contains(text, "T-test") {
		t.Error("missing thread ID")
	}
	if !strings.Contains(text, "pm") {
		t.Error("missing PM entry")
	}
	if !strings.Contains(text, "coder") {
		t.Error("missing Coder entry")
	}
	if !strings.Contains(text, "$0.4950") {
		t.Error("missing cost")
	}
}

func TestFormatMediationContext(t *testing.T) {
	ctx := FormatMediationContext(
		"coder", "Use maps for O(1) lookup",
		"reviewer", "Use sorted slice for readability",
	)

	if !strings.Contains(ctx, "coder's position") {
		t.Error("missing coder position")
	}
	if !strings.Contains(ctx, "reviewer's position") {
		t.Error("missing reviewer position")
	}
	if !strings.Contains(ctx, "O(1)") {
		t.Error("missing coder's argument")
	}
}

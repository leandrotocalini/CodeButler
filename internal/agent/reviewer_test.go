package agent

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultReviewerConfig(t *testing.T) {
	cfg := DefaultReviewerConfig()
	if cfg.MaxRounds != 3 {
		t.Errorf("expected 3 max rounds, got %d", cfg.MaxRounds)
	}
	if cfg.MaxTurns != 30 {
		t.Errorf("expected 30 max turns, got %d", cfg.MaxTurns)
	}
	if cfg.BaseBranch != "main" {
		t.Errorf("expected main base branch, got %s", cfg.BaseBranch)
	}
}

func TestReviewerRunner_ReviewWithDiff(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "r1", Name: "Read", Arguments: `{"path":"internal/auth/handler.go"}`},
					},
				},
			},
			{
				Message: Message{
					Role:    "assistant",
					Content: "Review complete. 1. [security] handler.go:10 — missing input validation (blocker)",
				},
			},
		},
	}

	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "package auth\n\nfunc Handler() {}"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"},
			{Name: "Grep"},
			{Name: "Glob"},
		},
	}

	reviewer := NewReviewerRunner(
		provider,
		&discardSender{},
		executor,
		DefaultReviewerConfig(),
		"You are the Reviewer.",
	)

	diff := `diff --git a/internal/auth/handler.go b/internal/auth/handler.go
+func Handler() {
+    user := getUser()
+    fmt.Println(user.Name)
+}`

	result, err := reviewer.ReviewWithDiff(ctx, diff, "codebutler/feat", "C-test", "T-test")
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected review response")
	}
	if reviewer.CurrentRound() != 1 {
		t.Errorf("expected round 1, got %d", reviewer.CurrentRound())
	}
}

func TestReviewerRunner_CanReview(t *testing.T) {
	reviewer := NewReviewerRunner(
		&mockProvider{responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "ok"}},
			{Message: Message{Role: "assistant", Content: "ok"}},
			{Message: Message{Role: "assistant", Content: "ok"}},
		}},
		&discardSender{},
		&mockExecutor{},
		ReviewerConfig{MaxRounds: 3, MaxTurns: 10, BaseBranch: "main", Model: "test"},
		"test",
	)

	ctx := context.Background()

	if !reviewer.CanReview() {
		t.Error("should be able to review at round 0")
	}

	reviewer.ReviewWithDiff(ctx, "diff1", "branch", "C", "T")
	if !reviewer.CanReview() {
		t.Error("should be able to review at round 1")
	}

	reviewer.ReviewWithDiff(ctx, "diff2", "branch", "C", "T")
	if !reviewer.CanReview() {
		t.Error("should be able to review at round 2")
	}

	reviewer.ReviewWithDiff(ctx, "diff3", "branch", "C", "T")
	if reviewer.CanReview() {
		t.Error("should NOT be able to review at round 3 (max reached)")
	}
}

func TestFormatReviewPrompt(t *testing.T) {
	diff := "+func Hello() {}"
	prompt := FormatReviewPrompt(diff, "feat/login", "main", 1, 3)

	if !strings.Contains(prompt, "Review Round 1/3") {
		t.Error("missing round info")
	}
	if !strings.Contains(prompt, "feat/login") {
		t.Error("missing branch name")
	}
	if !strings.Contains(prompt, "+func Hello() {}") {
		t.Error("missing diff content")
	}
	if !strings.Contains(prompt, "Invariants") {
		t.Error("missing invariants section")
	}
	if !strings.Contains(prompt, "Risk Matrix") {
		t.Error("missing risk matrix section")
	}
	if !strings.Contains(prompt, "Test Plan") {
		t.Error("missing test plan section")
	}
}

func TestFormatReviewPrompt_ReReview(t *testing.T) {
	prompt := FormatReviewPrompt("diff", "feat", "main", 2, 3)
	if !strings.Contains(prompt, "re-review") {
		t.Error("re-review indicator missing for round > 1")
	}
}

func TestFormatReviewPrompt_FinalRound(t *testing.T) {
	prompt := FormatReviewPrompt("diff", "feat", "main", 3, 3)
	if !strings.Contains(prompt, "final round") {
		t.Error("final round indicator missing")
	}
}

func TestFormatReviewFeedback(t *testing.T) {
	issues := []ReviewIssue{
		{Tag: "security", File: "handler.go", Line: 42, Message: "SQL injection", Severity: "blocker"},
		{Tag: "test", File: "handler_test.go", Line: 0, Message: "missing edge case test", Severity: "warning"},
		{Tag: "quality", File: "util.go", Line: 10, Message: "consider using suggestion pattern", Severity: "suggestion"},
	}

	feedback := FormatReviewFeedback(issues)

	if !strings.Contains(feedback, "1. [security] handler.go:42") {
		t.Error("missing security issue with file:line")
	}
	if !strings.Contains(feedback, "SQL injection") {
		t.Error("missing issue message")
	}
	if !strings.Contains(feedback, "(blocker)") {
		t.Error("missing blocker marker")
	}
	if !strings.Contains(feedback, "2. [test] handler_test.go") {
		t.Error("missing test issue")
	}
	if !strings.Contains(feedback, "3. [quality] util.go:10") {
		t.Error("missing quality issue")
	}
}

func TestFormatReviewFeedback_NoIssues(t *testing.T) {
	feedback := FormatReviewFeedback(nil)
	if !strings.Contains(feedback, "LGTM") {
		t.Error("expected LGTM for no issues")
	}
}

func TestParseReviewIssues(t *testing.T) {
	text := `Here are the issues I found:

1. [security] handler.go:42 — SQL injection in user query (blocker)
2. [test] handler_test.go — missing test for nil user case
3. [quality] util.go:10 — redundant error wrapping
- [consistency] config.go:5 — doesn't follow project naming convention
`

	issues := ParseReviewIssues(text)

	if len(issues) != 4 {
		t.Fatalf("expected 4 issues, got %d: %+v", len(issues), issues)
	}

	// Check first issue
	if issues[0].Tag != "security" {
		t.Errorf("issue 0 tag: got %q", issues[0].Tag)
	}
	if issues[0].Severity != "blocker" {
		t.Errorf("issue 0 severity: got %q", issues[0].Severity)
	}
	if !strings.Contains(issues[0].Message, "SQL injection") {
		t.Errorf("issue 0 message: got %q", issues[0].Message)
	}

	// Check test issue
	if issues[1].Tag != "test" {
		t.Errorf("issue 1 tag: got %q", issues[1].Tag)
	}

	// Check quality issue
	if issues[2].Tag != "quality" {
		t.Errorf("issue 2 tag: got %q", issues[2].Tag)
	}

	// Check consistency issue
	if issues[3].Tag != "consistency" {
		t.Errorf("issue 3 tag: got %q", issues[3].Tag)
	}
}

func TestParseReviewIssues_NoIssues(t *testing.T) {
	issues := ParseReviewIssues("This code looks great! No issues found.")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestParseReviewIssues_InvalidTags(t *testing.T) {
	text := `1. [typo] file.go:1 — misspelling
2. [opinion] file.go:2 — I prefer tabs`

	issues := ParseReviewIssues(text)
	if len(issues) != 0 {
		t.Errorf("expected 0 valid issues, got %d", len(issues))
	}
}

func TestHasBlockers(t *testing.T) {
	tests := []struct {
		name   string
		issues []ReviewIssue
		want   bool
	}{
		{
			name:   "no issues",
			issues: nil,
			want:   false,
		},
		{
			name: "only warnings",
			issues: []ReviewIssue{
				{Severity: "warning"},
				{Severity: "suggestion"},
			},
			want: false,
		},
		{
			name: "has blocker",
			issues: []ReviewIssue{
				{Severity: "warning"},
				{Severity: "blocker"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasBlockers(tt.issues); got != tt.want {
				t.Errorf("HasBlockers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountByTag(t *testing.T) {
	issues := []ReviewIssue{
		{Tag: "security"},
		{Tag: "security"},
		{Tag: "test"},
		{Tag: "quality"},
	}

	counts := CountByTag(issues)
	if counts["security"] != 2 {
		t.Errorf("security: got %d", counts["security"])
	}
	if counts["test"] != 1 {
		t.Errorf("test: got %d", counts["test"])
	}
	if counts["quality"] != 1 {
		t.Errorf("quality: got %d", counts["quality"])
	}
}

func TestTwoPassReviewPrompt(t *testing.T) {
	diff := "+func foo() {}"
	prompt := TwoPassReviewPrompt(diff)

	if !strings.Contains(prompt, "Quick review") {
		t.Error("missing quick review header")
	}
	if !strings.Contains(prompt, "Security vulnerabilities") {
		t.Error("missing security check")
	}
	if !strings.Contains(prompt, "+func foo() {}") {
		t.Error("missing diff content")
	}
}

func TestNeedsDeepReview(t *testing.T) {
	tests := []struct {
		response string
		want     bool
	}{
		{"LGTM — no obvious issues.", false},
		{"LGTM - no obvious issues found", false},
		{"Found potential SQL injection in handler.go", true},
		{"Issues: 1. Missing error handling", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.response, func(t *testing.T) {
			if got := NeedsDeepReview(tt.response); got != tt.want {
				t.Errorf("NeedsDeepReview(%q) = %v, want %v", tt.response, got, tt.want)
			}
		})
	}
}

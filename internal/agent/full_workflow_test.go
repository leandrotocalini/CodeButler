package agent

import (
	"context"
	"strings"
	"testing"
)

// TestFullWorkflow_PMCoderReviewerLead tests the complete implement workflow:
// User → PM plans → Coder implements → Reviewer reviews → Lead retrospective → done.
// All LLM calls are mocked. This verifies the wiring and handoff contracts.
func TestFullWorkflow_PMCoderReviewerLead(t *testing.T) {
	ctx := context.Background()
	channel := "C-full-e2e"
	thread := "T-full-e2e-001"

	// =========================================================
	// Phase 1: PM receives user request, explores, proposes plan
	// =========================================================
	pmProvider := &mockProvider{
		responses: []*ChatResponse{
			// PM explores codebase
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "pm-1", Name: "Read", Arguments: `{"path":"main.go"}`},
						{ID: "pm-2", Name: "Grep", Arguments: `{"pattern":"handler","path":"."}`},
					},
				},
			},
			// PM proposes plan with delegation to Coder
			{
				Message: Message{
					Role: "assistant",
					Content: `I've explored the codebase. Here's my plan:

@codebutler.coder

## Task

Add rate limiting to the API endpoints.

Changes:
- internal/middleware/ratelimit.go:1 — create rate limiter middleware
- internal/server/routes.go:15 — wire rate limiter to routes
- internal/middleware/ratelimit_test.go:1 — add tests

## Context

The server uses stdlib net/http. No existing rate limiting.
Complexity: medium (new middleware + route wiring).`,
				},
			},
		},
	}

	pmExecutor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "package main"},
			"Grep": {Content: "server.go:10:func handler()"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"},
			{Name: "SendMessage"}, {Name: "Bash"},
		},
	}

	pmRunner := NewPMRunner(
		pmProvider,
		&discardSender{},
		pmExecutor,
		DefaultPMConfig(),
		"You are the PM.",
		WithPMWorkflows(DefaultWorkflows()),
	)

	pmResult, intent, err := pmRunner.ClassifyAndRun(ctx, Task{
		Messages: []Message{
			{Role: "user", Content: "add rate limiting to all API endpoints"},
		},
		Channel: channel,
		Thread:  thread,
	})
	if err != nil {
		t.Fatalf("PM failed: %v", err)
	}

	// Verify PM classification
	if intent.Type != IntentWorkflow {
		t.Errorf("expected workflow intent, got %s", intent.Type)
	}

	// Verify PM produced plan with delegation
	if !strings.Contains(pmResult.Response, "@codebutler.coder") {
		t.Fatal("PM should delegate to Coder")
	}

	plan := pmResult.Response

	// =====================================================
	// Phase 2: Coder receives plan, implements, creates PR
	// =====================================================
	coderProvider := &mockProvider{
		responses: []*ChatResponse{
			// Read existing code
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-1", Name: "Read", Arguments: `{"path":"internal/server/routes.go"}`},
			}}},
			// Write rate limiter
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-2", Name: "Write", Arguments: `{"path":"internal/middleware/ratelimit.go","content":"package middleware"}`},
			}}},
			// Write test
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-3", Name: "Write", Arguments: `{"path":"internal/middleware/ratelimit_test.go","content":"package middleware"}`},
			}}},
			// Edit routes
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-4", Name: "Edit", Arguments: `{"path":"internal/server/routes.go","old":"// routes","new":"ratelimit.New()"}`},
			}}},
			// Run tests
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-5", Name: "Bash", Arguments: `{"command":"go test ./..."}`},
			}}},
			// Commit + Push + PR
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-6", Name: "GitCommit", Arguments: `{"files":["internal/middleware/ratelimit.go","internal/middleware/ratelimit_test.go","internal/server/routes.go"],"message":"feat: add rate limiting"}`},
			}}},
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-7", Name: "GitPush", Arguments: `{}`},
			}}},
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-8", Name: "GHCreatePR", Arguments: `{"title":"feat: add rate limiting","body":"Rate limiting middleware","base":"main","head":"codebutler/rate-limit"}`},
			}}},
			// Notify reviewer
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c-9", Name: "SendMessage", Arguments: `{"text":"@codebutler.reviewer PR ready: https://github.com/org/repo/pull/99"}`},
			}}},
			// Done
			{Message: Message{Role: "assistant", Content: "PR #99 created. Sent to Reviewer."}},
		},
	}

	coderExecutor := &mockExecutor{
		results: map[string]ToolResult{
			"Read":        {Content: "// routes"},
			"Write":       {Content: "written"},
			"Edit":        {Content: "edited"},
			"Bash":        {Content: "PASS"},
			"GitCommit":   {Content: "committed"},
			"GitPush":     {Content: "pushed"},
			"GHCreatePR":  {Content: "https://github.com/org/repo/pull/99"},
			"SendMessage": {Content: "sent"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Write"}, {Name: "Edit"},
			{Name: "Bash"}, {Name: "Grep"}, {Name: "Glob"},
			{Name: "GitCommit"}, {Name: "GitPush"}, {Name: "GHCreatePR"},
			{Name: "SendMessage"},
		},
	}

	coderRunner := NewCoderRunner(
		coderProvider,
		&discardSender{},
		coderExecutor,
		CoderConfig{
			Model:       "anthropic/claude-sonnet-4-20250514",
			MaxTurns:    50,
			WorktreeDir: "/repo/.codebutler/branches/codebutler/rate-limit",
			BaseBranch:  "main",
			HeadBranch:  "codebutler/rate-limit",
		},
		"You are the Coder.",
	)

	coderResult, err := coderRunner.RunWithPlan(ctx, plan, channel, thread)
	if err != nil {
		t.Fatalf("Coder failed: %v", err)
	}
	if !strings.Contains(coderResult.Response, "PR") {
		t.Error("Coder should mention PR in response")
	}

	// =========================================================
	// Phase 3: Reviewer reviews the diff
	// =========================================================
	reviewerProvider := &mockProvider{
		responses: []*ChatResponse{
			// Reviewer reads the diff
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "r-1", Name: "Read", Arguments: `{"path":"internal/middleware/ratelimit.go"}`},
			}}},
			// Reviewer produces structured feedback
			{Message: Message{Role: "assistant", Content: `## Review

### Invariants
- Existing endpoints must continue working
- Rate limit should not affect health checks

### Risk Matrix
- Security: low (rate limiting adds protection)
- Performance: medium (per-request overhead)
- Compatibility: none
- Correctness: low

### Test Plan
- Unit test for rate limiter middleware
- Integration test for rate-limited endpoint

### Issues
1. [test] ratelimit_test.go — missing test for burst capacity edge case
2. [quality] ratelimit.go:15 — consider using sync.Pool for token bucket allocation (suggestion)

Overall: minor issues. LGTM with the test addition.

@codebutler.lead Review complete. PR approved with minor suggestions.`}},
		},
	}

	reviewerExecutor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "package middleware\n\nfunc RateLimit() {}"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"},
			{Name: "SendMessage"},
		},
	}

	reviewerRunner := NewReviewerRunner(
		reviewerProvider,
		&discardSender{},
		reviewerExecutor,
		DefaultReviewerConfig(),
		"You are the Reviewer.",
	)

	diff := `diff --git a/internal/middleware/ratelimit.go b/internal/middleware/ratelimit.go
new file mode 100644
+package middleware
+
+func RateLimit() http.Handler {
+    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+        // rate limit logic
+    })
+}`

	reviewResult, err := reviewerRunner.ReviewWithDiff(ctx, diff, "codebutler/rate-limit", channel, thread)
	if err != nil {
		t.Fatalf("Reviewer failed: %v", err)
	}

	// Verify review produced structured feedback
	if reviewResult.Response == "" {
		t.Error("Reviewer should produce a response")
	}
	if !strings.Contains(reviewResult.Response, "@codebutler.lead") {
		t.Error("Reviewer should hand off to Lead")
	}

	// Parse review issues from the response
	issues := ParseReviewIssues(reviewResult.Response)
	if len(issues) < 1 {
		t.Errorf("expected at least 1 review issue, got %d", len(issues))
	}

	// No blockers, so review is approved
	if HasBlockers(issues) {
		t.Error("expected no blockers in this review")
	}

	// =========================================================
	// Phase 4: Lead runs retrospective
	// =========================================================
	leadProvider := &mockProvider{
		responses: []*ChatResponse{
			// Lead discusses with agents
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "l-1", Name: "SendMessage", Arguments: `{"text":"@codebutler.coder What was the hardest part of implementing rate limiting?"}`},
			}}},
			// Lead produces retrospective
			{Message: Message{Role: "assistant", Content: `## Retrospective

### Went Well
1. PM correctly identified the need for rate limiting and provided clear file references
2. Coder implemented cleanly with tests included from the start
3. Reviewer caught the burst capacity edge case

### Friction
1. PM didn't check for existing rate limiting libraries in go.mod
2. Reviewer's sync.Pool suggestion was a style preference, not a real issue
3. No design discussion before implementation (straight to code)

### Proposals
1. **Process:** Add a "check existing dependencies" step to the implement workflow
2. **Prompt:** Update Reviewer MD to distinguish style suggestions from real issues
3. **Skill:** Create a "dependency check" skill for PM to find relevant libraries
4. **Guardrail:** Coder should run benchmarks when adding middleware (performance-sensitive path)`}},
		},
	}

	leadExecutor := &mockExecutor{
		results: map[string]ToolResult{
			"SendMessage": {Content: "sent"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"},
			{Name: "SendMessage"},
		},
	}

	leadRunner := NewLeadRunner(
		leadProvider,
		&discardSender{},
		leadExecutor,
		DefaultLeadConfig(),
		"You are the Lead.",
	)

	agentResults := map[string]*Result{
		"pm":       pmResult,
		"coder":    coderResult,
		"reviewer": reviewResult,
	}

	leadResult, err := leadRunner.RunRetrospective(
		ctx,
		"User requested rate limiting. PM planned, Coder implemented, Reviewer approved with minor suggestions.",
		agentResults,
		channel,
		thread,
	)
	if err != nil {
		t.Fatalf("Lead failed: %v", err)
	}
	if leadResult.Response == "" {
		t.Error("Lead should produce a retrospective response")
	}
	if !strings.Contains(leadResult.Response, "Went Well") {
		t.Error("Lead response should contain 'Went Well' section")
	}
	if !strings.Contains(leadResult.Response, "Friction") {
		t.Error("Lead response should contain 'Friction' section")
	}

	// =========================================================
	// Phase 5: Verify thread report generation
	// =========================================================
	agentResults["lead"] = leadResult
	report := NewThreadReport(thread, agentResults)

	if report.ThreadID != thread {
		t.Errorf("thread ID: got %q", report.ThreadID)
	}
	if len(report.AgentMetrics) != 4 {
		t.Errorf("expected 4 agent metrics (pm, coder, reviewer, lead), got %d", len(report.AgentMetrics))
	}

	// Cost is zero with mocks (no real token usage) — verify it doesn't panic
	_ = report.TotalCost

	// Verify report can be serialized
	data, err := MarshalReport(report)
	if err != nil {
		t.Fatalf("report marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty report JSON")
	}

	// Verify usage report formatting
	report.Outcome = "success"
	usageText := FormatUsageReport(report)
	if !strings.Contains(usageText, "pm") || !strings.Contains(usageText, "coder") {
		t.Error("usage report should include all agents")
	}

	// =========================================================
	// Phase 6: Verify all handoff contracts
	// =========================================================

	// PM → Coder: plan contains @codebutler.coder + Task section + file refs
	if !strings.Contains(plan, "## Task") {
		t.Error("PM plan should have Task section")
	}
	_, fileRefs := ParsePlan(plan)
	if len(fileRefs) < 2 {
		t.Errorf("PM plan should have file refs, got %d", len(fileRefs))
	}

	// Coder → Reviewer: PR notification with @codebutler.reviewer
	coderSentReviewerHandoff := false
	for _, req := range coderProvider.requests {
		for _, m := range req.Messages {
			for _, tc := range m.ToolCalls {
				if tc.Name == "SendMessage" && strings.Contains(tc.Arguments, "@codebutler.reviewer") {
					coderSentReviewerHandoff = true
				}
			}
		}
	}
	if !coderSentReviewerHandoff {
		t.Error("Coder should send handoff to Reviewer via SendMessage")
	}

	// Reviewer → Lead: review response contains @codebutler.lead
	if !strings.Contains(reviewResult.Response, "@codebutler.lead") {
		t.Error("Reviewer should hand off to Lead")
	}

	// Lead discussed with agents
	leadDiscussed := false
	for _, req := range leadProvider.requests {
		for _, m := range req.Messages {
			for _, tc := range m.ToolCalls {
				if tc.Name == "SendMessage" && strings.Contains(tc.Arguments, "@codebutler.coder") {
					leadDiscussed = true
				}
			}
		}
	}
	if !leadDiscussed {
		t.Error("Lead should discuss with agents via SendMessage")
	}
}

// TestFullWorkflow_ReviewerFeedbackLoop tests the review loop:
// Reviewer finds blockers → Coder fixes → Reviewer re-reviews → approved.
func TestFullWorkflow_ReviewerFeedbackLoop(t *testing.T) {
	ctx := context.Background()

	// Round 1: Reviewer finds a blocker
	reviewerProvider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: `1. [security] handler.go:42 — SQL injection via unsanitized input (blocker)
2. [test] handler_test.go — missing test for error path`}},
			// Round 2: Reviewer approves after fix
			{Message: Message{Role: "assistant", Content: `Previous issues addressed. LGTM!

@codebutler.lead Review complete. PR approved.`}},
		},
	}

	reviewer := NewReviewerRunner(
		reviewerProvider,
		&discardSender{},
		&mockExecutor{},
		DefaultReviewerConfig(),
		"You are the Reviewer.",
	)

	// Round 1
	result1, err := reviewer.ReviewWithDiff(ctx, "diff with injection", "feat", "C", "T")
	if err != nil {
		t.Fatalf("round 1 failed: %v", err)
	}

	issues := ParseReviewIssues(result1.Response)
	if !HasBlockers(issues) {
		t.Error("round 1 should have blockers")
	}

	// Coder fixes and pushes (simulated)
	// Round 2: re-review
	if !reviewer.CanReview() {
		t.Fatal("should be able to review again")
	}

	result2, err := reviewer.ReviewWithDiff(ctx, "diff with fix", "feat", "C", "T")
	if err != nil {
		t.Fatalf("round 2 failed: %v", err)
	}

	issues2 := ParseReviewIssues(result2.Response)
	if HasBlockers(issues2) {
		t.Error("round 2 should not have blockers")
	}
	if !strings.Contains(result2.Response, "@codebutler.lead") {
		t.Error("round 2 should approve and notify Lead")
	}
}

// TestFullWorkflow_LeadMediatesDisagreement tests the mediation flow.
func TestFullWorkflow_LeadMediatesDisagreement(t *testing.T) {
	ctx := context.Background()

	leadProvider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: `After reviewing both positions:

The Coder's approach of using a map is correct for this use case. The dataset
will grow beyond 100 entries based on the roadmap, making O(1) lookup important.

Decision: Use a map. Reviewer should update their mental model of expected
dataset sizes for this component.`}},
		},
	}

	lead := NewLeadRunner(
		leadProvider,
		&discardSender{},
		&mockExecutor{},
		DefaultLeadConfig(),
		"You are the Lead.",
	)

	dispute := FormatMediationContext(
		"coder", "Map gives O(1) lookup. Dataset will grow.",
		"reviewer", "Slice is simpler. Dataset is small.",
	)

	result, err := lead.Mediate(ctx, dispute, "C", "T")
	if err != nil {
		t.Fatalf("mediation failed: %v", err)
	}
	if result.Response == "" {
		t.Error("Lead should produce a mediation decision")
	}
	if !strings.Contains(result.Response, "map") {
		t.Error("Lead should reference the chosen approach")
	}
}

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- E2E: User → PM → Coder → PR ---
//
// These tests verify the full flow: a user posts a feature request,
// the PM classifies and proposes a plan, the plan is handed off to the
// Coder, and the Coder implements and creates a PR.
//
// All LLM calls are mocked — these are integration tests for the wiring,
// not the models.

// captureSender records all messages sent.
type captureSender struct {
	messages []capturedMessage
}

type capturedMessage struct {
	Channel string
	Thread  string
	Text    string
}

func (s *captureSender) SendMessage(_ context.Context, channel, thread, text string) error {
	s.messages = append(s.messages, capturedMessage{
		Channel: channel,
		Thread:  thread,
		Text:    text,
	})
	return nil
}

// gitTracker records git tool calls to verify commit/push/PR creation.
type gitTracker struct {
	committed bool
	pushed    bool
	prCreated bool
	prURL     string
	files     []string
	message   string
}

func (g *gitTracker) toolResults() map[string]ToolResult {
	return map[string]ToolResult{
		"Read": {Content: `package main\n\nfunc main() {\n\tprintln("hello")\n}`},
		"Write": {Content: "file written successfully"},
		"Edit":  {Content: "edit applied successfully"},
		"Bash":  {Content: "PASS\nok  \tgithub.com/example/proj\t0.5s"},
		"Grep":  {Content: "main.go:1:package main"},
		"Glob":  {Content: "main.go\ninternal/auth/handler.go"},
		"GitCommit": {Content: "committed 2 files"},
		"GitPush":   {Content: "pushed to origin/codebutler/add-login"},
		"GHCreatePR": {Content: "https://github.com/org/repo/pull/42"},
		"SendMessage": {Content: "message sent"},
	}
}

func TestE2E_UserToPMToCoderToPR(t *testing.T) {
	ctx := context.Background()

	// === Phase 1: PM receives user request and proposes plan ===
	pmSender := &captureSender{}
	pmProvider := &mockProvider{
		responses: []*ChatResponse{
			// Turn 1: PM uses SendMessage to ask a clarifying question
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "pm-1",
							Name:      "SendMessage",
							Arguments: `{"text": "I'll help you add a login page. Let me explore the codebase first."}`,
						},
					},
				},
			},
			// Turn 2: PM explores codebase
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "pm-2",
							Name:      "Read",
							Arguments: `{"path": "main.go"}`,
						},
						{
							ID:        "pm-3",
							Name:      "Glob",
							Arguments: `{"pattern": "internal/**/*.go"}`,
						},
					},
				},
			},
			// Turn 3: PM proposes plan and delegates to Coder
			{
				Message: Message{
					Role: "assistant",
					Content: `@codebutler.coder

## Task

Implement a login page with JWT authentication.

Changes:
- internal/auth/handler.go:1 — create new auth handler with login endpoint
- internal/auth/middleware.go:1 — create JWT middleware
- cmd/main.go:10 — wire auth routes

## Context

The project uses Go with a simple main.go entrypoint. No existing auth.`,
				},
			},
		},
	}

	pmExecutor := &mockExecutor{
		results: map[string]ToolResult{
			"SendMessage": {Content: "message sent"},
			"Read":        {Content: `package main\n\nfunc main() {\n\tprintln("hello")\n}`},
			"Glob":        {Content: "main.go\ninternal/server/server.go"},
		},
		toolDefs: []ToolDefinition{
			{Name: "SendMessage", Description: "Send a Slack message"},
			{Name: "Read", Description: "Read a file"},
			{Name: "Glob", Description: "Search files by pattern"},
			{Name: "Grep", Description: "Search file contents"},
			{Name: "Bash", Description: "Run a shell command"},
		},
	}

	pmRunner := NewPMRunner(
		pmProvider,
		pmSender,
		pmExecutor,
		PMConfig{
			Model:    "anthropic/claude-sonnet-4-20250514",
			MaxTurns: 15,
		},
		"You are the PM agent. Plan work and delegate to Coder.",
		WithPMWorkflows(DefaultWorkflows()),
	)

	// Run PM
	pmResult, intent, err := pmRunner.ClassifyAndRun(ctx, Task{
		Messages: []Message{
			{Role: "user", Content: "implement a login page with JWT authentication"},
		},
		Channel: "C-test",
		Thread:  "T-test-001",
	})

	if err != nil {
		t.Fatalf("PM run failed: %v", err)
	}
	if pmResult.Response == "" {
		t.Fatal("PM should produce a text response with the plan")
	}

	// Verify PM intent classification
	if intent.Type != IntentWorkflow || intent.Name != "implement" {
		t.Errorf("expected implement workflow, got %s/%s", intent.Type, intent.Name)
	}

	// Verify PM used tools (explored codebase)
	if pmResult.ToolCalls == 0 {
		t.Error("expected PM to use tools for exploration")
	}

	// Verify PM's response contains a delegation to Coder
	if !strings.Contains(pmResult.Response, "@codebutler.coder") {
		t.Error("PM response should contain @codebutler.coder delegation")
	}

	// === Phase 2: Extract plan and verify structure ===
	plan, fileRefs := ParsePlan(pmResult.Response)
	if plan == "" {
		t.Fatal("parsed plan should not be empty")
	}
	if len(fileRefs) < 2 {
		t.Errorf("expected at least 2 file refs in plan, got %d", len(fileRefs))
	}

	// Verify complexity classification
	complexity := ClassifyComplexity(plan)
	if complexity != ComplexityComplex {
		// "authentication" is a complex signal
		t.Errorf("expected complex classification for auth task, got %s", complexity)
	}
	coderModel := ModelForComplexity(complexity, "")
	if !strings.Contains(coderModel, "opus") {
		t.Errorf("expected opus for complex task, got %s", coderModel)
	}

	// === Phase 3: Coder receives plan and implements ===
	coderSender := &captureSender{}
	tracker := &gitTracker{}
	coderProvider := &mockProvider{
		responses: []*ChatResponse{
			// Turn 1: Coder reads existing files
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-1",
							Name:      "Read",
							Arguments: `{"path": "main.go"}`,
						},
					},
				},
			},
			// Turn 2: Coder creates auth handler
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-2",
							Name:      "Write",
							Arguments: `{"path": "internal/auth/handler.go", "content": "package auth\n\nfunc LoginHandler() {}"}`,
						},
					},
				},
			},
			// Turn 3: Coder creates middleware
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-3",
							Name:      "Write",
							Arguments: `{"path": "internal/auth/middleware.go", "content": "package auth\n\nfunc JWTMiddleware() {}"}`,
						},
					},
				},
			},
			// Turn 4: Coder runs tests
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-4",
							Name:      "Bash",
							Arguments: `{"command": "go test ./..."}`,
						},
					},
				},
			},
			// Turn 5: Coder commits, pushes, creates PR
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:   "c-5",
							Name: "GitCommit",
							Arguments: mustJSON(t, map[string]interface{}{
								"files":   []string{"internal/auth/handler.go", "internal/auth/middleware.go", "cmd/main.go"},
								"message": "feat: add login page with JWT authentication",
							}),
						},
					},
				},
			},
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-6",
							Name:      "GitPush",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:   "c-7",
							Name: "GHCreatePR",
							Arguments: mustJSON(t, map[string]interface{}{
								"title": "feat: add login page with JWT authentication",
								"body":  "Implements login endpoint with JWT middleware.",
								"base":  "main",
								"head":  "codebutler/add-login",
							}),
						},
					},
				},
			},
			// Turn 8: Coder sends handoff to Reviewer
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:        "c-8",
							Name:      "SendMessage",
							Arguments: `{"text": "@codebutler.reviewer PR ready for review: https://github.com/org/repo/pull/42"}`,
						},
					},
				},
			},
			// Turn 9: Coder responds with completion
			{
				Message: Message{
					Role:    "assistant",
					Content: "Implementation complete. PR #42 created and sent to Reviewer.",
				},
			},
		},
	}

	coderExecutor := &mockExecutor{
		results: tracker.toolResults(),
		toolDefs: []ToolDefinition{
			{Name: "Read", Description: "Read a file"},
			{Name: "Write", Description: "Write a file"},
			{Name: "Edit", Description: "Edit a file"},
			{Name: "Bash", Description: "Run a shell command"},
			{Name: "Grep", Description: "Search file contents"},
			{Name: "Glob", Description: "Search files by pattern"},
			{Name: "GitCommit", Description: "Commit files"},
			{Name: "GitPush", Description: "Push to remote"},
			{Name: "GHCreatePR", Description: "Create a pull request"},
			{Name: "SendMessage", Description: "Send a Slack message"},
		},
	}

	coderRunner := NewCoderRunner(
		coderProvider,
		coderSender,
		coderExecutor,
		CoderConfig{
			Model:       coderModel,
			MaxTurns:    50,
			WorktreeDir: "/repo/.codebutler/branches/codebutler/add-login",
			BaseBranch:  "main",
			HeadBranch:  "codebutler/add-login",
		},
		"You are the Coder agent. Implement the plan in the worktree.",
	)

	// Run Coder with the PM's plan
	coderResult, err := coderRunner.RunWithPlan(ctx, plan, "C-test", "T-test-001")
	if err != nil {
		t.Fatalf("Coder run failed: %v", err)
	}

	// === Phase 4: Verify the full flow ===

	// Coder should have produced a response
	if coderResult.Response == "" {
		t.Error("Coder should produce a final text response")
	}
	if !strings.Contains(coderResult.Response, "PR") {
		t.Error("Coder response should mention PR creation")
	}

	// Coder should have used git tools
	if coderResult.ToolCalls < 5 {
		t.Errorf("expected at least 5 tool calls (read, write×2, test, commit, push, PR), got %d", coderResult.ToolCalls)
	}

	// Verify SendMessage tool was called (via the mock executor, not captureSender).
	// The mock executor handles SendMessage and returns "message sent".
	// The actual Slack message sending is handled by the tool implementation,
	// not the executor mock. What we verify is that the LLM requested the
	// SendMessage tool call with @codebutler.reviewer in the arguments.
	foundReviewerHandoff := false
	for _, req := range coderProvider.requests {
		for _, m := range req.Messages {
			for _, tc := range m.ToolCalls {
				if tc.Name == "SendMessage" && strings.Contains(tc.Arguments, "@codebutler.reviewer") {
					foundReviewerHandoff = true
				}
			}
		}
	}
	if !foundReviewerHandoff {
		t.Error("Coder should hand off to Reviewer via SendMessage")
	}

	// Verify the entire provider got called correctly
	if pmProvider.calls != len(pmProvider.responses) {
		t.Errorf("PM: expected %d LLM calls, got %d", len(pmProvider.responses), pmProvider.calls)
	}
	if coderProvider.calls != len(coderProvider.responses) {
		t.Errorf("Coder: expected %d LLM calls, got %d", len(coderProvider.responses), coderProvider.calls)
	}

	// Verify Coder's conversation started with the PM's plan as user message
	if len(coderProvider.requests) > 0 {
		firstReq := coderProvider.requests[0]
		foundPlan := false
		for _, m := range firstReq.Messages {
			if m.Role == "user" && strings.Contains(m.Content, "@codebutler.coder") {
				foundPlan = true
				break
			}
		}
		if !foundPlan {
			t.Error("Coder's first request should contain the PM's plan as a user message")
		}
	}

	// Verify tool definitions were passed to the LLM
	if len(coderProvider.requests) > 0 {
		if len(coderProvider.requests[0].Tools) == 0 {
			t.Error("Coder's LLM requests should include tool definitions")
		}
	}
}

func TestE2E_PMClassifiesToBugfix(t *testing.T) {
	ctx := context.Background()

	pmProvider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role:    "assistant",
					Content: "@codebutler.coder Fix the null pointer in auth handler.\n\nChanges:\n- internal/auth/handler.go:42 — add nil check",
				},
			},
		},
	}

	pmExecutor := &mockExecutor{
		toolDefs: []ToolDefinition{
			{Name: "Read", Description: "Read a file"},
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

	_, intent, err := pmRunner.ClassifyAndRun(ctx, Task{
		Messages: []Message{
			{Role: "user", Content: "there's a crash bug in the auth handler, it panics on nil user"},
		},
	})

	if err != nil {
		t.Fatalf("PM run failed: %v", err)
	}

	// "crash" and "bug" should match bugfix workflow
	if intent.Type != IntentWorkflow || intent.Name != "bugfix" {
		t.Errorf("expected bugfix workflow, got %s/%s", intent.Type, intent.Name)
	}
}

func TestE2E_CoderSandboxEnforcement(t *testing.T) {
	// Verify that the Coder's sandbox validator catches dangerous paths
	// in a realistic plan context
	plan := `@codebutler.coder

## Task
Read the server config.

Changes:
- /etc/passwd:1 — read system file
`

	_, refs := ParsePlan(plan)
	validator := NewSandboxValidator("/repo/.codebutler/branches/codebutler/feat")

	for _, ref := range refs {
		if err := validator.ValidatePath(ref.Path); err == nil && strings.HasPrefix(ref.Path, "/") {
			t.Errorf("sandbox should block absolute path outside worktree: %s", ref.Path)
		}
	}
}

func TestE2E_CoderComplexityRouting(t *testing.T) {
	tests := []struct {
		plan           string
		wantComplexity TaskComplexity
		wantModel      string
	}{
		{
			plan:           "Fix a typo in the README",
			wantComplexity: ComplexitySimple,
			wantModel:      "anthropic/claude-sonnet-4-20250514",
		},
		{
			plan:           "Implement JWT authentication with encryption and security hardening",
			wantComplexity: ComplexityComplex,
			wantModel:      "anthropic/claude-opus-4-20250514",
		},
		{
			plan:           "Add a new API endpoint for user profiles",
			wantComplexity: ComplexityMedium,
			wantModel:      "anthropic/claude-sonnet-4-20250514",
		},
	}

	for _, tt := range tests {
		name := tt.plan
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			complexity := ClassifyComplexity(tt.plan)
			if complexity != tt.wantComplexity {
				t.Errorf("complexity: got %s, want %s", complexity, tt.wantComplexity)
			}
			model := ModelForComplexity(complexity, "anthropic/claude-sonnet-4-20250514")
			if model != tt.wantModel {
				t.Errorf("model: got %s, want %s", model, tt.wantModel)
			}
		})
	}
}

func TestE2E_PRDescriptionFromPlan(t *testing.T) {
	// PRDescription receives the plan summary (not the raw delegation message).
	// The Coder extracts the task description before generating the PR.
	planSummary := "Implement login page with JWT authentication."

	files := []string{
		"internal/auth/handler.go",
		"internal/auth/middleware.go",
		"cmd/main.go",
	}

	desc := PRDescription(planSummary, files)

	if !strings.Contains(desc, "login") || !strings.Contains(desc, "JWT") {
		t.Error("PR description should include plan summary")
	}
	if !strings.Contains(desc, "`internal/auth/handler.go`") {
		t.Error("PR description should list changed files")
	}
	if !strings.Contains(desc, "CodeButler") {
		t.Error("PR description should have CodeButler attribution")
	}
}

func TestE2E_DelegationMessageFormat(t *testing.T) {
	msg := DelegationMessage("coder", "Implement login page", "Project uses Go with stdlib HTTP")

	if !strings.Contains(msg, "@codebutler.coder") {
		t.Error("delegation should target coder")
	}
	if !strings.Contains(msg, "## Task") {
		t.Error("delegation should have Task section")
	}
	if !strings.Contains(msg, "Implement login page") {
		t.Error("delegation should include the plan")
	}
	if !strings.Contains(msg, "## Context") {
		t.Error("delegation should have Context section")
	}
}

func TestE2E_CoderRunWithPlan(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "t1", Name: "Read", Arguments: `{"path":"main.go"}`},
					},
				},
			},
			{
				Message: Message{
					Role:    "assistant",
					Content: "Done implementing the feature.",
				},
			},
		},
	}

	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "package main"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"},
		},
	}

	coder := NewCoderRunner(
		provider,
		&discardSender{},
		executor,
		DefaultCoderConfig(),
		"You are the Coder.",
	)

	result, err := coder.RunWithPlan(ctx, "Implement feature X", "C-test", "T-test")
	if err != nil {
		t.Fatalf("RunWithPlan failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected non-empty response")
	}
	if result.TurnsUsed != 2 {
		t.Errorf("expected 2 turns, got %d", result.TurnsUsed)
	}

	// Verify the plan was passed as user message
	if len(provider.requests) > 0 {
		firstReq := provider.requests[0]
		hasPlan := false
		for _, m := range firstReq.Messages {
			if m.Role == "user" && m.Content == "Implement feature X" {
				hasPlan = true
				break
			}
		}
		if !hasPlan {
			t.Error("plan should be in the first user message")
		}
	}
}

// mustJSON marshals v to a JSON string, failing the test if marshaling fails.
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return string(b)
}

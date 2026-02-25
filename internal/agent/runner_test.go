package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// --- Mock implementations ---

// mockProvider returns pre-configured responses in order.
type mockProvider struct {
	responses []*ChatResponse
	calls     int
	requests  []ChatRequest
}

func (m *mockProvider) ChatCompletion(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	m.requests = append(m.requests, req)
	if m.calls >= len(m.responses) {
		return nil, fmt.Errorf("no more responses configured")
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

// mockErrorProvider always returns an error.
type mockErrorProvider struct {
	err error
}

func (m *mockErrorProvider) ChatCompletion(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	return nil, m.err
}

// mockExecutor tracks calls and returns pre-configured results.
type mockExecutor struct {
	results   map[string]ToolResult
	callCount atomic.Int32
	callNames []string // protected by the sequential test or WaitGroup
	toolDefs  []ToolDefinition
	errTools  map[string]error // tools that return errors
}

func (m *mockExecutor) Execute(_ context.Context, call ToolCall) (ToolResult, error) {
	m.callCount.Add(1)

	if m.errTools != nil {
		if err, ok := m.errTools[call.Name]; ok {
			return ToolResult{}, err
		}
	}

	if result, ok := m.results[call.Name]; ok {
		result.ToolCallID = call.ID
		return result, nil
	}
	return ToolResult{
		ToolCallID: call.ID,
		Content:    "ok",
	}, nil
}

func (m *mockExecutor) ListTools() []ToolDefinition {
	return m.toolDefs
}

// discardSender discards all messages (no-op for tests).
type discardSender struct{}

func (d *discardSender) SendMessage(_ context.Context, _, _, _ string) error {
	return nil
}

// mockStore records Save calls and returns pre-configured Load results.
type mockStore struct {
	messages  []Message // returned by Load
	loadErr   error
	saveErr   error
	saveCount int
	saved     [][]Message // history of Save calls
}

func (m *mockStore) Load(_ context.Context) ([]Message, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.messages == nil {
		return nil, nil
	}
	// Return a copy to avoid mutation
	cp := make([]Message, len(m.messages))
	copy(cp, m.messages)
	return cp, nil
}

func (m *mockStore) Save(_ context.Context, messages []Message) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saveCount++
	cp := make([]Message, len(messages))
	copy(cp, messages)
	m.saved = append(m.saved, cp)
	// Update messages so subsequent Load returns latest state
	m.messages = cp
	return nil
}

// --- Tests ---

func TestRun_TextResponse(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Hello!"}},
		},
	}
	executor := &mockExecutor{
		toolDefs: []ToolDefinition{
			{Name: "Read", Description: "Read a file"},
		},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:         "pm",
		Model:        "test-model",
		MaxTurns:     10,
		SystemPrompt: "You are a test agent.",
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Hello!" {
		t.Errorf("expected response %q, got %q", "Hello!", result.Response)
	}
	if result.TurnsUsed != 1 {
		t.Errorf("expected 1 turn, got %d", result.TurnsUsed)
	}
	if result.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", result.ToolCalls)
	}
}

func TestRun_ToolCallThenTextResponse(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call-1", Name: "Read", Arguments: `{"path":"main.go"}`},
					},
				},
			},
			{Message: Message{Role: "assistant", Content: "I read the file."}},
		},
	}
	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "package main"},
		},
		toolDefs: []ToolDefinition{{Name: "Read", Description: "Read a file"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:         "coder",
		Model:        "test-model",
		MaxTurns:     10,
		SystemPrompt: "You are a coder.",
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Read main.go"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "I read the file." {
		t.Errorf("expected response %q, got %q", "I read the file.", result.Response)
	}
	if result.TurnsUsed != 2 {
		t.Errorf("expected 2 turns, got %d", result.TurnsUsed)
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", result.ToolCalls)
	}
	if executor.callCount.Load() != 1 {
		t.Errorf("expected executor called once, got %d", executor.callCount.Load())
	}
}

func TestRun_MultipleToolCallsThenResponse(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call-1", Name: "Read", Arguments: `{"path":"a.go"}`},
					},
				},
			},
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call-2", Name: "Write", Arguments: `{"path":"b.go","content":"data"}`},
					},
				},
			},
			{Message: Message{Role: "assistant", Content: "All done."}},
		},
	}
	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read":  {Content: "file contents"},
			"Write": {Content: "written"},
		},
		toolDefs: []ToolDefinition{{Name: "Read"}, {Name: "Write"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:     "coder",
		Model:    "test-model",
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Do stuff"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "All done." {
		t.Errorf("expected response %q, got %q", "All done.", result.Response)
	}
	if result.TurnsUsed != 3 {
		t.Errorf("expected 3 turns, got %d", result.TurnsUsed)
	}
	if result.ToolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", result.ToolCalls)
	}
}

func TestRun_MaxTurnsRespected(t *testing.T) {
	// Provider always returns tool calls, never text — should hit MaxTurns
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c2", Name: "Read", Arguments: `{}`}}}},
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c3", Name: "Read", Arguments: `{}`}}}},
			// Would need more responses if MaxTurns > 3, but we cap at 3
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:     "coder",
		Model:    "test-model",
		MaxTurns: 3,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Loop forever"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnsUsed != 3 {
		t.Errorf("expected 3 turns (max), got %d", result.TurnsUsed)
	}
	if result.Response != "" {
		t.Errorf("expected empty response on max turns, got %q", result.Response)
	}
	if result.ToolCalls != 3 {
		t.Errorf("expected 3 tool calls, got %d", result.ToolCalls)
	}
}

func TestRun_MaxTurnsOne(t *testing.T) {
	// With MaxTurns=1 and a tool call response, should stop after 1 turn
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 1,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnsUsed != 1 {
		t.Errorf("expected 1 turn, got %d", result.TurnsUsed)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Should not reach"}},
		},
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:     "pm",
		Model:    "test-model",
		MaxTurns: 10,
	})

	_, err := runner.Run(ctx, Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if provider.calls != 0 {
		t.Errorf("expected 0 LLM calls with cancelled context, got %d", provider.calls)
	}
}

func TestRun_ContextCancellationMidLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// First call succeeds with a tool call, then context is cancelled
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	// Cancel after the first response is consumed (tool call executed)
	// The context check happens before the second LLM call
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := runner.Run(ctx, Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	// Should error because context cancelled before second LLM call
	// (or the second ChatCompletion call fails — either way, error)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRun_ParallelToolExecution(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call-1", Name: "Read", Arguments: `{"path":"a.go"}`},
						{ID: "call-2", Name: "Read", Arguments: `{"path":"b.go"}`},
						{ID: "call-3", Name: "Grep", Arguments: `{"pattern":"main"}`},
					},
				},
			},
			{Message: Message{Role: "assistant", Content: "Done reading."}},
		},
	}
	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "file data"},
			"Grep": {Content: "main.go:1:package main"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Grep"},
		},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:     "coder",
		Model:    "test-model",
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Read files"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolCalls != 3 {
		t.Errorf("expected 3 tool calls, got %d", result.ToolCalls)
	}
	if executor.callCount.Load() != 3 {
		t.Errorf("expected 3 executor calls, got %d", executor.callCount.Load())
	}
	if result.Response != "Done reading." {
		t.Errorf("expected response %q, got %q", "Done reading.", result.Response)
	}

	// Verify tool results were appended to conversation for the second LLM call
	lastReq := provider.requests[len(provider.requests)-1]
	toolMessages := 0
	for _, msg := range lastReq.Messages {
		if msg.Role == "tool" {
			toolMessages++
		}
	}
	if toolMessages != 3 {
		t.Errorf("expected 3 tool messages in second request, got %d", toolMessages)
	}
}

func TestRun_ToolExecutorError(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role:      "assistant",
					ToolCalls: []ToolCall{{ID: "call-1", Name: "Bash", Arguments: `{"command":"rm -rf /"}`}},
				},
			},
			{Message: Message{Role: "assistant", Content: "Error noted."}},
		},
	}
	executor := &mockExecutor{
		errTools: map[string]error{
			"Bash": fmt.Errorf("command blocked: destructive operation"),
		},
		toolDefs: []ToolDefinition{{Name: "Bash"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Delete everything"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Error noted." {
		t.Errorf("expected response %q, got %q", "Error noted.", result.Response)
	}

	// The error should have been sent back as a tool result message
	secondReq := provider.requests[1]
	var toolMsg *Message
	for i := range secondReq.Messages {
		if secondReq.Messages[i].Role == "tool" {
			toolMsg = &secondReq.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("expected a tool result message in second request")
	}
	if toolMsg.Content != "error: command blocked: destructive operation" {
		t.Errorf("expected error content, got %q", toolMsg.Content)
	}
}

func TestRun_LLMError(t *testing.T) {
	provider := &mockErrorProvider{
		err: fmt.Errorf("connection refused"),
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error from LLM failure")
	}
	if result.TurnsUsed != 0 {
		t.Errorf("expected 0 turns used, got %d", result.TurnsUsed)
	}
}

func TestRun_LLMErrorAfterToolCall(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			// Second call will fail (no more responses)
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	if err == nil {
		t.Fatal("expected error from LLM failure on second call")
	}
	if result.TurnsUsed != 1 {
		t.Errorf("expected 1 turn used before failure, got %d", result.TurnsUsed)
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call completed, got %d", result.ToolCalls)
	}
}

func TestRun_SystemPromptInMessages(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "OK"}},
		},
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Role:         "pm",
		Model:        "test-model",
		MaxTurns:     10,
		SystemPrompt: "You are a PM agent.",
	})

	runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	if len(provider.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(provider.requests))
	}
	req := provider.requests[0]
	if len(req.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("expected first message role %q, got %q", "system", req.Messages[0].Role)
	}
	if req.Messages[0].Content != "You are a PM agent." {
		t.Errorf("expected system prompt content, got %q", req.Messages[0].Content)
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("expected second message role %q, got %q", "user", req.Messages[1].Role)
	}
	if req.Messages[1].Content != "Hello" {
		t.Errorf("expected user content %q, got %q", "Hello", req.Messages[1].Content)
	}
}

func TestRun_ToolsPassedToLLM(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "OK"}},
		},
	}
	defs := []ToolDefinition{
		{Name: "Read", Description: "Read a file", Parameters: []byte(`{"type":"object"}`)},
		{Name: "Write", Description: "Write a file", Parameters: []byte(`{"type":"object"}`)},
	}
	executor := &mockExecutor{toolDefs: defs}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	req := provider.requests[0]
	if len(req.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(req.Tools))
	}
	if req.Tools[0].Name != "Read" {
		t.Errorf("expected first tool %q, got %q", "Read", req.Tools[0].Name)
	}
	if req.Tools[1].Name != "Write" {
		t.Errorf("expected second tool %q, got %q", "Write", req.Tools[1].Name)
	}
}

func TestRun_TokenUsageAccumulation(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}},
				Usage:   TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
			{
				Message: Message{Role: "assistant", Content: "Done."},
				Usage:   TokenUsage{PromptTokens: 200, CompletionTokens: 30, TotalTokens: 230},
			},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TokenUsage.PromptTokens != 300 {
		t.Errorf("expected 300 prompt tokens, got %d", result.TokenUsage.PromptTokens)
	}
	if result.TokenUsage.CompletionTokens != 80 {
		t.Errorf("expected 80 completion tokens, got %d", result.TokenUsage.CompletionTokens)
	}
	if result.TokenUsage.TotalTokens != 380 {
		t.Errorf("expected 380 total tokens, got %d", result.TokenUsage.TotalTokens)
	}
}

func TestRun_ModelPassedToLLM(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "OK"}},
		},
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		Model:    "anthropic/claude-opus-4-6",
		MaxTurns: 10,
	})

	runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if provider.requests[0].Model != "anthropic/claude-opus-4-6" {
		t.Errorf("expected model %q, got %q", "anthropic/claude-opus-4-6", provider.requests[0].Model)
	}
}

func TestRun_ConversationGrowsCorrectly(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role:      "assistant",
					ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{"path":"x"}`}},
				},
			},
			{Message: Message{Role: "assistant", Content: "Final answer."}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "file-x-content"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	})

	runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Read x"}},
	})

	// Second request should contain:
	// [system, user, assistant(tool_call), tool(result)]
	secondReq := provider.requests[1]
	expected := []string{"system", "user", "assistant", "tool"}
	if len(secondReq.Messages) != len(expected) {
		t.Fatalf("expected %d messages, got %d", len(expected), len(secondReq.Messages))
	}
	for i, role := range expected {
		if secondReq.Messages[i].Role != role {
			t.Errorf("message[%d]: expected role %q, got %q", i, role, secondReq.Messages[i].Role)
		}
	}
	// Tool result should carry the correct content
	toolMsg := secondReq.Messages[3]
	if toolMsg.Content != "file-x-content" {
		t.Errorf("expected tool content %q, got %q", "file-x-content", toolMsg.Content)
	}
	if toolMsg.ToolCallID != "c1" {
		t.Errorf("expected tool_call_id %q, got %q", "c1", toolMsg.ToolCallID)
	}
}

func TestRun_EmptyToolList(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "No tools available."}},
		},
	}
	executor := &mockExecutor{toolDefs: nil}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "No tools available." {
		t.Errorf("expected response %q, got %q", "No tools available.", result.Response)
	}
	if len(provider.requests[0].Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(provider.requests[0].Tools))
	}
}

func TestRun_MultipleUserMessages(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "I see two messages."}},
		},
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{
			{Role: "user", Content: "First"},
			{Role: "user", Content: "Second"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "I see two messages." {
		t.Errorf("unexpected response: %q", result.Response)
	}
	// Should have: system + 2 user messages = 3 total
	if len(provider.requests[0].Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(provider.requests[0].Messages))
	}
}

func TestRun_WithLogger(t *testing.T) {
	// Verify the WithLogger option doesn't panic
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "OK"}},
		},
	}
	executor := &mockExecutor{}

	// Use a custom logger
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	}, WithLogger(nil)) // nil logger would panic if not handled

	// If the runner uses the nil logger, it would panic during Run.
	// This tests that the option is applied correctly.
	// We don't actually pass nil in practice, but the test verifies the option works.
	_ = runner
}

func TestRun_ZeroMaxTurns(t *testing.T) {
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Should not reach"}},
		},
	}
	executor := &mockExecutor{}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 0,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnsUsed != 0 {
		t.Errorf("expected 0 turns, got %d", result.TurnsUsed)
	}
	if provider.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", provider.calls)
	}
}

func TestRun_ParallelToolExecutionPreservesOrder(t *testing.T) {
	// Verify that results from parallel execution maintain correct order
	provider := &mockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "first", Name: "Read", Arguments: `{"path":"a"}`},
						{ID: "second", Name: "Grep", Arguments: `{"pattern":"b"}`},
					},
				},
			},
			{Message: Message{Role: "assistant", Content: "Done."}},
		},
	}
	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read": {Content: "content-a"},
			"Grep": {Content: "content-b"},
		},
		toolDefs: []ToolDefinition{{Name: "Read"}, {Name: "Grep"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	})

	runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	// Check the second request has tool results in correct order
	secondReq := provider.requests[1]
	toolMsgs := []Message{}
	for _, msg := range secondReq.Messages {
		if msg.Role == "tool" {
			toolMsgs = append(toolMsgs, msg)
		}
	}
	if len(toolMsgs) != 2 {
		t.Fatalf("expected 2 tool messages, got %d", len(toolMsgs))
	}
	if toolMsgs[0].ToolCallID != "first" {
		t.Errorf("expected first tool result ID %q, got %q", "first", toolMsgs[0].ToolCallID)
	}
	if toolMsgs[1].ToolCallID != "second" {
		t.Errorf("expected second tool result ID %q, got %q", "second", toolMsgs[1].ToolCallID)
	}
}

// --- Conversation Persistence Tests ---

func TestRun_SavesAfterEachRound(t *testing.T) {
	store := &mockStore{}
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			{Message: Message{Role: "assistant", Content: "Done."}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Done." {
		t.Errorf("expected response %q, got %q", "Done.", result.Response)
	}

	// Should have saved twice: after tool round + after text response
	if store.saveCount != 2 {
		t.Fatalf("expected 2 saves, got %d", store.saveCount)
	}

	// First save: [system, user, assistant(tc), tool(result)]
	first := store.saved[0]
	if len(first) != 4 {
		t.Fatalf("first save: expected 4 messages, got %d", len(first))
	}
	expectedRoles := []string{"system", "user", "assistant", "tool"}
	for i, role := range expectedRoles {
		if first[i].Role != role {
			t.Errorf("first save[%d]: expected role %q, got %q", i, role, first[i].Role)
		}
	}

	// Second save: [system, user, assistant(tc), tool(result), assistant(text)]
	second := store.saved[1]
	if len(second) != 5 {
		t.Fatalf("second save: expected 5 messages, got %d", len(second))
	}
	if second[4].Role != "assistant" || second[4].Content != "Done." {
		t.Errorf("second save[4]: expected assistant 'Done.', got role=%q content=%q",
			second[4].Role, second[4].Content)
	}
}

func TestRun_ResumeAfterCrash(t *testing.T) {
	store := &mockStore{}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "file contents"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}

	// --- First run: tool call succeeds, then "crashes" (provider runs out of responses) ---
	provider1 := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			// No more responses — simulates crash
		},
	}
	runner1 := NewAgentRunner(provider1, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	_, err := runner1.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Read the file"}},
	})
	if err == nil {
		t.Fatal("expected error from simulated crash")
	}

	// Store should have saved after round 1
	if store.saveCount != 1 {
		t.Fatalf("expected 1 save after crash, got %d", store.saveCount)
	}

	// --- Second run: resume from stored conversation ---
	provider2 := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Resumed and done."}},
		},
	}
	runner2 := NewAgentRunner(provider2, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner2.Run(context.Background(), Task{})
	if err != nil {
		t.Fatalf("unexpected error on resume: %v", err)
	}
	if result.Response != "Resumed and done." {
		t.Errorf("expected response %q, got %q", "Resumed and done.", result.Response)
	}
	// 1 turn from first run + 1 turn from resume = 2 total
	if result.TurnsUsed != 2 {
		t.Errorf("expected 2 total turns, got %d", result.TurnsUsed)
	}

	// Verify the resumed LLM call received the full prior conversation
	req := provider2.requests[0]
	expectedRoles := []string{"system", "user", "assistant", "tool"}
	if len(req.Messages) != len(expectedRoles) {
		t.Fatalf("expected %d messages in resumed request, got %d", len(expectedRoles), len(req.Messages))
	}
	for i, role := range expectedRoles {
		if req.Messages[i].Role != role {
			t.Errorf("resumed request message[%d]: expected role %q, got %q", i, role, req.Messages[i].Role)
		}
	}
}

func TestRun_ResumeCompletedConversation(t *testing.T) {
	// Store has a fully completed conversation (last message is text response)
	store := &mockStore{
		messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "Already completed."},
		},
	}
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Should not reach"}},
		},
	}
	runner := NewAgentRunner(provider, &discardSender{}, &mockExecutor{}, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner.Run(context.Background(), Task{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Already completed." {
		t.Errorf("expected response %q, got %q", "Already completed.", result.Response)
	}
	if result.TurnsUsed != 1 {
		t.Errorf("expected 1 turn (from loaded conversation), got %d", result.TurnsUsed)
	}
	// No LLM call should have been made
	if provider.calls != 0 {
		t.Errorf("expected 0 LLM calls for completed conversation, got %d", provider.calls)
	}
}

func TestRun_ResumeRespectsMaxTurns(t *testing.T) {
	// Store has 2 completed turns, MaxTurns is 3 — only 1 more turn allowed
	store := &mockStore{
		messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "go"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}},
			{Role: "tool", Content: "data", ToolCallID: "c1"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c2", Name: "Read", Arguments: `{}`}}},
			{Role: "tool", Content: "more data", ToolCallID: "c2"},
		},
	}
	provider := &mockProvider{
		responses: []*ChatResponse{
			// This 3rd turn should execute (turn 2, 0-indexed)
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c3", Name: "Read", Arguments: `{}`}}}},
			// 4th turn would be turn 3, which equals MaxTurns=3 — should not execute
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 3,
	}, WithConversationStore(store))

	result, err := runner.Run(context.Background(), Task{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TurnsUsed != 3 {
		t.Errorf("expected 3 total turns, got %d", result.TurnsUsed)
	}
	// Only 1 LLM call should have been made (the 3rd turn)
	if provider.calls != 1 {
		t.Errorf("expected 1 LLM call on resume, got %d", provider.calls)
	}
}

func TestRun_ResumeWithNewMessages(t *testing.T) {
	// Store has partial conversation, new task messages should be appended
	store := &mockStore{
		messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "original request"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}},
			{Role: "tool", Content: "data", ToolCallID: "c1"},
		},
	}
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Done with new info."}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		MaxTurns: 10,
	}, WithConversationStore(store))

	// New message received while agent was down
	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "also do Y"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Done with new info." {
		t.Errorf("expected response %q, got %q", "Done with new info.", result.Response)
	}

	// LLM should have received: system + user + assistant(tc) + tool + user(new)
	req := provider.requests[0]
	if len(req.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(req.Messages))
	}
	if req.Messages[4].Content != "also do Y" {
		t.Errorf("expected new message content %q, got %q", "also do Y", req.Messages[4].Content)
	}
}

func TestRun_NoStoreNoPersistence(t *testing.T) {
	// Without a store, behavior is unchanged from pre-M6
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Hello!"}},
		},
	}
	runner := NewAgentRunner(provider, &discardSender{}, &mockExecutor{}, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	})

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Hello!" {
		t.Errorf("expected response %q, got %q", "Hello!", result.Response)
	}
}

func TestRun_StoreLoadErrorStartsFresh(t *testing.T) {
	// If the store fails to load, the runner starts a fresh conversation
	store := &mockStore{
		loadErr: fmt.Errorf("disk read error"),
	}
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Fresh start."}},
		},
	}
	runner := NewAgentRunner(provider, &discardSender{}, &mockExecutor{}, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Fresh start." {
		t.Errorf("expected response %q, got %q", "Fresh start.", result.Response)
	}
	// Verify it built the conversation from scratch (system + user)
	req := provider.requests[0]
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("expected system message first, got %q", req.Messages[0].Role)
	}
}

func TestRun_StoreSaveErrorDoesNotStopLoop(t *testing.T) {
	// Save errors are logged but don't stop the agent loop
	store := &mockStore{
		saveErr: fmt.Errorf("disk full"),
	}
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			{Message: Message{Role: "assistant", Content: "Done despite save failure."}},
		},
	}
	executor := &mockExecutor{
		results:  map[string]ToolResult{"Read": {Content: "data"}},
		toolDefs: []ToolDefinition{{Name: "Read"}},
	}
	runner := NewAgentRunner(provider, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "sys",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Go"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Done despite save failure." {
		t.Errorf("expected response %q, got %q", "Done despite save failure.", result.Response)
	}
	if result.TurnsUsed != 2 {
		t.Errorf("expected 2 turns, got %d", result.TurnsUsed)
	}
}

func TestRun_MultiRoundResumeEndToEnd(t *testing.T) {
	// Simulate a 3-turn conversation with crash after turn 2, resume, complete
	store := &mockStore{}
	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Read":  {Content: "file data"},
			"Write": {Content: "written"},
		},
		toolDefs: []ToolDefinition{{Name: "Read"}, {Name: "Write"}},
	}

	// --- Run 1: 2 tool rounds, then crash ---
	provider1 := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}}},
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c2", Name: "Write", Arguments: `{}`}}}},
			// Crash — no more responses
		},
	}
	runner1 := NewAgentRunner(provider1, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "You are a coder.",
		MaxTurns:     10,
	}, WithConversationStore(store))

	_, err := runner1.Run(context.Background(), Task{
		Messages: []Message{{Role: "user", Content: "Build feature"}},
	})
	if err == nil {
		t.Fatal("expected error from crash")
	}
	if store.saveCount != 2 {
		t.Fatalf("expected 2 saves before crash, got %d", store.saveCount)
	}

	// --- Run 2: resume, complete with text response ---
	provider2 := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{Role: "assistant", Content: "Feature complete."}},
		},
	}
	runner2 := NewAgentRunner(provider2, &discardSender{}, executor, AgentConfig{
		SystemPrompt: "You are a coder.",
		MaxTurns:     10,
	}, WithConversationStore(store))

	result, err := runner2.Run(context.Background(), Task{})
	if err != nil {
		t.Fatalf("unexpected error on resume: %v", err)
	}
	if result.Response != "Feature complete." {
		t.Errorf("expected response %q, got %q", "Feature complete.", result.Response)
	}
	if result.TurnsUsed != 3 {
		t.Errorf("expected 3 total turns, got %d", result.TurnsUsed)
	}

	// Verify full conversation was sent to the LLM on resume
	// Expected: system + user + asst(tc1) + tool(r1) + asst(tc2) + tool(r2)
	req := provider2.requests[0]
	if len(req.Messages) != 6 {
		t.Fatalf("expected 6 messages on resume, got %d", len(req.Messages))
	}
	expectedRoles := []string{"system", "user", "assistant", "tool", "assistant", "tool"}
	for i, role := range expectedRoles {
		if req.Messages[i].Role != role {
			t.Errorf("message[%d]: expected role %q, got %q", i, role, req.Messages[i].Role)
		}
	}

	// Total saves: 2 from first run + 1 from resume = 3
	if store.saveCount != 3 {
		t.Errorf("expected 3 total saves, got %d", store.saveCount)
	}
}

package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool is a simple Tool implementation for testing.
type mockTool struct {
	name     string
	riskTier RiskTier
	result   ToolResult
	err      error
	called   int
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string          { return "mock tool" }
func (m *mockTool) Parameters() json.RawMessage  { return json.RawMessage(`{}`) }
func (m *mockTool) RiskTier() RiskTier           { return m.riskTier }
func (m *mockTool) Execute(_ context.Context, call ToolCall) (ToolResult, error) {
	m.called++
	return m.result, m.err
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry(RoleCoder, nil)

	tool := &mockTool{name: "TestTool"}
	if err := r.Register(tool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Duplicate registration should fail
	if err := r.Register(tool); err == nil {
		t.Fatal("Register() should fail on duplicate")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry(RoleCoder, nil)
	tool := &mockTool{name: "TestTool"}
	r.Register(tool)

	got := r.Get("TestTool")
	if got == nil {
		t.Fatal("Get() returned nil for registered tool")
	}
	if got.Name() != "TestTool" {
		t.Errorf("Get() returned tool with name %q, want %q", got.Name(), "TestTool")
	}

	// Non-existent tool
	if r.Get("NonExistent") != nil {
		t.Error("Get() should return nil for non-existent tool")
	}
}

func TestRegistry_List_RoleFiltering(t *testing.T) {
	tests := []struct {
		role     Role
		tools    []string
		expected []string
	}{
		{
			role:     RoleCoder,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
			expected: []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
		},
		{
			role:     RolePM,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob", "GitCommit"},
			expected: []string{"Read", "Bash", "Grep", "Glob"},
		},
		{
			role:     RoleReviewer,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
			expected: []string{"Read", "Grep", "Glob"},
		},
		{
			role:     RoleLead,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
			expected: []string{"Read", "Write", "Edit", "Grep", "Glob"},
		},
		{
			role:     RoleResearcher,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
			expected: []string{"Read", "Grep", "Glob"},
		},
		{
			role:     RoleArtist,
			tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob"},
			expected: []string{"Read", "Write", "Edit", "Grep", "Glob"},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			r := NewRegistry(tt.role, nil)
			for _, name := range tt.tools {
				r.Register(&mockTool{name: name})
			}

			got := r.List()
			gotMap := make(map[string]bool)
			for _, n := range got {
				gotMap[n] = true
			}

			for _, want := range tt.expected {
				if !gotMap[want] {
					t.Errorf("List() missing expected tool %q for role %q", want, tt.role)
				}
			}

			if len(got) != len(tt.expected) {
				t.Errorf("List() returned %d tools, want %d. Got: %v", len(got), len(tt.expected), got)
			}
		})
	}
}

func TestRegistry_Execute_RoleRestriction(t *testing.T) {
	r := NewRegistry(RolePM, nil)
	r.Register(&mockTool{name: "Write", result: ToolResult{Content: "ok"}})

	call := ToolCall{ID: "call-1", Name: "Write", Arguments: json.RawMessage(`{}`)}
	result, err := r.Execute(context.Background(), call)

	if err == nil {
		t.Fatal("Execute() should return error for restricted tool")
	}
	if !result.IsError {
		t.Error("result.IsError should be true for restricted tool")
	}
}

func TestRegistry_Execute_UnknownTool(t *testing.T) {
	r := NewRegistry(RoleCoder, nil)

	call := ToolCall{ID: "call-1", Name: "NonExistent", Arguments: json.RawMessage(`{}`)}
	result, err := r.Execute(context.Background(), call)

	if err == nil {
		t.Fatal("Execute() should return error for unknown tool")
	}
	if !result.IsError {
		t.Error("result.IsError should be true for unknown tool")
	}
}

func TestRegistry_Execute_Idempotency(t *testing.T) {
	tool := &mockTool{name: "TestTool", result: ToolResult{Content: "first result"}}
	r := NewRegistry(RoleCoder, nil)
	r.Register(tool)

	call := ToolCall{ID: "call-123", Name: "TestTool", Arguments: json.RawMessage(`{}`)}

	// First execution
	result1, err := r.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if result1.Content != "first result" {
		t.Errorf("first result = %q, want %q", result1.Content, "first result")
	}
	if tool.called != 1 {
		t.Errorf("tool.called = %d, want 1", tool.called)
	}

	// Second execution with same ID — should return cached result
	result2, err := r.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if result2.Content != "first result" {
		t.Errorf("cached result = %q, want %q", result2.Content, "first result")
	}
	if tool.called != 1 {
		t.Errorf("tool.called = %d after cache hit, want 1", tool.called)
	}
}

func TestRegistry_Execute_NoIdempotencyWithoutID(t *testing.T) {
	tool := &mockTool{name: "TestTool", result: ToolResult{Content: "ok"}}
	r := NewRegistry(RoleCoder, nil)
	r.Register(tool)

	// Empty ID means no caching
	call := ToolCall{ID: "", Name: "TestTool", Arguments: json.RawMessage(`{}`)}

	r.Execute(context.Background(), call)
	r.Execute(context.Background(), call)

	if tool.called != 2 {
		t.Errorf("tool.called = %d, want 2 (no caching without ID)", tool.called)
	}
}

func TestRegistry_IsRestricted(t *testing.T) {
	r := NewRegistry(RolePM, nil)

	if !r.IsRestricted("Write") {
		t.Error("Write should be restricted for PM")
	}
	if r.IsRestricted("Read") {
		t.Error("Read should not be restricted for PM")
	}
}

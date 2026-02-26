package agent

import (
	"context"
	"log/slog"
	"testing"
)

func TestNeedsCompaction(t *testing.T) {
	tests := []struct {
		name       string
		cfg        CompactionConfig
		totalUsed  int
		wantCompact bool
	}{
		{
			name:        "under threshold",
			cfg:         CompactionConfig{ContextWindowTokens: 100000, Threshold: 0.8},
			totalUsed:   50000,
			wantCompact: false,
		},
		{
			name:        "at threshold",
			cfg:         CompactionConfig{ContextWindowTokens: 100000, Threshold: 0.8},
			totalUsed:   80000,
			wantCompact: true,
		},
		{
			name:        "over threshold",
			cfg:         CompactionConfig{ContextWindowTokens: 100000, Threshold: 0.8},
			totalUsed:   90000,
			wantCompact: true,
		},
		{
			name:        "zero window disables compaction",
			cfg:         CompactionConfig{ContextWindowTokens: 0, Threshold: 0.8},
			totalUsed:   1000000,
			wantCompact: false,
		},
		{
			name:        "custom threshold 0.5",
			cfg:         CompactionConfig{ContextWindowTokens: 100000, Threshold: 0.5},
			totalUsed:   50000,
			wantCompact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsCompaction(tt.cfg, tt.totalUsed)
			if got != tt.wantCompact {
				t.Errorf("NeedsCompaction() = %v, want %v", got, tt.wantCompact)
			}
		})
	}
}

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig(128000)

	if cfg.ContextWindowTokens != 128000 {
		t.Errorf("expected ContextWindowTokens 128000, got %d", cfg.ContextWindowTokens)
	}
	if cfg.Threshold != 0.8 {
		t.Errorf("expected Threshold 0.8, got %f", cfg.Threshold)
	}
	if cfg.RecentKeep != 4 {
		t.Errorf("expected RecentKeep 4, got %d", cfg.RecentKeep)
	}
}

func TestCompactConversation(t *testing.T) {
	// Mock provider that returns a summary
	provider := &mockProvider{
		responses: []*ChatResponse{
			{Message: Message{
				Role:    "assistant",
				Content: "## Progress so far\n- Read 5 files\n- Found the auth module\n- Tests passing",
			}},
		},
	}
	logger := slog.Default()

	messages := []Message{
		{Role: "system", Content: "You are an agent."},
		{Role: "user", Content: "Implement auth"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{"path":"a.go"}`}}},
		{Role: "tool", Content: "package main", ToolCallID: "c1"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c2", Name: "Read", Arguments: `{"path":"b.go"}`}}},
		{Role: "tool", Content: "func foo() {}", ToolCallID: "c2"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c3", Name: "Write", Arguments: `{"path":"c.go"}`}}},
		{Role: "tool", Content: "written", ToolCallID: "c3"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c4", Name: "Read", Arguments: `{"path":"d.go"}`}}},
		{Role: "tool", Content: "package auth", ToolCallID: "c4"},
	}

	compacted, err := CompactConversation(ctx(), provider, "test-model", messages, 2, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: system + summary + recent (last 2 assistant groups)
	// Recent 2 groups: c3+tool, c4+tool = 4 messages
	// Compacted: system + summary + 4 = 6
	if len(compacted) < 3 {
		t.Fatalf("expected at least 3 compacted messages, got %d", len(compacted))
	}

	// First message must be system
	if compacted[0].Role != "system" {
		t.Errorf("expected system message first, got %q", compacted[0].Role)
	}

	// Second message should be the summary
	if compacted[1].Role != "user" {
		t.Errorf("expected summary as user message, got %q", compacted[1].Role)
	}
	if compacted[1].Content == "" {
		t.Error("expected non-empty summary content")
	}

	// Compacted should be shorter than original
	if len(compacted) >= len(messages) {
		t.Errorf("expected compacted (%d) to be shorter than original (%d)", len(compacted), len(messages))
	}
}

func TestCompactConversation_TooFewMessages(t *testing.T) {
	provider := &mockProvider{}
	logger := slog.Default()

	messages := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	}

	compacted, err := CompactConversation(ctx(), provider, "test-model", messages, 4, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return unchanged
	if len(compacted) != len(messages) {
		t.Errorf("expected %d messages (unchanged), got %d", len(messages), len(compacted))
	}
}

func TestCompactConversation_AllRecent(t *testing.T) {
	// When recentKeep covers all assistant groups, the middle is too small
	// to justify compaction — should return original unchanged.
	provider := &mockProvider{}
	logger := slog.Default()

	// Only 1 assistant group — keepRecent=10 covers everything.
	// Middle = [user message only] which is < 2, so no compaction.
	messages := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "Read", Arguments: `{}`}}},
		{Role: "tool", Content: "data", ToolCallID: "c1"},
	}

	compacted, err := CompactConversation(ctx(), provider, "model", messages, 10, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compacted) != len(messages) {
		t.Errorf("expected %d messages (unchanged), got %d", len(messages), len(compacted))
	}
}

func TestFindRecentStart(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		keep     int
		want     int
	}{
		{
			name: "keep last 2 groups",
			messages: []Message{
				{Role: "system"},
				{Role: "user"},
				{Role: "assistant"}, // group 1
				{Role: "tool"},
				{Role: "assistant"}, // group 2
				{Role: "tool"},
				{Role: "assistant"}, // group 3
				{Role: "tool"},
			},
			keep: 2,
			want: 4,
		},
		{
			name: "keep last 1 group",
			messages: []Message{
				{Role: "system"},
				{Role: "user"},
				{Role: "assistant"},
				{Role: "tool"},
				{Role: "assistant"},
				{Role: "tool"},
			},
			keep: 1,
			want: 4,
		},
		{
			name: "keep more than available",
			messages: []Message{
				{Role: "system"},
				{Role: "user"},
				{Role: "assistant"},
				{Role: "tool"},
			},
			keep: 5,
			want: 2,
		},
		{
			name: "assistant with multiple tool results",
			messages: []Message{
				{Role: "system"},
				{Role: "user"},
				{Role: "assistant"}, // group 1 with 2 tools
				{Role: "tool"},
				{Role: "tool"},
				{Role: "assistant"}, // group 2
				{Role: "tool"},
			},
			keep: 1,
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findRecentStart(tt.messages, tt.keep)
			if got != tt.want {
				t.Errorf("findRecentStart() = %d, want %d", got, tt.want)
			}
		})
	}
}

func ctx() context.Context {
	return context.Background()
}

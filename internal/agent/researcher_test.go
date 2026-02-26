package agent

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultResearcherConfig(t *testing.T) {
	cfg := DefaultResearcherConfig()
	if cfg.MaxTurns != 15 {
		t.Errorf("expected 15 max turns, got %d", cfg.MaxTurns)
	}
	if cfg.Model == "" {
		t.Error("expected non-empty model")
	}
}

func TestResearcherRunner_Research(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			// Researcher searches the web
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "rs-1", Name: "WebSearch", Arguments: `{"query":"go rate limiting best practices"}`},
			}}},
			// Researcher reads a source
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "rs-2", Name: "WebFetch", Arguments: `{"url":"https://example.com/rate-limiting"}`},
			}}},
			// Researcher responds with findings
			{Message: Message{Role: "assistant", Content: `Research: Go rate limiting

Findings:
1. Use token bucket algorithm for API rate limiting — https://example.com/rate-limiting
2. golang.org/x/time/rate provides a standard implementation — https://pkg.go.dev/golang.org/x/time/rate

Recommendation: Use golang.org/x/time/rate with a per-IP token bucket.

Persisted: .codebutler/research/go-rate-limiting.md

Sources:
- https://example.com/rate-limiting — comprehensive guide
- https://pkg.go.dev/golang.org/x/time/rate — stdlib docs`}},
		},
	}

	executor := &mockExecutor{
		results: map[string]ToolResult{
			"WebSearch": {Content: `[{"title":"Rate Limiting","url":"https://example.com/rate-limiting","snippet":"Guide"}]`},
			"WebFetch":  {Content: "# Rate Limiting\nUse token bucket..."},
		},
		toolDefs: []ToolDefinition{
			{Name: "WebSearch"}, {Name: "WebFetch"},
			{Name: "Read"}, {Name: "Glob"}, {Name: "Write"},
			{Name: "SendMessage"},
		},
	}

	researcher := NewResearcherRunner(
		provider,
		&discardSender{},
		executor,
		ResearcherConfig{
			Model:       "anthropic/claude-sonnet-4-20250514",
			MaxTurns:    15,
			ResearchDir: ".codebutler/research/",
		},
		"You are the Researcher.",
	)

	result, err := researcher.Research(ctx, "go rate limiting best practices", "coder", "C-test", "T-test")
	if err != nil {
		t.Fatalf("research failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected research response")
	}
	if !strings.Contains(result.Response, "token bucket") {
		t.Error("response should include findings")
	}
}

func TestFormatResearchPrompt(t *testing.T) {
	prompt := FormatResearchPrompt("jwt best practices", "reviewer", ".codebutler/research/")

	if !strings.Contains(prompt, "reviewer") {
		t.Error("missing requester name")
	}
	if !strings.Contains(prompt, "jwt best practices") {
		t.Error("missing query")
	}
	if !strings.Contains(prompt, "check existing research") {
		t.Error("missing existing research check instruction")
	}
	if !strings.Contains(prompt, "WebSearch") {
		t.Error("missing WebSearch instruction")
	}
}

func TestFormatResearchPrompt_NoResearchDir(t *testing.T) {
	prompt := FormatResearchPrompt("test query", "pm", "")
	if strings.Contains(prompt, "check existing research") {
		t.Error("should not mention existing research when no dir configured")
	}
}

func TestFormatResearchFindings(t *testing.T) {
	findings := []ResearchFinding{
		{Key: "Token bucket is the standard approach", Source: "https://example.com/1"},
		{Key: "golang.org/x/time/rate is recommended", Source: "https://example.com/2"},
	}

	text := FormatResearchFindings("Go rate limiting", findings, "Use x/time/rate", ".codebutler/research/go-rate-limiting.md")

	if !strings.Contains(text, "Research: Go rate limiting") {
		t.Error("missing topic")
	}
	if !strings.Contains(text, "Token bucket") {
		t.Error("missing finding 1")
	}
	if !strings.Contains(text, "Use x/time/rate") {
		t.Error("missing recommendation")
	}
	if !strings.Contains(text, ".codebutler/research/go-rate-limiting.md") {
		t.Error("missing persisted path")
	}
}

func TestFormatResearchFindings_NotPersisted(t *testing.T) {
	text := FormatResearchFindings("test", nil, "answer", "")
	if !strings.Contains(text, "not persisted") {
		t.Error("should indicate not persisted when path is empty")
	}
}

func TestParseResearchFindings(t *testing.T) {
	text := `Research: Go rate limiting

Findings:
1. Token bucket algorithm — https://example.com/1
2. x/time/rate package — https://pkg.go.dev/golang.org/x/time/rate

Recommendation: Use x/time/rate.`

	findings := ParseResearchFindings(text)

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	if findings[0].Key != "Token bucket algorithm" {
		t.Errorf("finding 0 key: got %q", findings[0].Key)
	}
	if findings[0].Source != "https://example.com/1" {
		t.Errorf("finding 0 source: got %q", findings[0].Source)
	}
	if findings[1].Key != "x/time/rate package" {
		t.Errorf("finding 1 key: got %q", findings[1].Key)
	}
}

func TestParseResearchFindings_Empty(t *testing.T) {
	findings := ParseResearchFindings("No findings section here.")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestResearchTopicSlug(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"Go Rate Limiting", "go-rate-limiting"},
		{"JWT Best Practices 2024", "jwt-best-practices-2024"},
		{"OWASP Top 10", "owasp-top-10"},
		{"simple", "simple"},
		{"  spaces  and---dashes  ", "spaces-and-dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := ResearchTopicSlug(tt.topic)
			if got != tt.want {
				t.Errorf("ResearchTopicSlug(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}

func TestResearchTopicSlug_LongTopic(t *testing.T) {
	long := strings.Repeat("very long topic name ", 10)
	slug := ResearchTopicSlug(long)
	if len(slug) > 60 {
		t.Errorf("slug should be at most 60 chars, got %d", len(slug))
	}
}

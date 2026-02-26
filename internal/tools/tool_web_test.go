package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- Mock implementations ---

type mockWebSearcher struct {
	results []SearchResult
	err     error
	queries []string
}

func (m *mockWebSearcher) Search(_ context.Context, query string, _ int) ([]SearchResult, error) {
	m.queries = append(m.queries, query)
	return m.results, m.err
}

type mockWebFetcher struct {
	content string
	err     error
	urls    []string
}

func (m *mockWebFetcher) Fetch(_ context.Context, url string) (string, error) {
	m.urls = append(m.urls, url)
	return m.content, m.err
}

// --- WebSearch tests ---

func TestWebSearchTool_Success(t *testing.T) {
	searcher := &mockWebSearcher{
		results: []SearchResult{
			{Title: "Go Rate Limiting", URL: "https://example.com/rate", Snippet: "How to rate limit in Go"},
			{Title: "Token Bucket", URL: "https://example.com/bucket", Snippet: "Token bucket algorithm"},
		},
	}

	tool := NewWebSearchTool(searcher)
	result, err := tool.Execute(context.Background(), ToolCall{
		ID:        "ws-1",
		Arguments: json.RawMessage(`{"query": "go rate limiting best practices"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}

	// Verify JSON output
	var results []SearchResult
	if err := json.Unmarshal([]byte(result.Content), &results); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	tool := NewWebSearchTool(&mockWebSearcher{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"query": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for empty query")
	}
}

func TestWebSearchTool_SearchFails(t *testing.T) {
	searcher := &mockWebSearcher{err: fmt.Errorf("network error")}
	tool := NewWebSearchTool(searcher)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"query": "test"}`),
	})
	if !result.IsError {
		t.Error("expected error when search fails")
	}
}

func TestWebSearchTool_NoResults(t *testing.T) {
	tool := NewWebSearchTool(&mockWebSearcher{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"query": "extremely specific query"}`),
	})
	if result.IsError {
		t.Error("no results should not be an error")
	}
	if !strings.Contains(result.Content, "no results") {
		t.Error("should indicate no results found")
	}
}

func TestWebSearchTool_DefaultMaxResults(t *testing.T) {
	searcher := &mockWebSearcher{
		results: []SearchResult{{Title: "result", URL: "url", Snippet: "s"}},
	}
	tool := NewWebSearchTool(searcher)
	tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"query": "test"}`),
	})
	// Default max_results should be 5 (handled by searcher, not tool)
	if len(searcher.queries) != 1 || searcher.queries[0] != "test" {
		t.Error("query should be passed to searcher")
	}
}

func TestWebSearchTool_Properties(t *testing.T) {
	tool := NewWebSearchTool(nil)
	if tool.Name() != "WebSearch" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != Read {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

// --- WebFetch tests ---

func TestWebFetchTool_Success(t *testing.T) {
	fetcher := &mockWebFetcher{
		content: "# Rate Limiting in Go\n\nUse token bucket algorithm...",
	}

	tool := NewWebFetchTool(fetcher)
	result, err := tool.Execute(context.Background(), ToolCall{
		ID:        "wf-1",
		Arguments: json.RawMessage(`{"url": "https://example.com/rate-limiting"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "token bucket") {
		t.Error("content should include fetched page text")
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	tool := NewWebFetchTool(&mockWebFetcher{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"url": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for empty URL")
	}
}

func TestWebFetchTool_FetchFails(t *testing.T) {
	fetcher := &mockWebFetcher{err: fmt.Errorf("404 not found")}
	tool := NewWebFetchTool(fetcher)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"url": "https://example.com/missing"}`),
	})
	if !result.IsError {
		t.Error("expected error when fetch fails")
	}
}

func TestWebFetchTool_Truncation(t *testing.T) {
	// Create content larger than 50000 chars
	largeContent := strings.Repeat("x", 60000)
	fetcher := &mockWebFetcher{content: largeContent}
	tool := NewWebFetchTool(fetcher)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"url": "https://example.com/large"}`),
	})
	if result.IsError {
		t.Error("large content should not cause an error")
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Error("large content should be truncated with notice")
	}
	if len(result.Content) > 60000 {
		t.Error("content should be truncated to ~50000 chars")
	}
}

func TestWebFetchTool_Properties(t *testing.T) {
	tool := NewWebFetchTool(nil)
	if tool.Name() != "WebFetch" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != Read {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

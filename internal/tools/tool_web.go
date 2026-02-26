package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// --- WebSearch Tool ---

// WebSearcher executes web searches.
type WebSearcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// SearchResult represents a single web search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WebSearchTool allows agents to search the web.
type WebSearchTool struct {
	searcher WebSearcher
}

// NewWebSearchTool creates a new WebSearch tool.
func NewWebSearchTool(searcher WebSearcher) *WebSearchTool {
	return &WebSearchTool{searcher: searcher}
}

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Description() string {
	return "Search the web for information. Returns a list of relevant results with titles, URLs, and snippets."
}

func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"max_results": {
				"type": "integer",
				"description": "Maximum number of results to return (default 5)"
			}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) RiskTier() RiskTier { return Read }

func (t *WebSearchTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("invalid arguments: %s", err),
			IsError:    true,
		}, nil
	}

	if args.Query == "" {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    "query is required",
			IsError:    true,
		}, nil
	}

	if args.MaxResults <= 0 {
		args.MaxResults = 5
	}

	results, err := t.searcher.Search(ctx, args.Query, args.MaxResults)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("search failed: %s", err),
			IsError:    true,
		}, nil
	}

	if len(results) == 0 {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    "no results found",
		}, nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return ToolResult{
		ToolCallID: call.ID,
		Content:    string(output),
	}, nil
}

// --- WebFetch Tool ---

// WebFetcher fetches content from URLs.
type WebFetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}

// WebFetchTool allows agents to read web page content.
type WebFetchTool struct {
	fetcher WebFetcher
}

// NewWebFetchTool creates a new WebFetch tool.
func NewWebFetchTool(fetcher WebFetcher) *WebFetchTool {
	return &WebFetchTool{fetcher: fetcher}
}

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch and read the content of a web page. Returns the page content as text."
}

func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch content from"
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) RiskTier() RiskTier { return Read }

func (t *WebFetchTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("invalid arguments: %s", err),
			IsError:    true,
		}, nil
	}

	if args.URL == "" {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    "url is required",
			IsError:    true,
		}, nil
	}

	content, err := t.fetcher.Fetch(ctx, args.URL)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("fetch failed: %s", err),
			IsError:    true,
		}, nil
	}

	// Truncate very large pages to prevent context window issues
	const maxContentLen = 50000
	if len(content) > maxContentLen {
		content = content[:maxContentLen] + "\n\n... (content truncated)"
	}

	return ToolResult{
		ToolCallID: call.ID,
		Content:    content,
	}, nil
}

// Package agent implements the core agent loop: prompt → LLM → tool calls → execute → repeat.
// It is provider-agnostic and communicates through interfaces (LLMProvider, ToolExecutor,
// MessageSender), making it independently testable and extractable.
package agent

import "encoding/json"

// Message represents a conversation message in the agent loop.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResult represents the output of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatRequest is a request to the LLM provider.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
}

// ChatResponse is the LLM's response.
type ChatResponse struct {
	Message Message    `json:"message"`
	Usage   TokenUsage `json:"usage"`
}

// TokenUsage tracks token consumption for a single LLM call.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Task represents work for the agent to perform.
type Task struct {
	Messages []Message // Messages to process (user input, agent mentions, etc.)
	Channel  string    // Communication channel ID (e.g., Slack channel)
	Thread   string    // Thread ID for threaded conversations
}

// Result represents the outcome of an agent run.
type Result struct {
	Response      string     // Final text response (empty if max turns reached)
	TurnsUsed     int        // Number of LLM calls made
	TokenUsage    TokenUsage // Cumulative token usage across all turns
	ToolCalls     int        // Total number of tool calls executed
	LoopsDetected int        // Number of stuck conditions detected during the run
	Escalated     bool       // True if the agent escalated (all escape strategies exhausted)
}

// AgentConfig configures an agent runner instance.
type AgentConfig struct {
	Role         string // Agent role (pm, coder, reviewer, etc.)
	Model        string // LLM model ID for OpenRouter
	MaxTurns     int    // Maximum LLM calls per activation
	SystemPrompt string // Pre-built system prompt
}

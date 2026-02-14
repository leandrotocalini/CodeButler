// Package models defines the role interfaces for CodeButler v2.
//
// Three roles:
//   - ProductManager: conversation, planning, triage (cheap models: Kimi, GPT-4o-mini)
//   - Artist: image generation/editing (OpenAI gpt-image-1)
//   - Coder: code execution (Claude CLI)
package models

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// ProductManager — the brain ($0.001/call)
// ---------------------------------------------------------------------------

// ProductManager handles conversation, planning, memory, and triage.
// Implementations: Kimi (moonshot-v1-8k), GPT-4o-mini, DeepSeek, etc.
type ProductManager interface {
	// Chat sends messages and returns the response text.
	Chat(ctx context.Context, system string, messages []Message) (string, error)

	// ChatJSON sends messages and parses the response as JSON into out.
	ChatJSON(ctx context.Context, system string, messages []Message, out interface{}) error

	// ChatWithTools runs a tool-calling loop: the PM can autonomously call
	// tools (read files, grep, git log…) until it produces a final response.
	ChatWithTools(ctx context.Context, system string, messages []Message, tools []Tool) (string, error)

	// Name returns the provider:model identifier for logging.
	Name() string
}

// ---------------------------------------------------------------------------
// Artist — image generation ($0.01–0.10/call)
// ---------------------------------------------------------------------------

// Artist handles image generation and editing.
type Artist interface {
	Generate(ctx context.Context, req ImageGenRequest) (*ImageResult, error)
	Edit(ctx context.Context, req ImageEditRequest) (*ImageResult, error)
	Name() string
}

// ---------------------------------------------------------------------------
// Coder — code execution ($0.10–1.00/call)
// ---------------------------------------------------------------------------

// Coder writes code, runs tests, creates PRs.
type Coder interface {
	Run(ctx context.Context, req CoderRequest) (*CoderResult, error)
	Resume(ctx context.Context, sessionID string, message string) (*CoderResult, error)
	Name() string
}

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

// Message represents a chat message in a conversation.
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
	Image   []byte // optional attached image
}

// Tool defines a function-calling tool available to the ProductManager.
type Tool struct {
	Name        string                 // e.g. "ReadFile", "Grep"
	Description string                 // shown to the LLM
	Parameters  map[string]interface{} // JSON Schema object
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string // provider-assigned call ID
	Name      string // tool name
	Arguments string // raw JSON arguments
}

// ToolResult is the output of executing a tool call.
type ToolResult struct {
	CallID  string // matches ToolCall.ID
	Content string // text result (file contents, grep output, etc.)
	IsError bool
}

// ImageGenRequest for Artist.Generate.
type ImageGenRequest struct {
	Prompt string
	Size   string // "1024x1024", "512x512", etc.
}

// ImageEditRequest for Artist.Edit.
type ImageEditRequest struct {
	Prompt     string
	InputImage []byte
	Size       string
}

// ImageResult from Artist operations.
type ImageResult struct {
	Data      []byte // PNG image data
	LocalPath string // saved to .codebutler/images/<hash>.png
}

// CoderRequest for Coder.Run.
type CoderRequest struct {
	Prompt     string
	WorkDir    string
	MaxTurns   int
	Timeout    time.Duration
	Permission string // "bypassPermissions", etc.
}

// CoderResult from Coder operations.
type CoderResult struct {
	Response  string
	SessionID string
	IsError   bool
	CostUSD   float64
	NumTurns  int
}

package agent

import "context"

// LLMProvider makes chat completion calls to a language model.
// The OpenRouter client satisfies this interface via an adapter.
type LLMProvider interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// ToolExecutor executes tool calls and lists available tools.
// The tools.Registry satisfies this interface via an adapter.
type ToolExecutor interface {
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
	ListTools() []ToolDefinition
}

// MessageSender sends messages to a communication channel (e.g., Slack).
type MessageSender interface {
	SendMessage(ctx context.Context, channel, thread, text string) error
}

// ConversationStore persists agent conversations for crash recovery.
// Each agent maintains its own conversation per thread, stored as a JSON
// array of messages. The conversation package provides a file-based
// implementation with crash-safe writes (write temp + rename).
type ConversationStore interface {
	// Load reads the persisted conversation. Returns nil, nil if no
	// conversation exists yet (first activation).
	Load(ctx context.Context) ([]Message, error)

	// Save writes the full conversation to persistent storage. Must be
	// crash-safe: write to a temp file, then rename.
	Save(ctx context.Context, messages []Message) error
}

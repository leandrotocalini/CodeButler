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

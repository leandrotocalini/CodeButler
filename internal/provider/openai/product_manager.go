package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leandrotocalini/CodeButler/internal/models"
	"github.com/leandrotocalini/CodeButler/internal/tools"
)

// ProductManagerAdapter adapts the shared OpenAI Client to the
// models.ProductManager interface. Works with any OpenAI-compatible
// provider: Kimi, GPT-4o-mini, DeepSeek, etc.
type ProductManagerAdapter struct {
	client  *Client
	model   string
	repoDir string // for tool execution
}

// NewProductManager creates a ProductManager backed by an OpenAI-compatible API.
// repoDir is the repository root where tools will execute (read files, grep, etc).
func NewProductManager(client *Client, model string, repoDir string) models.ProductManager {
	return &ProductManagerAdapter{
		client:  client,
		model:   model,
		repoDir: repoDir,
	}
}

// Chat sends a simple message without tools.
func (a *ProductManagerAdapter) Chat(ctx context.Context, system string, msgs []models.Message) (string, error) {
	messages := a.buildMessages(system, msgs)

	resp, err := a.client.ChatCompletion(ctx, &ChatRequest{
		Model:    a.model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// ChatJSON sends a message and parses the JSON response into out.
func (a *ProductManagerAdapter) ChatJSON(ctx context.Context, system string, msgs []models.Message, out interface{}) error {
	text, err := a.Chat(ctx, system, msgs)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w\nraw: %s", err, text)
	}
	return nil
}

// ChatWithTools runs the tool-calling loop, letting the PM autonomously
// explore the codebase using ReadFile, Grep, ListFiles, GitLog, GitDiff.
func (a *ProductManagerAdapter) ChatWithTools(ctx context.Context, system string, msgs []models.Message, toolDefs []models.Tool) (string, error) {
	executor := tools.NewExecutor(a.repoDir)

	chatFn := func(ctx context.Context, sys string, loopMsgs []tools.LoopMessage, defs []tools.ToolDef) (*tools.LoopResponse, error) {
		return a.chatCompletion(ctx, sys, loopMsgs, defs)
	}

	return tools.RunLoop(ctx, chatFn, executor, system, msgs, toolDefs, tools.MaxToolIterations)
}

// Name returns the provider:model identifier.
func (a *ProductManagerAdapter) Name() string {
	return "openai:" + a.model
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// buildMessages converts models.Message to OpenAI ChatMessage format,
// prepending the system message.
func (a *ProductManagerAdapter) buildMessages(system string, msgs []models.Message) []ChatMessage {
	out := make([]ChatMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, ChatMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		out = append(out, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// chatCompletion bridges the tools.ChatFunc signature to our OpenAI client.
func (a *ProductManagerAdapter) chatCompletion(ctx context.Context, system string,
	loopMsgs []tools.LoopMessage, defs []tools.ToolDef) (*tools.LoopResponse, error) {

	// Build messages
	messages := make([]ChatMessage, 0, len(loopMsgs)+1)
	if system != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: system})
	}
	for _, m := range loopMsgs {
		msg := ChatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		// Convert tool calls
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: ToolCallFunc{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		messages = append(messages, msg)
	}

	// Convert tool defs
	var apiTools []ToolDef
	for _, d := range defs {
		apiTools = append(apiTools, ToolDef{
			Type: d.Type,
			Function: ToolDefFunc{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		})
	}

	resp, err := a.client.ChatCompletion(ctx, &ChatRequest{
		Model:    a.model,
		Messages: messages,
		Tools:    apiTools,
	})
	if err != nil {
		return nil, err
	}

	choice := resp.Choices[0]

	// Check if the model wants to call tools
	if len(choice.Message.ToolCalls) > 0 {
		calls := make([]models.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			calls[i] = models.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
		return &tools.LoopResponse{
			Content:      choice.Message.Content,
			ToolCalls:    calls,
			FinishReason: "tool_calls",
		}, nil
	}

	return &tools.LoopResponse{
		Content:      choice.Message.Content,
		FinishReason: "stop",
	}, nil
}

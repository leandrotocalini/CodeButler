package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leandrotocalini/CodeButler/internal/models"
)

// MaxToolIterations is the default limit on tool-calling round-trips.
const MaxToolIterations = 15

// ChatFunc is the signature for a single LLM chat completion call that
// supports function calling. It returns either a text response (if the
// model is done) or a list of tool calls to execute.
//
// This abstraction lets the loop work with any OpenAI-compatible API
// without depending on a specific HTTP client implementation.
type ChatFunc func(ctx context.Context, system string, messages []LoopMessage, toolDefs []ToolDef) (*LoopResponse, error)

// LoopMessage extends models.Message with tool-call support.
type LoopMessage struct {
	Role       string           `json:"role"` // "user", "assistant", "tool"
	Content    string           `json:"content,omitempty"`
	ToolCalls  []models.ToolCall `json:"tool_calls,omitempty"`  // assistant requesting tools
	ToolCallID string           `json:"tool_call_id,omitempty"` // tool result referencing a call
}

// ToolDef is the OpenAI function-calling tool definition format.
type ToolDef struct {
	Type     string      `json:"type"` // always "function"
	Function ToolDefFunc `json:"function"`
}

// ToolDefFunc describes a function for function-calling.
type ToolDefFunc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// LoopResponse is what ChatFunc returns from one completion call.
type LoopResponse struct {
	Content    string            // final text (when FinishReason == "stop")
	ToolCalls  []models.ToolCall // tool calls to execute (when FinishReason == "tool_calls")
	FinishReason string          // "stop" or "tool_calls"
}

// ToolDefsFromModels converts models.Tool slice to the OpenAI format.
func ToolDefsFromModels(tools []models.Tool) []ToolDef {
	defs := make([]ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToolDef{
			Type: "function",
			Function: ToolDefFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return defs
}

// RunLoop executes the tool-calling loop:
//  1. Call the LLM with messages + tool definitions
//  2. If the LLM returns tool calls, execute them and feed results back
//  3. Repeat until the LLM produces a final text response or maxIter is hit
//
// The executor handles the actual tool implementations (ReadFile, Grep, etc).
func RunLoop(ctx context.Context, chatFn ChatFunc, executor *Executor,
	system string, userMessages []models.Message, tools []models.Tool, maxIter int) (string, error) {

	if maxIter <= 0 {
		maxIter = MaxToolIterations
	}

	toolDefs := ToolDefsFromModels(tools)

	// Convert initial messages to loop format
	messages := make([]LoopMessage, len(userMessages))
	for i, m := range userMessages {
		messages[i] = LoopMessage{Role: m.Role, Content: m.Content}
	}

	for iter := 0; iter < maxIter; iter++ {
		resp, err := chatFn(ctx, system, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("chat completion failed (iter %d): %w", iter, err)
		}

		// Model produced a final response — we're done
		if resp.FinishReason == "stop" || len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, LoopMessage{
			Role:      "assistant",
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call and add results
		for _, tc := range resp.ToolCalls {
			result := executor.Execute(ctx, tc)

			content := result.Content
			if result.IsError {
				content = "ERROR: " + content
			}
			// Truncate very long results to avoid blowing up context
			if len(content) > 8000 {
				content = content[:8000] + "\n... (truncated)"
			}

			messages = append(messages, LoopMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: tc.ID,
			})
		}
	}

	// Exhausted iterations — do one final call without tools to force a response
	resp, err := chatFn(ctx, system, messages, nil)
	if err != nil {
		return "", fmt.Errorf("final chat call failed: %w", err)
	}
	return resp.Content, nil
}

// MessagesToJSON is a helper to marshal loop messages for debugging.
func MessagesToJSON(messages []LoopMessage) string {
	b, _ := json.MarshalIndent(messages, "", "  ")
	return string(b)
}

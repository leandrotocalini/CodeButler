package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// MessageSender sends messages to a communication channel.
type MessageSender interface {
	SendMessage(ctx context.Context, channel, threadTS, text string) error
}

// SendMessageTool sends a message to a Slack channel/thread.
type SendMessageTool struct {
	sender    MessageSender
	channelID string
	threadTS  string
}

// NewSendMessageTool creates a SendMessage tool bound to a specific thread.
func NewSendMessageTool(sender MessageSender, channelID, threadTS string) *SendMessageTool {
	return &SendMessageTool{
		sender:    sender,
		channelID: channelID,
		threadTS:  threadTS,
	}
}

func (t *SendMessageTool) Name() string        { return "SendMessage" }
func (t *SendMessageTool) Description() string  {
	return "Send a message to the Slack thread. Use this to @mention other agents or communicate with the user."
}
func (t *SendMessageTool) RiskTier() RiskTier   { return WriteVisible }
func (t *SendMessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {
				"type": "string",
				"description": "The message text to send. Use @codebutler.<role> to mention another agent."
			}
		},
		"required": ["text"]
	}`)
}

func (t *SendMessageTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Text == "" {
		return ToolResult{Content: "text is required", IsError: true}, nil
	}

	if err := t.sender.SendMessage(ctx, t.channelID, t.threadTS, args.Text); err != nil {
		return ToolResult{Content: fmt.Sprintf("failed to send message: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: "Message sent."}, nil
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type mockMessageSender struct {
	sent []string
	err  error
}

func (m *mockMessageSender) SendMessage(_ context.Context, _, _, text string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, text)
	return nil
}

func TestSendMessageTool_Success(t *testing.T) {
	sender := &mockMessageSender{}
	tool := NewSendMessageTool(sender, "C123", "T456")

	result, err := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"text": "hello @codebutler.coder"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if len(sender.sent) != 1 || sender.sent[0] != "hello @codebutler.coder" {
		t.Errorf("unexpected sent messages: %v", sender.sent)
	}
}

func TestSendMessageTool_EmptyText(t *testing.T) {
	sender := &mockMessageSender{}
	tool := NewSendMessageTool(sender, "C123", "T456")

	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"text": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for empty text")
	}
}

func TestSendMessageTool_SendFails(t *testing.T) {
	sender := &mockMessageSender{err: fmt.Errorf("slack error")}
	tool := NewSendMessageTool(sender, "C123", "T456")

	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"text": "hello"}`),
	})
	if !result.IsError {
		t.Error("expected error when send fails")
	}
}

func TestSendMessageTool_Properties(t *testing.T) {
	tool := NewSendMessageTool(nil, "", "")
	if tool.Name() != "SendMessage" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != WriteVisible {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- Mock implementations ---

type mockImageGenerator struct {
	url string
	err error
}

func (m *mockImageGenerator) GenerateImage(_ context.Context, _, _ string) (string, error) {
	return m.url, m.err
}

type mockImageEditor struct {
	url string
	err error
}

func (m *mockImageEditor) EditImage(_ context.Context, _, _, _ string) (string, error) {
	return m.url, m.err
}

// --- GenerateImage tests ---

func TestGenerateImageTool_Success(t *testing.T) {
	gen := &mockImageGenerator{url: "https://example.com/generated.png"}
	tool := NewGenerateImageTool(gen)

	result, err := tool.Execute(context.Background(), ToolCall{
		ID:        "gi-1",
		Arguments: json.RawMessage(`{"prompt": "a login page mockup"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "generated.png") {
		t.Error("result should contain the generated image URL")
	}
}

func TestGenerateImageTool_EmptyPrompt(t *testing.T) {
	tool := NewGenerateImageTool(&mockImageGenerator{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"prompt": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for empty prompt")
	}
}

func TestGenerateImageTool_GenerationFails(t *testing.T) {
	gen := &mockImageGenerator{err: fmt.Errorf("quota exceeded")}
	tool := NewGenerateImageTool(gen)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"prompt": "test"}`),
	})
	if !result.IsError {
		t.Error("expected error when generation fails")
	}
}

func TestGenerateImageTool_Properties(t *testing.T) {
	tool := NewGenerateImageTool(nil)
	if tool.Name() != "GenerateImage" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != WriteLocal {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

// --- EditImage tests ---

func TestEditImageTool_Success(t *testing.T) {
	editor := &mockImageEditor{url: "https://example.com/edited.png"}
	tool := NewEditImageTool(editor)

	result, err := tool.Execute(context.Background(), ToolCall{
		ID:        "ei-1",
		Arguments: json.RawMessage(`{"image_path": ".codebutler/images/mockup.png", "prompt": "add a header"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "edited.png") {
		t.Error("result should contain the edited image URL")
	}
}

func TestEditImageTool_MissingFields(t *testing.T) {
	tool := NewEditImageTool(&mockImageEditor{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"image_path": "", "prompt": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for missing fields")
	}
}

func TestEditImageTool_EditFails(t *testing.T) {
	editor := &mockImageEditor{err: fmt.Errorf("model error")}
	tool := NewEditImageTool(editor)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"image_path": "img.png", "prompt": "edit"}`),
	})
	if !result.IsError {
		t.Error("expected error when edit fails")
	}
}

func TestEditImageTool_Properties(t *testing.T) {
	tool := NewEditImageTool(nil)
	if tool.Name() != "EditImage" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != WriteLocal {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// --- Mock implementations ---

type mockGitCommitter struct {
	files   []string
	message string
	err     error
}

func (m *mockGitCommitter) Commit(_ context.Context, files []string, message string) error {
	m.files = files
	m.message = message
	return m.err
}

type mockGitPusher struct {
	pushed bool
	err    error
}

func (m *mockGitPusher) Push(_ context.Context) error {
	m.pushed = true
	return m.err
}

type mockPRCreator struct {
	title, body, base, head string
	url                     string
	err                     error
}

func (m *mockPRCreator) CreatePR(_ context.Context, title, body, base, head string) (string, error) {
	m.title = title
	m.body = body
	m.base = base
	m.head = head
	if m.err != nil {
		return "", m.err
	}
	return m.url, nil
}

// --- GitCommit tests ---

func TestGitCommitTool_Success(t *testing.T) {
	git := &mockGitCommitter{}
	tool := NewGitCommitTool(git)

	result, err := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"files": ["main.go", "util.go"], "message": "add feature"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if len(git.files) != 2 {
		t.Errorf("expected 2 files, got %d", len(git.files))
	}
	if git.message != "add feature" {
		t.Errorf("message: got %q", git.message)
	}
}

func TestGitCommitTool_NoFiles(t *testing.T) {
	tool := NewGitCommitTool(&mockGitCommitter{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"files": [], "message": "empty"}`),
	})
	if !result.IsError {
		t.Error("expected error for empty files")
	}
}

func TestGitCommitTool_NoMessage(t *testing.T) {
	tool := NewGitCommitTool(&mockGitCommitter{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"files": ["f.go"], "message": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for empty message")
	}
}

func TestGitCommitTool_CommitFails(t *testing.T) {
	git := &mockGitCommitter{err: fmt.Errorf("git error")}
	tool := NewGitCommitTool(git)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"files": ["f.go"], "message": "msg"}`),
	})
	if !result.IsError {
		t.Error("expected error when commit fails")
	}
}

func TestGitCommitTool_Properties(t *testing.T) {
	tool := NewGitCommitTool(nil)
	if tool.Name() != "GitCommit" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != WriteVisible {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

// --- GitPush tests ---

func TestGitPushTool_Success(t *testing.T) {
	git := &mockGitPusher{}
	tool := NewGitPushTool(git)

	result, err := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !git.pushed {
		t.Error("expected push to be called")
	}
}

func TestGitPushTool_Fails(t *testing.T) {
	git := &mockGitPusher{err: fmt.Errorf("push error")}
	tool := NewGitPushTool(git)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{}`),
	})
	if !result.IsError {
		t.Error("expected error when push fails")
	}
}

func TestGitPushTool_Properties(t *testing.T) {
	tool := NewGitPushTool(nil)
	if tool.Name() != "GitPush" {
		t.Errorf("name: got %q", tool.Name())
	}
}

// --- GHCreatePR tests ---

func TestGHCreatePRTool_Success(t *testing.T) {
	pr := &mockPRCreator{url: "https://github.com/org/repo/pull/42"}
	tool := NewGHCreatePRTool(pr)

	result, err := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"title": "feat: add login", "body": "description", "base": "main", "head": "codebutler/login"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if pr.title != "feat: add login" {
		t.Errorf("title: got %q", pr.title)
	}
}

func TestGHCreatePRTool_MissingFields(t *testing.T) {
	tool := NewGHCreatePRTool(&mockPRCreator{})
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"title": "", "body": "", "base": "", "head": ""}`),
	})
	if !result.IsError {
		t.Error("expected error for missing fields")
	}
}

func TestGHCreatePRTool_CreateFails(t *testing.T) {
	pr := &mockPRCreator{err: fmt.Errorf("auth error")}
	tool := NewGHCreatePRTool(pr)
	result, _ := tool.Execute(context.Background(), ToolCall{
		Arguments: json.RawMessage(`{"title": "feat", "body": "b", "base": "main", "head": "feat"}`),
	})
	if !result.IsError {
		t.Error("expected error when create fails")
	}
}

func TestGHCreatePRTool_Properties(t *testing.T) {
	tool := NewGHCreatePRTool(nil)
	if tool.Name() != "GHCreatePR" {
		t.Errorf("name: got %q", tool.Name())
	}
	if tool.RiskTier() != WriteVisible {
		t.Errorf("risk tier: got %v", tool.RiskTier())
	}
}

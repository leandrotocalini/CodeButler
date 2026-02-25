package conversation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leandrotocalini/codebutler/internal/agent"
)

func TestFileStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversations", "coder.json")
	store := NewFileStore(path)

	messages := []agent.Message{
		{Role: "system", Content: "You are a coder."},
		{Role: "user", Content: "Write hello world"},
		{Role: "assistant", Content: "Here is the code."},
	}

	ctx := context.Background()

	if err := store.Save(ctx, messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(loaded))
	}
	for i, msg := range loaded {
		if msg.Role != messages[i].Role {
			t.Errorf("message[%d].Role: expected %q, got %q", i, messages[i].Role, msg.Role)
		}
		if msg.Content != messages[i].Content {
			t.Errorf("message[%d].Content: expected %q, got %q", i, messages[i].Content, msg.Content)
		}
	}
}

func TestFileStore_SaveWithToolCalls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)

	messages := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "read file"},
		{
			Role: "assistant",
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "Read", Arguments: `{"path":"main.go"}`},
			},
		},
		{Role: "tool", Content: "package main", ToolCallID: "call-1"},
		{Role: "assistant", Content: "I read the file."},
	}

	ctx := context.Background()

	if err := store.Save(ctx, messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(loaded))
	}

	// Verify tool call preserved
	if len(loaded[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(loaded[2].ToolCalls))
	}
	tc := loaded[2].ToolCalls[0]
	if tc.ID != "call-1" || tc.Name != "Read" {
		t.Errorf("tool call mismatch: got ID=%q Name=%q", tc.ID, tc.Name)
	}

	// Verify tool result preserved
	if loaded[3].ToolCallID != "call-1" {
		t.Errorf("expected tool_call_id %q, got %q", "call-1", loaded[3].ToolCallID)
	}
}

func TestFileStore_LoadNonExistentFile(t *testing.T) {
	store := NewFileStore("/nonexistent/path/conv.json")

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for nonexistent file, got %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil messages for nonexistent file, got %d messages", len(loaded))
	}
}

func TestFileStore_LoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("create empty file: %v", err)
	}

	store := NewFileStore(path)
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for empty file, got %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil messages for empty file, got %d messages", len(loaded))
	}
}

func TestFileStore_LoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	store := NewFileStore(path)
	_, err := store.Load(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFileStore_CrashSafeWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)
	ctx := context.Background()

	// Save initial conversation
	initial := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}
	if err := store.Save(ctx, initial); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Save updated conversation
	updated := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	if err := store.Save(ctx, updated); err != nil {
		t.Fatalf("updated save: %v", err)
	}

	// Verify the temp file was cleaned up (renamed to target)
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}

	// Verify final content
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}
	if loaded[2].Content != "hi there" {
		t.Errorf("expected last message %q, got %q", "hi there", loaded[2].Content)
	}
}

func TestFileStore_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "dir", "conv.json")
	store := NewFileStore(path)

	messages := []agent.Message{{Role: "user", Content: "test"}}
	if err := store.Save(context.Background(), messages); err != nil {
		t.Fatalf("Save with directory creation failed: %v", err)
	}

	// Verify directory exists
	parentDir := filepath.Join(dir, "deep", "nested", "dir")
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}

	// Verify file is readable
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded))
	}
}

func TestFileStore_SaveEmptySlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)
	ctx := context.Background()

	if err := store.Save(ctx, []agent.Message{}); err != nil {
		t.Fatalf("Save empty slice failed: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 messages, got %d", len(loaded))
	}
}

func TestFileStore_OverwritesPrevious(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)
	ctx := context.Background()

	// Save first version
	v1 := []agent.Message{{Role: "user", Content: "first"}}
	if err := store.Save(ctx, v1); err != nil {
		t.Fatalf("Save v1: %v", err)
	}

	// Save second version (completely different)
	v2 := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "reply"},
	}
	if err := store.Save(ctx, v2); err != nil {
		t.Fatalf("Save v2: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}
	if loaded[1].Content != "second" {
		t.Errorf("expected %q, got %q", "second", loaded[1].Content)
	}
}

func TestFileStore_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)

	messages := []agent.Message{
		{Role: "system", Content: "You are a test agent."},
		{Role: "user", Content: "Hello"},
	}

	if err := store.Save(context.Background(), messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read raw file and verify it's valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !json.Valid(data) {
		t.Fatal("saved file is not valid JSON")
	}

	// Verify it's a JSON array
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("expected JSON array, got error: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("expected 2 elements in JSON array, got %d", len(raw))
	}
}

func TestFileStore_Path(t *testing.T) {
	path := "/home/user/.codebutler/branches/feature/conversations/pm.json"
	store := NewFileStore(path)
	if store.Path() != path {
		t.Errorf("expected path %q, got %q", path, store.Path())
	}
}

func TestFilePath(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		branch   string
		role     string
		expected string
	}{
		{
			name:     "basic",
			baseDir:  "/repo",
			branch:   "codebutler/add-login",
			role:     "coder",
			expected: "/repo/.codebutler/branches/codebutler/add-login/conversations/coder.json",
		},
		{
			name:     "pm role",
			baseDir:  "/home/user/project",
			branch:   "feature-x",
			role:     "pm",
			expected: "/home/user/project/.codebutler/branches/feature-x/conversations/pm.json",
		},
		{
			name:     "relative base",
			baseDir:  ".",
			branch:   "fix",
			role:     "reviewer",
			expected: ".codebutler/branches/fix/conversations/reviewer.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilePath(tt.baseDir, tt.branch, tt.role)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestFileStore_ResumeMidConversation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.json")
	store := NewFileStore(path)
	ctx := context.Background()

	// Simulate a conversation that was saved after tool execution
	// (agent crashed before next LLM call)
	midConversation := []agent.Message{
		{Role: "system", Content: "You are a coder."},
		{Role: "user", Content: "Implement feature X"},
		{
			Role: "assistant",
			ToolCalls: []agent.ToolCall{
				{ID: "call-1", Name: "Read", Arguments: `{"path":"main.go"}`},
			},
		},
		{Role: "tool", Content: "package main\nfunc main() {}", ToolCallID: "call-1"},
	}

	if err := store.Save(ctx, midConversation); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load and verify the conversation is intact for resume
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(loaded))
	}

	// Verify message order: system → user → assistant(tc) → tool
	expectedRoles := []string{"system", "user", "assistant", "tool"}
	for i, role := range expectedRoles {
		if loaded[i].Role != role {
			t.Errorf("message[%d].Role: expected %q, got %q", i, role, loaded[i].Role)
		}
	}

	// Last message is a tool result → ready for next LLM call
	last := loaded[len(loaded)-1]
	if last.Role != "tool" {
		t.Errorf("expected last message role %q, got %q", "tool", last.Role)
	}
	if last.ToolCallID != "call-1" {
		t.Errorf("expected tool_call_id %q, got %q", "call-1", last.ToolCallID)
	}
}

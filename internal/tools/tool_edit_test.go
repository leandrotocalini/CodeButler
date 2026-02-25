package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEditTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewEditTool(sb)

	// Create a test file
	original := "line 1\nline 2\nline 3\n"
	testFile := filepath.Join(root, "edit.txt")
	os.WriteFile(testFile, []byte(original), 0o644)

	tests := []struct {
		name        string
		args        editArgs
		wantContent string
		wantError   bool
	}{
		{
			name:        "replace string",
			args:        editArgs{Path: "edit.txt", OldString: "line 2", NewString: "modified line 2"},
			wantContent: "line 1\nmodified line 2\nline 3\n",
		},
		{
			name:      "old_string not found",
			args:      editArgs{Path: "edit.txt", OldString: "does not exist", NewString: "replacement"},
			wantError: true,
		},
		{
			name:      "non-existent file",
			args:      editArgs{Path: "missing.txt", OldString: "a", NewString: "b"},
			wantError: true,
		},
		{
			name:      "path escape",
			args:      editArgs{Path: "../../../etc/passwd", OldString: "root", NewString: "evil"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset file for each test
			if tt.name == "replace string" {
				os.WriteFile(testFile, []byte(original), 0o644)
			}

			argsJSON, _ := json.Marshal(tt.args)
			call := ToolCall{ID: "edit-1", Name: "Edit", Arguments: argsJSON}

			result, _ := tool.Execute(context.Background(), call)

			if tt.wantError {
				if !result.IsError {
					t.Errorf("expected error, got: %q", result.Content)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}

			data, _ := os.ReadFile(testFile)
			if string(data) != tt.wantContent {
				t.Errorf("content = %q, want %q", string(data), tt.wantContent)
			}
		})
	}
}

func TestEditTool_Idempotency(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewEditTool(sb)

	// Write file with already-applied content
	os.WriteFile(filepath.Join(root, "done.txt"), []byte("new content here"), 0o644)

	argsJSON, _ := json.Marshal(editArgs{
		Path:      "done.txt",
		OldString: "old content",
		NewString: "new content",
	})
	call := ToolCall{ID: "edit-idem", Name: "Edit", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)

	if result.IsError {
		t.Errorf("idempotent edit should not error, got: %s", result.Content)
	}
	if result.Content != "edit already applied (idempotent)" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestEditTool_MultipleOccurrences(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewEditTool(sb)

	os.WriteFile(filepath.Join(root, "dup.txt"), []byte("aaa bbb aaa"), 0o644)

	argsJSON, _ := json.Marshal(editArgs{
		Path:      "dup.txt",
		OldString: "aaa",
		NewString: "ccc",
	})
	call := ToolCall{ID: "edit-dup", Name: "Edit", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)

	if !result.IsError {
		t.Error("should error when old_string appears multiple times")
	}
}

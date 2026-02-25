package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewReadTool(sb)

	// Create a test file
	content := "hello world\nline 2\n"
	os.WriteFile(filepath.Join(root, "test.txt"), []byte(content), 0o644)

	tests := []struct {
		name      string
		args      readArgs
		want      string
		wantError bool
	}{
		{
			name: "read existing file",
			args: readArgs{Path: "test.txt"},
			want: content,
		},
		{
			name:      "read non-existent file",
			args:      readArgs{Path: "missing.txt"},
			wantError: true,
		},
		{
			name:      "path escape",
			args:      readArgs{Path: "../../../etc/passwd"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			call := ToolCall{ID: "read-1", Name: "Read", Arguments: argsJSON}

			result, _ := tool.Execute(context.Background(), call)

			if tt.wantError {
				if !result.IsError {
					t.Errorf("expected error result, got: %q", result.Content)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}
			if result.Content != tt.want {
				t.Errorf("content = %q, want %q", result.Content, tt.want)
			}
		})
	}
}

func TestReadTool_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewReadTool(sb)

	call := ToolCall{ID: "read-bad", Name: "Read", Arguments: json.RawMessage(`{invalid`)}
	result, _ := tool.Execute(context.Background(), call)

	if !result.IsError {
		t.Error("expected error for invalid JSON")
	}
}

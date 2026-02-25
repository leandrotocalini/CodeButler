package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewWriteTool(sb)

	tests := []struct {
		name      string
		args      writeArgs
		wantError bool
	}{
		{
			name: "write new file",
			args: writeArgs{Path: "output.txt", Content: "hello world"},
		},
		{
			name: "write nested file",
			args: writeArgs{Path: "deep/nested/dir/file.txt", Content: "nested content"},
		},
		{
			name: "overwrite existing file",
			args: writeArgs{Path: "output.txt", Content: "overwritten"},
		},
		{
			name:      "path escape",
			args:      writeArgs{Path: "../../../tmp/evil.txt", Content: "bad"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			call := ToolCall{ID: "write-1", Name: "Write", Arguments: argsJSON}

			result, _ := tool.Execute(context.Background(), call)

			if tt.wantError {
				if !result.IsError {
					t.Error("expected error result")
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}

			// Verify file was written
			safePath := filepath.Join(root, tt.args.Path)
			data, err := os.ReadFile(safePath)
			if err != nil {
				t.Fatalf("file not found: %v", err)
			}
			if string(data) != tt.args.Content {
				t.Errorf("file content = %q, want %q", string(data), tt.args.Content)
			}
		})
	}
}

func TestWriteTool_AtomicWrite(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewWriteTool(sb)

	// Write a file
	argsJSON, _ := json.Marshal(writeArgs{Path: "atomic.txt", Content: "content"})
	call := ToolCall{ID: "w1", Name: "Write", Arguments: argsJSON}
	tool.Execute(context.Background(), call)

	// Verify no temp files remain
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != "atomic.txt" {
			t.Errorf("unexpected file remaining: %s", e.Name())
		}
	}
}

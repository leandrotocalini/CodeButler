package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewGrepTool(sb)

	// Create test files
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "util.go"), []byte("package main\nfunc helper() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "readme.md"), []byte("# Project\nThis is a project\n"), 0o644)

	tests := []struct {
		name      string
		args      grepArgs
		wantSub   string
		wantError bool
	}{
		{
			name:    "find pattern in files",
			args:    grepArgs{Pattern: "func main"},
			wantSub: "main.go",
		},
		{
			name:    "with include filter",
			args:    grepArgs{Pattern: "package", Include: "*.go"},
			wantSub: "package main",
		},
		{
			name:    "specific path",
			args:    grepArgs{Pattern: "Project", Path: "readme.md"},
			wantSub: "Project",
		},
		{
			name: "no matches",
			args: grepArgs{Pattern: "nonexistent_string_xyz"},
		},
		{
			name:      "empty pattern",
			args:      grepArgs{Pattern: ""},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			call := ToolCall{ID: "grep-1", Name: "Grep", Arguments: argsJSON}

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

			if tt.wantSub != "" && !strings.Contains(result.Content, tt.wantSub) {
				t.Errorf("output %q does not contain %q", result.Content, tt.wantSub)
			}
		})
	}
}

func TestGrepTool_PathEscape(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewGrepTool(sb)

	argsJSON, _ := json.Marshal(grepArgs{Pattern: "root", Path: "../../../etc/passwd"})
	call := ToolCall{ID: "grep-escape", Name: "Grep", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)
	if !result.IsError {
		t.Error("should reject path escape")
	}
}

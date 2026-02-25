package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewGlobTool(sb)

	// Create test file structure
	os.MkdirAll(filepath.Join(root, "src", "pkg"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "pkg", "util.go"), []byte("package pkg"), 0o644)
	os.WriteFile(filepath.Join(root, "readme.md"), []byte("# readme"), 0o644)

	tests := []struct {
		name        string
		pattern     string
		wantCount   int
		wantContain string
	}{
		{"all go files recursive", "**/*.go", 2, "main.go"},
		{"top-level md", "*.md", 1, "readme.md"},
		{"specific directory", "src/*.go", 1, "main.go"},
		{"nested glob", "src/**/*.go", 2, "util.go"},
		{"no matches", "**/*.rs", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(globArgs{Pattern: tt.pattern})
			call := ToolCall{ID: "glob-1", Name: "Glob", Arguments: argsJSON}

			result, _ := tool.Execute(context.Background(), call)

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}

			if tt.wantCount == 0 {
				if result.Content != "no files matched" {
					t.Errorf("expected no matches, got: %q", result.Content)
				}
				return
			}

			lines := strings.Split(strings.TrimSpace(result.Content), "\n")
			if len(lines) != tt.wantCount {
				t.Errorf("got %d matches, want %d. Matches: %v", len(lines), tt.wantCount, lines)
			}

			if tt.wantContain != "" && !strings.Contains(result.Content, tt.wantContain) {
				t.Errorf("output %q does not contain %q", result.Content, tt.wantContain)
			}
		})
	}
}

func TestGlobTool_EmptyPattern(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewGlobTool(sb)

	argsJSON, _ := json.Marshal(globArgs{Pattern: ""})
	call := ToolCall{ID: "glob-empty", Name: "Glob", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)
	if !result.IsError {
		t.Error("expected error for empty pattern")
	}
}

func TestMatchDoubleGlob(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"src/main.go", "**/*.go", true},
		{"main.go", "**/*.go", true},
		{"src/pkg/util.go", "**/*.go", true},
		{"src/main.go", "src/*.go", true},
		{"src/pkg/util.go", "src/**/*.go", true},
		{"readme.md", "*.go", false},
		{"src/main.go", "*.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			got := matchDoubleGlob(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("matchDoubleGlob(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

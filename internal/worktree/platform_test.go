package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected Platform
	}{
		{"go project", []string{"go.mod"}, PlatformGo},
		{"node project", []string{"package.json"}, PlatformNode},
		{"python with requirements", []string{"requirements.txt"}, PlatformPython},
		{"python with pyproject", []string{"pyproject.toml"}, PlatformPython},
		{"rust project", []string{"Cargo.toml"}, PlatformRust},
		{"unknown project", []string{"README.md"}, PlatformUnknown},
		{"empty directory", nil, PlatformUnknown},
		{"go takes priority over node", []string{"go.mod", "package.json"}, PlatformGo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				os.WriteFile(filepath.Join(dir, f), []byte(""), 0o644)
			}

			got := DetectPlatform(dir)
			if got != tt.expected {
				t.Errorf("DetectPlatform() = %q, want %q", got, tt.expected)
			}
		})
	}
}

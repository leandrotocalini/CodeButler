package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewSandbox(t *testing.T) {
	dir := t.TempDir()
	sb, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("NewSandbox() error = %v", err)
	}
	if sb.Root != dir {
		t.Errorf("Root = %q, want %q", sb.Root, dir)
	}
}

func TestSandbox_ValidatePath(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)

	// Create a subdirectory and file for tests
	subdir := filepath.Join(root, "src")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(subdir, "main.go"), []byte("package main"), 0o644)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"relative path", "src/main.go", false},
		{"absolute path within root", filepath.Join(root, "src/main.go"), false},
		{"root itself", root, false},
		{"new file in root", "newfile.txt", false},
		{"path escape with ..", "../../../etc/passwd", true},
		{"absolute path outside root", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sb.ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr = %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSandbox_ValidatePath_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)

	// Create a symlink that points outside the sandbox
	outsideDir := t.TempDir()
	os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644)

	linkPath := filepath.Join(root, "escape")
	err := os.Symlink(outsideDir, linkPath)
	if err != nil {
		t.Skip("symlinks not supported")
	}

	_, err = sb.ValidatePath("escape/secret.txt")
	if err == nil {
		t.Error("ValidatePath() should reject symlink escape")
	}
}

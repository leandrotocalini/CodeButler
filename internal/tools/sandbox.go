package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Sandbox enforces path restrictions, ensuring all file operations
// stay within the allowed worktree root.
type Sandbox struct {
	// Root is the absolute path of the worktree root.
	Root string
}

// NewSandbox creates a sandbox rooted at the given directory.
// The root must be an absolute path.
func NewSandbox(root string) (*Sandbox, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("sandbox: invalid root %q: %w", root, err)
	}
	return &Sandbox{Root: abs}, nil
}

// ValidatePath checks that a path is within the sandbox root.
// Returns the cleaned absolute path or an error if the path escapes.
func (s *Sandbox) ValidatePath(path string) (string, error) {
	// Resolve relative paths against the sandbox root
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(s.Root, path))
	}

	// Evaluate symlinks to prevent symlink escapes
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the file doesn't exist yet (Write), check the parent dir
		parent := filepath.Dir(abs)
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr != nil {
			// Parent doesn't exist either; validate the cleaned path directly
			resolved = abs
		} else {
			resolved = filepath.Join(resolvedParent, filepath.Base(abs))
		}
	}

	// Ensure the resolved path is within the sandbox
	rootWithSep := s.Root + string(filepath.Separator)
	if resolved != s.Root && !strings.HasPrefix(resolved, rootWithSep) {
		return "", fmt.Errorf("path %q resolves to %q which is outside sandbox root %q", path, resolved, s.Root)
	}

	return abs, nil
}

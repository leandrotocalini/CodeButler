package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditTool performs exact string replacement in a file within the sandbox.
// Idempotent: if old_string is not found but new_string is present, the edit
// was already applied — skip silently.
type EditTool struct {
	sandbox *Sandbox
}

// NewEditTool creates an EditTool sandboxed to the given root.
func NewEditTool(sandbox *Sandbox) *EditTool {
	return &EditTool{sandbox: sandbox}
}

type editArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (t *EditTool) Name() string        { return "Edit" }
func (t *EditTool) Description() string { return "Replace an exact string in a file" }
func (t *EditTool) RiskTier() RiskTier  { return WriteLocal }

func (t *EditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to edit (relative to worktree root or absolute)"
			},
			"old_string": {
				"type": "string",
				"description": "The exact string to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The replacement string"
			}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *EditTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args editArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	safePath, err := t.sandbox.ValidatePath(args.Path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("failed to read file: %v", err), IsError: true}, nil
	}

	content := string(data)

	// Check if old_string exists
	if !strings.Contains(content, args.OldString) {
		// Idempotency: if new_string is already present, the edit was already applied
		if strings.Contains(content, args.NewString) {
			return ToolResult{Content: "edit already applied (idempotent)"}, nil
		}
		return ToolResult{Content: "old_string not found in file", IsError: true}, nil
	}

	// Ensure old_string is unique (only one occurrence)
	count := strings.Count(content, args.OldString)
	if count > 1 {
		return ToolResult{
			Content: fmt.Sprintf("old_string found %d times — must be unique. Provide more context", count),
			IsError: true,
		}, nil
	}

	// Perform the replacement
	newContent := strings.Replace(content, args.OldString, args.NewString, 1)

	// Atomic write
	dir := filepath.Dir(safePath)
	tmpFile, err := os.CreateTemp(dir, ".codebutler-edit-*")
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("failed to create temp file: %v", err), IsError: true}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to write: %v", err), IsError: true}, nil
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to close: %v", err), IsError: true}, nil
	}

	if err := os.Rename(tmpPath, safePath); err != nil {
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to rename: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: fmt.Sprintf("edited %s: replaced 1 occurrence", args.Path)}, nil
}

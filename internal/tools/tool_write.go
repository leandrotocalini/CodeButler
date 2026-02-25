package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTool writes content to a file within the sandbox using atomic write
// (write to temp + rename) for idempotency and crash safety.
type WriteTool struct {
	sandbox *Sandbox
}

// NewWriteTool creates a WriteTool sandboxed to the given root.
func NewWriteTool(sandbox *Sandbox) *WriteTool {
	return &WriteTool{sandbox: sandbox}
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteTool) Name() string        { return "Write" }
func (t *WriteTool) Description() string { return "Write content to a file (creates or overwrites)" }
func (t *WriteTool) RiskTier() RiskTier  { return WriteLocal }

func (t *WriteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to write (relative to worktree root or absolute)"
			},
			"content": {
				"type": "string",
				"description": "Content to write to the file"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args writeArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	safePath, err := t.sandbox.ValidatePath(args.Path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Create parent directories if needed
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Content: fmt.Sprintf("failed to create directory: %v", err), IsError: true}, nil
	}

	// Atomic write: write to temp file, then rename
	tmpFile, err := os.CreateTemp(dir, ".codebutler-write-*")
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("failed to create temp file: %v", err), IsError: true}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(args.Content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to write: %v", err), IsError: true}, nil
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to close temp file: %v", err), IsError: true}, nil
	}

	if err := os.Rename(tmpPath, safePath); err != nil {
		os.Remove(tmpPath)
		return ToolResult{Content: fmt.Sprintf("failed to rename: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path)}, nil
}

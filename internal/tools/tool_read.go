package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ReadTool reads file contents within the sandbox.
type ReadTool struct {
	sandbox *Sandbox
}

// NewReadTool creates a ReadTool sandboxed to the given root.
func NewReadTool(sandbox *Sandbox) *ReadTool {
	return &ReadTool{sandbox: sandbox}
}

type readArgs struct {
	Path string `json:"path"`
}

func (t *ReadTool) Name() string        { return "Read" }
func (t *ReadTool) Description() string { return "Read the contents of a file" }
func (t *ReadTool) RiskTier() RiskTier  { return Read }

func (t *ReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to read (relative to worktree root or absolute)"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ReadTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args readArgs
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

	return ToolResult{Content: string(data)}, nil
}

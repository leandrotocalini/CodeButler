package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// GrepTool searches file contents using grep within the sandbox.
type GrepTool struct {
	sandbox *Sandbox
}

// NewGrepTool creates a GrepTool sandboxed to the given root.
func NewGrepTool(sandbox *Sandbox) *GrepTool {
	return &GrepTool{sandbox: sandbox}
}

type grepArgs struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path,omitempty"`      // directory or file, default "."
	Include   string `json:"include,omitempty"`    // file glob filter, e.g. "*.go"
	Recursive bool   `json:"recursive,omitempty"`  // default true
}

func (t *GrepTool) Name() string        { return "Grep" }
func (t *GrepTool) Description() string { return "Search file contents for a pattern using grep" }
func (t *GrepTool) RiskTier() RiskTier  { return Read }

func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The regex pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "Directory or file to search in (default: worktree root)"
			},
			"include": {
				"type": "string",
				"description": "File glob filter (e.g. '*.go', '*.ts')"
			},
			"recursive": {
				"type": "boolean",
				"description": "Search recursively (default: true)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args grepArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Pattern == "" {
		return ToolResult{Content: "pattern is required", IsError: true}, nil
	}

	// Build grep command arguments
	grepArgs := []string{"-n"} // line numbers
	if args.Recursive || args.Path == "" {
		grepArgs = append(grepArgs, "-r")
	}
	if args.Include != "" {
		grepArgs = append(grepArgs, "--include="+args.Include)
	}
	grepArgs = append(grepArgs, args.Pattern)

	// Determine search path
	searchPath := "."
	if args.Path != "" {
		safePath, err := t.sandbox.ValidatePath(args.Path)
		if err != nil {
			return ToolResult{Content: err.Error(), IsError: true}, nil
		}
		searchPath = safePath
	}
	grepArgs = append(grepArgs, searchPath)

	cmd := exec.CommandContext(ctx, "grep", grepArgs...)
	cmd.Dir = t.sandbox.Root

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	if err != nil {
		// grep exits 1 when no matches found — this is not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return ToolResult{Content: "no matches found"}, nil
		}
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return ToolResult{Content: fmt.Sprintf("grep error: %s", errMsg), IsError: true}, nil
	}

	return ToolResult{Content: output}, nil
}

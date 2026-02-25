package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const defaultBashTimeout = 120 * time.Second

// BashTool executes shell commands within the sandbox directory.
// Risk tier depends on command classification (safe, unknown, dangerous).
type BashTool struct {
	sandbox *Sandbox
	timeout time.Duration
}

// NewBashTool creates a BashTool that runs commands in the sandbox root.
func NewBashTool(sandbox *Sandbox) *BashTool {
	return &BashTool{sandbox: sandbox, timeout: defaultBashTimeout}
}

type bashArgs struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout,omitempty"` // optional timeout in seconds
}

func (t *BashTool) Name() string        { return "Bash" }
func (t *BashTool) Description() string { return "Execute a shell command" }
func (t *BashTool) RiskTier() RiskTier  { return WriteLocal } // dynamic, overridden by classifier

func (t *BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			},
			"timeout": {
				"type": "integer",
				"description": "Optional timeout in seconds (default 120)"
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args bashArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Command == "" {
		return ToolResult{Content: "command is required", IsError: true}, nil
	}

	// Classify the command risk
	risk := ClassifyBashCommand(args.Command)
	if risk == Destructive {
		return ToolResult{
			Content: fmt.Sprintf("command classified as DESTRUCTIVE: %q — requires user approval", args.Command),
			IsError: true,
		}, nil
	}

	// Set timeout
	timeout := t.timeout
	if args.Timeout != nil && *args.Timeout > 0 {
		timeout = time.Duration(*args.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", args.Command)
	cmd.Dir = t.sandbox.Root

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Content: fmt.Sprintf("command timed out after %s\n%s", timeout, output),
				IsError: true,
			}, nil
		}
		return ToolResult{
			Content: fmt.Sprintf("exit status: %v\n%s", err, output),
			IsError: true,
		}, nil
	}

	return ToolResult{Content: output}, nil
}

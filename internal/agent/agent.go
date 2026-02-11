package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/config"
)

type Result struct {
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	CostUSD   float64 `json:"total_cost_usd"`
	NumTurns  int     `json:"num_turns"`
	RawJSON   string  `json:"-"` // raw output from claude -p for debugging
}

type Agent struct {
	workDir        string
	maxTurns       int
	timeout        time.Duration
	permissionMode string
}

func New(workDir string, cfg config.ClaudeConfig) *Agent {
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 5
	}
	timeout := time.Duration(cfg.Timeout) * time.Minute
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	permissionMode := cfg.PermissionMode
	if permissionMode == "" {
		permissionMode = "bypassPermissions"
	}

	return &Agent{
		workDir:        workDir,
		maxTurns:       maxTurns,
		timeout:        timeout,
		permissionMode: permissionMode,
	}
}

// ContinuationMarker is the string Claude appends when it hits max turns and has more work to do.
const ContinuationMarker = "[CONTINUING]"

// NeedInputMarker is the string Claude appends when it needs user confirmation or input to proceed.
const NeedInputMarker = "[NEED_USER_INPUT]"

const whatsAppSystemPrompt = `You are responding via WhatsApp. Important rules:
- Do NOT use EnterPlanMode. Present plans as normal messages instead.
- When proposing a plan or architecture, ALWAYS end with: "Reply *yes* to implement, or describe the changes you want."
- ALWAYS include a text response, even when you only performed tool calls. Never return empty output.
- You have a limited number of tool-use turns per invocation. If you are stopped because you hit the turn limit and still have more work to do, summarize what you accomplished so far and what you will do next, then end your message with exactly: ` + ContinuationMarker + `
- When you need user confirmation, a decision, or any input before you can proceed, end your message with exactly: ` + NeedInputMarker

const imageInstruction = `
- When you see <attached-image path="...">, use the Read tool to view the image file at that path.`

const sendImageInstruction = `
- To send the user an image file, wrap the absolute path in your response: <send-image path="/absolute/path/to/file.png">optional caption</send-image>
  You can include multiple images. Text around the tags is sent as a normal message.`

func (a *Agent) Run(ctx context.Context, prompt, sessionID string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// Build system prompt dynamically — only add instructions relevant to this prompt
	sysPrompt := whatsAppSystemPrompt + sendImageInstruction
	if strings.Contains(prompt, "<attached-image") {
		sysPrompt += imageInstruction
	}

	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--max-turns", fmt.Sprintf("%d", a.maxTurns),
		"--permission-mode", a.permissionMode,
		"--append-system-prompt", sysPrompt,
	}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = a.workDir

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude timed out after %s", a.timeout)
		}
		// Try to parse stderr for details
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("claude exec failed: %w", err)
	}

	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		// If JSON parsing fails, treat raw output as the result
		return &Result{
			Result:  string(output),
			RawJSON: string(output),
		}, nil
	}

	result.RawJSON = string(output)
	return &result, nil
}

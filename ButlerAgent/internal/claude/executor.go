package claude

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Error    error
}

type Executor struct {
	timeout time.Duration
}

func NewExecutor() *Executor {
	return &Executor{
		timeout: 5 * time.Minute,
	}
}

func (e *Executor) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

func (e *Executor) Execute(repoPath string, args ...string) (*ExecutionResult, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repository path is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecutionResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
			return result, fmt.Errorf("failed to execute claude: %w", err)
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Error = fmt.Errorf("command timed out after %v", e.timeout)
		return result, result.Error
	}

	return result, nil
}

func (e *Executor) ExecutePrompt(repoPath, prompt string) (*ExecutionResult, error) {
	return e.Execute(repoPath, prompt)
}

func (e *Executor) CheckInstalled() bool {
	cmd := exec.Command("claude", "--version")
	err := cmd.Run()
	return err == nil
}

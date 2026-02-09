# Claude Package

This package handles Claude Code CLI execution for CodeButler.

## Files

### executor.go
- **Executor**: Manages Claude Code CLI execution
- **ExecutionResult**: Contains command output, errors, and timing
- **Execute()**: Run Claude Code with arguments
- **ExecutePrompt()**: Run a prompt in a repository
- **CheckInstalled()**: Verify Claude CLI is available

## Features

- Execute Claude Code commands in any repository
- Capture stdout, stderr, and exit codes
- Configurable timeout (default: 5 minutes)
- Uses Claude Code CLI's built-in authentication
- Check if Claude CLI is installed

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/claude"
)

func main() {
    // Create executor (no token needed)
    executor := claude.NewExecutor()

    // Optional: Set custom timeout
    executor.SetTimeout(10 * time.Minute)

    // Check if Claude is installed
    if !executor.CheckInstalled() {
        fmt.Println("Claude CLI not installed")
        return
    }

    // Execute a prompt
    result, err := executor.ExecutePrompt(
        "/path/to/repo",
        "add error handling to the API endpoints",
    )

    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }

    fmt.Printf("Exit Code: %d\n", result.ExitCode)
    fmt.Printf("Duration: %v\n", result.Duration)
    fmt.Printf("Output:\n%s\n", result.Stdout)

    if result.Stderr != "" {
        fmt.Printf("Errors:\n%s\n", result.Stderr)
    }
}
```

## ExecutionResult

```go
type ExecutionResult struct {
    Stdout   string        // Standard output
    Stderr   string        // Standard error
    ExitCode int           // Exit code (0 = success)
    Duration time.Duration // Execution time
    Error    error         // Error if execution failed
}
```

## Configuration

### Authentication

Claude Code CLI handles authentication automatically. Users need to login once:

```bash
# One-time setup
claude login
# Opens browser, login with Claude account
```

After that, all `claude` commands (including those from CodeButler) use the saved authentication.

No tokens to manage, no environment variables needed.

### Timeout

Default timeout is 5 minutes. Adjust based on your needs:

```go
executor.SetTimeout(10 * time.Minute)  // For long-running tasks
executor.SetTimeout(30 * time.Second)  // For quick commands
```

**Note**: Some Claude Code operations can take several minutes, especially:
- Large codebases
- Complex refactorings
- Multiple file changes
- First run (model loading)

## Claude CLI Installation

Install from: https://docs.anthropic.com/en/docs/claude-code

Verify installation:
```bash
claude --version
```

## Error Handling

```go
result, err := executor.ExecutePrompt(repoPath, prompt)

if err != nil {
    // Execution failed (timeout, not found, etc.)
    fmt.Printf("Failed: %v\n", err)
    return
}

if result.ExitCode != 0 {
    // Command ran but returned error
    fmt.Printf("Claude returned error: %s\n", result.Stderr)
    return
}

// Success
fmt.Println(result.Stdout)
```

## Common Issues

### "claude: command not found"
- Claude CLI not installed or not in PATH
- Use `executor.CheckInstalled()` to verify

### "context deadline exceeded"
- Command timed out (default: 5 minutes)
- Increase timeout with `SetTimeout()`

### "exit status 1"
- Claude Code returned an error
- Check `result.Stderr` for details
- Check `result.ExitCode` for error type

## Security Notes

- Authentication handled by Claude Code CLI (secure OAuth flow)
- Commands run in the specified repository directory
- No shell expansion or injection (uses exec.Command directly)
- Output is captured, not displayed to console

## Performance

Typical execution times:
- Simple prompts: 10-30 seconds
- File changes: 30-60 seconds
- Complex refactorings: 1-3 minutes
- Large codebases: 2-5 minutes

**Tip**: Set realistic timeout values based on your use case.

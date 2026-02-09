# Commands Package

This package handles parsing and validation of WhatsApp commands for CodeButler.

## Command Format

All commands start with `@codebutler`:

```
@codebutler <command> [arguments]
```

## Available Commands

### help
Show help message with all available commands.

**Usage:**
```
@codebutler help
@codebutler h
@codebutler
```

### repos
List all available repositories in Sources/.

**Usage:**
```
@codebutler repos
@codebutler list
@codebutler ls
```

**Output:**
- Repository name
- Project type (go, node, python, etc.)
- CLAUDE.md status (✅/❌)

### use
Select a repository to work with.

**Usage:**
```
@codebutler use <repo-name>
@codebutler select <repo-name>
```

**Example:**
```
@codebutler use aurum
```

**Requirements:**
- Repository must have CLAUDE.md file

### status
Show currently active repository.

**Usage:**
```
@codebutler status
@codebutler current
@codebutler pwd
```

### run
Execute a Claude Code command in the active repository.

**Usage:**
```
@codebutler run <prompt>
@codebutler exec <prompt>
@codebutler do <prompt>
```

**Example:**
```
@codebutler run add a new function to handle user authentication
```

**Requirements:**
- Must have an active repository selected with `use`
- Claude Code CLI must be installed

### clear
Clear the active repository selection.

**Usage:**
```
@codebutler clear
@codebutler reset
```

## Command Parsing

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/commands"
)

func main() {
    // Parse command
    cmd := commands.Parse("@codebutler use aurum")

    if cmd == nil {
        fmt.Println("Not a command")
        return
    }

    // Validate
    if err := commands.ValidateCommand(cmd); err != nil {
        fmt.Printf("Invalid: %v\n", err)
        return
    }

    // Use command
    fmt.Printf("Type: %s\n", cmd.Type)
    fmt.Printf("Args: %v\n", cmd.Args)
    fmt.Printf("First arg: %s\n", cmd.GetArg(0))
}
```

## Command Types

```go
const (
    CommandHelp    = "help"
    CommandRepos   = "repos"
    CommandUse     = "use"
    CommandRun     = "run"
    CommandStatus  = "status"
    CommandClear   = "clear"
    CommandUnknown = "unknown"
)
```

## Validation Rules

- **use**: Requires repository name argument
- **run**: Requires prompt argument
- Unknown commands return error with suggestion to use `help`

## Testing

```bash
go test ./internal/commands/... -v
```

Tests cover:
- ✅ All command types
- ✅ Command aliases
- ✅ Argument parsing
- ✅ Validation rules
- ✅ Edge cases (empty, malformed, etc.)

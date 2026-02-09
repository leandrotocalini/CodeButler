# Bot Package

This package orchestrates CodeButler's command handling, integrating all components.

## Architecture

```
Bot Handler
├── Commands Parser  (parse @codebutler commands)
├── Session Manager  (track active repo per chat)
├── Repo Scanner     (find repositories with CLAUDE.md)
└── Claude Executor  (run Claude Code CLI)
```

## Files

### handler.go
- **Bot**: Main orchestrator
- **HandleCommand()**: Process incoming commands
- **handleRepos()**: List repositories
- **handleUse()**: Select repository
- **handleStatus()**: Show active repo
- **handleRun()**: Execute Claude Code (in background)
- **handleClear()**: Clear session
- **executeInBackground()**: Async Claude Code execution

## Usage Example

```go
package main

import (
    "github.com/leandrotocalini/CodeButler/internal/bot"
    "github.com/leandrotocalini/CodeButler/internal/config"
)

func main() {
    cfg, _ := config.Load("config.json")

    // Create bot with message sender
    codebutler := bot.NewBot(cfg, func(chatID, text string) error {
        return whatsappClient.SendMessage(chatID, text)
    })

    // Load repositories
    codebutler.LoadRepositories()

    // Handle incoming message
    response := codebutler.HandleCommand(
        "120363405395407771@g.us",
        "@codebutler repos",
    )

    if response != "" {
        whatsappClient.SendMessage(chatID, response)
    }
}
```

## Command Flow

### 1. Parse Command
```
Message: "@codebutler repos"
    ↓
Commands.Parse()
    ↓
Command{Type: CommandRepos, Args: []}
```

### 2. Validate Command
```
Command{Type: CommandUse, Args: []}
    ↓
ValidateCommand()
    ↓
Error: "use requires repository name"
```

### 3. Execute Command
```
Command{Type: CommandUse, Args: ["aurum"]}
    ↓
Bot.handleUse()
    ↓
- Check if repo exists
- Check if has CLAUDE.md ✅
- SessionMgr.SetActiveRepo()
    ↓
Response: "✅ Now using: *aurum*"
```

### 4. Run Command (Background)
```
Command{Type: CommandRun, Args: ["add", "login", "function"]}
    ↓
Bot.handleRun()
    ↓
- Check active repo (SessionMgr)
- Check Claude CLI installed
- Return "Executing..." immediately
- Launch executeInBackground() goroutine
    ↓
executeInBackground()
    ↓
- Execute Claude Code
- Wait for completion (may take minutes)
- Send result back via sendMessage callback
```

## Background Execution

The `run` command executes asynchronously:

1. **Immediate response**: "🤖 Executing... ⏳"
2. **Background goroutine**: Runs Claude Code
3. **Completion callback**: Sends result when done

This prevents blocking WhatsApp message handler.

**User Experience:**
```
User: @codebutler run add error handling
Bot:  🤖 Executing in *aurum*...
      ⏳ This may take a few minutes...

[... 2 minutes later ...]

Bot:  ✅ Execution completed in *aurum*
      ⏱️  Duration: 127.3s

      📤 Output:
      ```
      Added error handling to:
      - api/handlers.go
      - api/middleware.go
      ...
      ```
```

## Error Handling

### Repository Not Found
```
@codebutler use nonexistent
↓
❌ Repository 'nonexistent' not found
💡 List repos with: @codebutler repos
```

### No CLAUDE.md
```
@codebutler use project-without-claude
↓
❌ Repository 'project-without-claude' doesn't have CLAUDE.md
💡 Add a CLAUDE.md file to use this repo
```

### No Active Repo
```
@codebutler run fix bug
↓
❌ No active repository
💡 Select one first: @codebutler use <repo-name>
```

### Claude CLI Not Installed
```
@codebutler run add feature
↓
❌ Claude Code CLI not installed
💡 Install from: https://docs.anthropic.com/en/docs/claude-code
```

### Execution Error
```
@codebutler run invalid prompt
↓
[Initial] 🤖 Executing...
[Later]   ❌ Error: command failed
          ```
          Error: Could not understand prompt
          ```
```

## Session Management

Sessions are per-chat:

```go
// Chat A
@codebutler use project-A
@codebutler run add feature
// Works on project-A

// Chat B (different group)
@codebutler use project-B
@codebutler run fix bug
// Works on project-B
```

No interference between chats.

## Output Truncation

Large outputs are truncated to avoid WhatsApp message limits:

- **Stdout**: Max 3000 characters
- **Stderr**: Max 1000 characters
- Truncated with: `... (truncated)`

## Integration Example

```go
// In main WhatsApp handler
client.OnMessage(func(msg whatsapp.Message) {
    // Check access control
    if !access.IsAllowed(msg, cfg) {
        return
    }

    // Handle @codebutler commands
    response := codebutler.HandleCommand(msg.Chat, msg.Content)

    if response != "" {
        client.SendMessage(msg.Chat, response)
    }
})
```

## Dependencies

- **commands**: Parse and validate commands
- **session**: Track active repo per chat
- **repo**: Scan and manage repositories
- **claude**: Execute Claude Code CLI
- **config**: Configuration (OAuth token, etc.)

## Testing

To test without WhatsApp:

```go
bot := bot.NewBot(cfg, func(chatID, text string) error {
    fmt.Printf("Would send to %s: %s\n", chatID, text)
    return nil
})

response := bot.HandleCommand("test-chat", "@codebutler repos")
fmt.Println(response)
```

## Future Enhancements

Possible improvements:
- Command history per session
- Cancel running executions
- Progress updates during execution
- Multiple concurrent executions
- Execution queue
- Rate limiting
- User permissions

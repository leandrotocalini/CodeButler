# Session Package

This package manages user sessions for CodeButler, tracking active repositories per chat.

## Features

- Track active repository per chat/group
- Thread-safe session management
- Simple key-value store (chatID → session)

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/session"
)

func main() {
    mgr := session.NewManager()

    chatID := "120363405395407771@g.us"

    // Set active repository
    mgr.SetActiveRepo(chatID, "my-project", "/path/to/my-project")

    // Get active repository
    name, path, err := mgr.GetActiveRepo(chatID)
    if err != nil {
        fmt.Println("No active repo")
    } else {
        fmt.Printf("Active: %s at %s\n", name, path)
    }

    // Clear session
    mgr.ClearSession(chatID)

    // List all sessions
    sessions := mgr.ListSessions()
    fmt.Printf("Total sessions: %d\n", len(sessions))
}
```

## Session Struct

```go
type Session struct {
    ChatID          string  // WhatsApp chat/group ID
    ActiveRepo      string  // Repository name
    ActiveRepoPath  string  // Full path to repository
    LastCommandTime int64   // Unix timestamp of last command
}
```

## Thread Safety

All operations are thread-safe using `sync.RWMutex`:
- Multiple concurrent reads allowed
- Writes are exclusive
- No race conditions

## Why Per-Chat Sessions?

Each WhatsApp group/chat has its own session:
- **Group A** can work on `project-1`
- **Group B** can work on `project-2` simultaneously
- No interference between groups

## Methods

### NewManager()
Creates a new session manager.

### SetActiveRepo(chatID, repoName, repoPath)
Sets the active repository for a chat.
- Creates session if doesn't exist
- Updates existing session if exists

### GetActiveRepo(chatID)
Gets the active repository for a chat.
- Returns: `(repoName, repoPath, error)`
- Error if no session or no active repo

### ClearSession(chatID)
Removes a chat's session entirely.

### ListSessions()
Returns all active sessions (for debugging/monitoring).

## Example: Bot Integration

```go
type Bot struct {
    sessionMgr *session.Manager
    // ...
}

func (b *Bot) handleUseCommand(chatID, repoName string) {
    // Find repo
    repo := findRepo(repoName)

    // Set as active
    b.sessionMgr.SetActiveRepo(chatID, repo.Name, repo.Path)
}

func (b *Bot) handleRunCommand(chatID, prompt string) {
    // Get active repo
    name, path, err := b.sessionMgr.GetActiveRepo(chatID)
    if err != nil {
        return "No active repository"
    }

    // Execute in that repo
    executeClaudeCode(path, prompt)
}
```

## Persistence

Currently **in-memory only**:
- Sessions lost on restart
- Users must re-select repo after bot restart

**Future enhancement**: Could persist to SQLite/Redis for durability.

## Scalability

Current implementation is fine for:
- Single instance
- < 100 concurrent chats
- Low memory usage (few KB per session)

For larger scale, consider:
- External session store (Redis)
- Session expiration
- Distributed session management

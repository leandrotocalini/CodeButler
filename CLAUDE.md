# CodeButler Project Context

## Project Overview

CodeButler is a WhatsApp-based controller for Claude Code that enables multi-repository development workflows through a simple Go binary. It acts as a bridge between WhatsApp and Claude Code SDK, allowing developers to interact with multiple code repositories from their phone.

## Core Concept

Think of CodeButler as a **multi-repo aware WhatsApp bot** that:
1. Connects to WhatsApp via QR code authentication
2. Receives messages from your personal number or approved groups
3. Routes requests to the appropriate repository in `Sources/`
4. Executes Claude Code SDK in that repository's context
5. Returns responses via WhatsApp

## Architecture

```
WhatsApp ←→ Go Server (CodeButler) ←→ Claude Code SDK (per-repo)
                ↓
         Sources/ directory
         ├── repo-a/
         ├── repo-b/
         └── repo-c/
```

## Key Design Decisions

### 1. Why Go?
- **Single binary**: Easy deployment (just `./butler`)
- **Fast startup**: ~10ms vs Node.js ~200ms
- **Low memory**: ~20MB idle vs Node.js ~50MB
- **Concurrency**: Goroutines for parallel message handling
- **No runtime**: Ship one file, no dependencies

### 2. Why Claude Code SDK (not Claude API)?
- **Cost**: Free with Claude Pro/Max subscription
- **Features**: Full tool use, file operations, bash
- **Context**: Better understanding of code structure
- **Sessions**: Persistent conversation history
- **No limits**: No token counting or rate limits (within subscription)

### 3. Why Multi-Repo Focus?
Real developers often work on:
- Multiple microservices
- Frontend + Backend repos
- Shared libraries
- Different client projects

CodeButler makes it natural to ask questions like:
- "Compare auth implementation between api-service and mobile-app"
- "What's the data flow from frontend to backend?"
- "Find all uses of UserModel across repos"

### 4. Why WhatsApp?
- **Mobile-first**: Code from anywhere
- **Voice input**: Transcribe ideas while walking
- **Group collab**: Team can share one assistant
- **Familiar UX**: Everyone knows WhatsApp
- **Always on**: No need to open laptop

## File Structure

```
codebutler/
├── main.go                   # Entry point
├── config.json              # Runtime config (gitignored)
├── .gitignore
│
├── internal/
│   ├── whatsapp/            # WhatsApp client & handlers
│   ├── access/              # Access control logic
│   ├── audio/               # Whisper API integration
│   ├── repo/                # Repository management
│   ├── claude/              # Claude Code SDK executor
│   └── config/              # Config loading
│
└── Sources/                 # Multi-repo workspace
    ├── repo-a/
    │   └── CLAUDE.md        # Repo-specific context
    ├── repo-b/
    │   └── CLAUDE.md
    └── repo-c/
        └── CLAUDE.md
```

## Configuration System

### config.json (auto-generated on first run)

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "1234567890@s.whatsapp.net",
    "allowedGroups": [
      {
        "jid": "120363123456789012@g.us",
        "name": "CodeButler Developer",
        "enabled": true,
        "isDevControl": true
      },
      {
        "jid": "123456789-1234567890@g.us",
        "name": "Team Alpha",
        "enabled": true,
        "isDevControl": false
      }
    ]
  },
  "openai": {
    "apiKey": "sk-..."
  },
  "claudeCode": {
    "oauthToken": "from-env-CLAUDE_CODE_OAUTH_TOKEN"
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

**Important**: `config.json` is in `.gitignore` - it contains secrets!

## "CodeButler Developer" Group - Development Control Center

### Purpose

The "CodeButler Developer" group is a special WhatsApp group automatically created during first-time setup. It serves as your personal development control center with the following characteristics:

**Key Properties:**
- **Private**: Only you as a member (single-user group)
- **Auto-created**: Created or found during setup
- **Pre-enabled**: Automatically added to `allowedGroups` with `isDevControl: true`
- **Dedicated**: Used exclusively for development workflow control

### Why a Group Instead of Personal Chat?

1. **Organization**: Separate personal messages from dev commands
2. **Context Preservation**: Group history persists better than personal chat
3. **Future-proof**: Easy to add team members later if needed
4. **Notifications**: Can mute/unmute independently from personal chat
5. **Multi-device**: Works better across multiple devices

### Special Capabilities

When CodeButler detects a message from the CodeButler Developer group (`isDevControl: true`):

```go
func handleMessage(msg whatsapp.Message, cfg config.Config) {
    // Check if from dev control group
    isDevControl := false
    for _, group := range cfg.WhatsApp.AllowedGroups {
        if group.JID == msg.Sender && group.IsDevControl {
            isDevControl = true
            break
        }
    }

    if isDevControl {
        // Enable special commands:
        // - System status queries
        // - Bulk operations across all repos
        // - Workflow automation
        // - Config modifications
        // - Debug commands
    }
}
```

### Usage Examples

**System Status:**
```
User (in CodeButler Developer): @codebutler status
Bot: 📊 System Status
     - Active repos: 3
     - Running sessions: 1 (api-service)
     - Queue: empty
     - Last activity: 2m ago
     - Memory usage: 45MB
     - Uptime: 3h 24m
```

**Bulk Operations:**
```
User: @codebutler run tests in all repos
Bot: Running tests across 3 repositories...

     ✅ api-service: 45 passed, 0 failed
     ✅ frontend-app: 123 passed, 2 skipped
     ❌ mobile-client: 18 passed, 2 failed

     Total: 186 passed, 2 failed, 2 skipped
     Duration: 12.4s
```

**Workflow Automation:**
```
User: @codebutler when api-service tests pass, create PR
Bot: Created workflow automation:

     Trigger: api-service tests pass
     Actions:
     1. Run linter
     2. Build Docker image
     3. Create PR to main branch

     Saved as: workflow-api-pr-creation
     Status: Active ✅
```

**Cross-Repo Analysis:**
```
User: @codebutler compare auth flow between api-service and mobile-client
Bot: Analyzing authentication flows...

     API Service (Sources/api-service/):
     - JWT with RS256
     - Token expiry: 24h
     - Refresh token: Yes
     - Session storage: Redis

     Mobile Client (Sources/mobile-client/):
     - JWT stored in Keychain
     - Auto-refresh on 401
     - Biometric authentication: Yes
     - Offline mode: Basic features only

     Differences:
     - Mobile uses Keychain, API uses Redis
     - Mobile has biometric auth
     - API has longer token expiry
```

### Comparison: Personal Chat vs CodeButler Developer Group

| Feature | Personal Chat | CodeButler Developer Group |
|---------|---------------|------------------------|
| **Basic commands** | ✅ Yes | ✅ Yes |
| **Repo operations** | ✅ Yes | ✅ Yes |
| **System status** | ❌ Limited | ✅ Full details |
| **Bulk operations** | ❌ No | ✅ Yes |
| **Workflow automation** | ❌ No | ✅ Yes |
| **Config changes** | ❌ No | ✅ Yes (future) |
| **Debug commands** | ❌ No | ✅ Yes |
| **Context mixing** | ❌ Mixed with personal msgs | ✅ Dev-only |

### Recommendation

Use the CodeButler Developer group as your primary interface with CodeButler for all development tasks. Reserve personal chat for quick questions or when you're in a conversation and don't want to switch contexts.

## Component Breakdown

### 1. WhatsApp Handler (`internal/whatsapp/`)

**Purpose**: Manage WhatsApp Web connection

**Key Files**:
- `client.go` - WhatsApp socket connection
- `auth.go` - QR code generation & session management
- `handler.go` - Message event handlers
- `media.go` - Download voice messages, images
- `groups.go` - Group creation and management

**Libraries**:
- `github.com/Rhymen/go-whatsapp` - WhatsApp Web protocol

**Responsibilities**:
- Establish WhatsApp connection via QR
- Persist session to avoid re-scanning QR
- Listen for incoming messages
- Send outgoing messages
- Download media (voice messages)
- Fetch group metadata
- **Create/find "CodeButler Developer" control group**
- Manage group operations (create, add members, get info)

**Group Management (`internal/whatsapp/groups.go`)**:

```go
package whatsapp

import (
    "fmt"
    whatsapp "github.com/Rhymen/go-whatsapp"
)

type Group struct {
    JID  string
    Name string
}

// GetGroups returns all groups the user is part of
func (c *Client) GetGroups() ([]Group, error) {
    groups := []Group{}

    // Get all chats
    chats, err := c.conn.GetAllChats()
    if err != nil {
        return nil, err
    }

    // Filter for group chats
    for _, chat := range chats {
        if strings.HasSuffix(chat.Jid, "@g.us") {
            groups = append(groups, Group{
                JID:  chat.Jid,
                Name: chat.Name,
            })
        }
    }

    return groups, nil
}

// CreateGroup creates a new WhatsApp group
func (c *Client) CreateGroup(name string, participants []string) (string, error) {
    // WhatsApp groups require JID format for participants
    // participants should be like: "1234567890@s.whatsapp.net"

    groupJID, err := c.conn.CreateGroup(name, participants)
    if err != nil {
        return "", fmt.Errorf("failed to create group: %w", err)
    }

    return groupJID, nil
}

// GetGroupInfo returns detailed info about a group
func (c *Client) GetGroupInfo(groupJID string) (*whatsapp.GroupInfo, error) {
    info, err := c.conn.GetGroupInfo(groupJID)
    if err != nil {
        return nil, fmt.Errorf("failed to get group info: %w", err)
    }

    return &info, nil
}

// AddParticipants adds members to a group
func (c *Client) AddParticipants(groupJID string, participants []string) error {
    _, err := c.conn.UpdateGroupParticipants(groupJID, participants, whatsapp.ParticipantsAdd)
    return err
}

// RemoveParticipants removes members from a group
func (c *Client) RemoveParticipants(groupJID string, participants []string) error {
    _, err := c.conn.UpdateGroupParticipants(groupJID, participants, whatsapp.ParticipantsRemove)
    return err
}
```

### 2. Access Control (`internal/access/`)

**Purpose**: Validate message senders

**Key Files**:
- `control.go` - Validation logic
- `groups.go` - Group allow-list management

**Logic**:
```go
func IsAllowed(sender string, config Config) bool {
    // Personal number always allowed
    if sender == config.WhatsApp.PersonalNumber {
        return true
    }

    // Check if sender is from allowed group
    for _, group := range config.WhatsApp.AllowedGroups {
        if group.JID == sender && group.Enabled {
            return true
        }
    }

    return false
}
```

**Security Model**:
- Default deny (reject unknown senders)
- Explicit allow-list in config.json
- Personal number has full access
- Groups can be enabled/disabled without removing

### 3. Audio Processor (`internal/audio/`)

**Purpose**: Transcribe voice messages

**Key Files**:
- `transcribe.go` - OpenAI Whisper API integration
- `download.go` - Save audio to temp file

**Flow**:
```
1. Detect voice message in WhatsApp handler
2. Download audio to /tmp/audio-{timestamp}.ogg
3. Call OpenAI Whisper API:
   POST https://api.openai.com/v1/audio/transcriptions
   Body: multipart/form-data with audio file
4. Extract transcription text
5. Delete temp file
6. Return text for processing
```

**API Call**:
```go
func TranscribeAudio(audioPath string, apiKey string) (string, error) {
    file, _ := os.Open(audioPath)
    defer file.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)
    part, _ := writer.CreateFormFile("file", filepath.Base(audioPath))
    io.Copy(part, file)
    writer.WriteField("model", "whisper-1")
    writer.Close()

    req, _ := http.NewRequest("POST",
        "https://api.openai.com/v1/audio/transcriptions",
        body)
    req.Header.Set("Authorization", "Bearer " + apiKey)
    req.Header.Set("Content-Type", writer.FormDataContentType())

    resp, _ := http.DefaultClient.Do(req)
    defer resp.Body.Close()

    var result struct {
        Text string `json:"text"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    return result.Text, nil
}
```

### 4. Repository Manager (`internal/repo/`)

**Purpose**: Discover and route to repositories

**Key Files**:
- `manager.go` - Scan Sources/ directory
- `router.go` - Determine target repo from message
- `context.go` - Load CLAUDE.md for repo

**Repository Discovery**:
```go
func DiscoverRepos(sourcesPath string) ([]Repository, error) {
    var repos []Repository

    entries, _ := os.ReadDir(sourcesPath)
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        repoPath := filepath.Join(sourcesPath, entry.Name())

        // Check if it's a git repo
        gitPath := filepath.Join(repoPath, ".git")
        if _, err := os.Stat(gitPath); os.IsNotExist(err) {
            continue
        }

        // Load CLAUDE.md if exists
        claudePath := filepath.Join(repoPath, "CLAUDE.md")
        context := ""
        if data, err := os.ReadFile(claudePath); err == nil {
            context = string(data)
        }

        repos = append(repos, Repository{
            Name: entry.Name(),
            Path: repoPath,
            Context: context,
        })
    }

    return repos, nil
}
```

**Routing Logic**:
```go
func RouteMessage(message string, repos []Repository) *Repository {
    // Explicit: "in repo-name: do something"
    if strings.HasPrefix(message, "in ") {
        parts := strings.SplitN(message, ":", 2)
        repoName := strings.TrimPrefix(parts[0], "in ")
        repoName = strings.TrimSpace(repoName)

        for _, repo := range repos {
            if repo.Name == repoName {
                return &repo
            }
        }
    }

    // Implicit: analyze message for repo mentions
    for _, repo := range repos {
        if strings.Contains(message, repo.Name) {
            return &repo
        }
    }

    // Default: return first repo or ask user
    if len(repos) > 0 {
        return &repos[0]
    }

    return nil
}
```

### 5. Claude Code Executor (`internal/claude/`)

**Purpose**: Execute Claude Code SDK in repository context

**Key Files**:
- `executor.go` - Spawn Claude Code process
- `session.go` - Manage conversation sessions
- `tools.go` - Handle tool use results

**Execution Flow**:
```go
func ExecuteInRepo(repo Repository, prompt string, sessionID string) (string, error) {
    // 1. Prepare working directory
    workDir := repo.Path

    // 2. Build Claude Code command
    cmd := exec.Command("claude",
        "--non-interactive",
        "--session-id", sessionID,
        prompt,
    )
    cmd.Dir = workDir

    // 3. Set environment
    cmd.Env = append(os.Environ(),
        "CLAUDE_CODE_OAUTH_TOKEN=" + os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
    )

    // 4. Execute and capture output
    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", err
    }

    return string(output), nil
}
```

**Session Management**:
- Each repo has its own session ID
- Sessions persist across messages
- Stored in `~/.claude/sessions/{repo-name}/`
- Allows multi-turn conversations

**Alternative Approach (SDK as library)**:
```go
// If Claude Code SDK provides a Go library in future
import "github.com/anthropic-ai/claude-code-go"

func ExecuteWithSDK(repo Repository, prompt string) (string, error) {
    sdk := claude.NewSDK(claude.Config{
        OAuthToken: os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
        WorkDir: repo.Path,
    })

    result, err := sdk.Query(claude.QueryOptions{
        Prompt: prompt,
        Context: repo.Context, // CLAUDE.md content
        Tools: []string{"bash", "read", "write", "edit"},
    })

    return result.Output, err
}
```

### 6. Config Manager (`internal/config/`)

**Purpose**: Load and validate configuration

**Key Files**:
- `load.go` - Read config.json
- `types.go` - Config struct definitions

**Config Struct**:
```go
type Config struct {
    WhatsApp WhatsAppConfig `json:"whatsapp"`
    OpenAI   OpenAIConfig   `json:"openai"`
    Claude   ClaudeConfig   `json:"claudeCode"`
    Sources  SourcesConfig  `json:"sources"`
}

type WhatsAppConfig struct {
    SessionPath    string         `json:"sessionPath"`
    PersonalNumber string         `json:"personalNumber"`
    AllowedGroups  []AllowedGroup `json:"allowedGroups"`
}

type AllowedGroup struct {
    JID          string `json:"jid"`
    Name         string `json:"name"`
    Enabled      bool   `json:"enabled"`
    IsDevControl bool   `json:"isDevControl"` // True for "CodeButler Developer" group
}

type OpenAIConfig struct {
    APIKey string `json:"apiKey"`
}

type ClaudeConfig struct {
    OAuthToken string `json:"oauthToken"`
}

type SourcesConfig struct {
    RootPath string `json:"rootPath"`
}
```

**Loading Logic**:
```go
func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    // Validate
    if cfg.WhatsApp.PersonalNumber == "" {
        return nil, errors.New("personalNumber is required")
    }

    // Load OAuth token from env if not in config
    if cfg.Claude.OAuthToken == "" {
        cfg.Claude.OAuthToken = os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")
    }

    return &cfg, nil
}
```

## Main Application Flow

### main.go Structure

```go
package main

import (
    "codebutler/internal/whatsapp"
    "codebutler/internal/access"
    "codebutler/internal/audio"
    "codebutler/internal/repo"
    "codebutler/internal/claude"
    "codebutler/internal/config"
)

func main() {
    // 1. Check if first run
    if !fileExists("config.json") {
        runFirstTimeSetup()
        return
    }

    // 2. Load configuration
    cfg, err := config.Load("config.json")
    if err != nil {
        log.Fatal("Failed to load config:", err)
    }

    // 3. Discover repositories
    repos, err := repo.DiscoverRepos(cfg.Sources.RootPath)
    if err != nil {
        log.Fatal("Failed to discover repos:", err)
    }
    log.Printf("Found %d repositories", len(repos))

    // 4. Connect to WhatsApp
    wa, err := whatsapp.Connect(cfg.WhatsApp.SessionPath)
    if err != nil {
        log.Fatal("Failed to connect to WhatsApp:", err)
    }
    defer wa.Disconnect()

    // 5. Register message handler
    wa.OnMessage(func(msg whatsapp.Message) {
        // Access control
        if !access.IsAllowed(msg.Sender, cfg) {
            log.Printf("Rejected message from: %s", msg.Sender)
            return
        }

        // Handle voice messages
        content := msg.Content
        if msg.IsVoice {
            audioPath := audio.Download(msg.MediaURL)
            transcript, _ := audio.Transcribe(audioPath, cfg.OpenAI.APIKey)
            content = transcript
        }

        // Route to repository
        targetRepo := repo.RouteMessage(content, repos)
        if targetRepo == nil {
            wa.SendMessage(msg.Sender, "No repository found. Use: in <repo-name>: <message>")
            return
        }

        // Execute Claude Code
        sessionID := getSessionID(msg.Sender, targetRepo.Name)
        result, err := claude.Execute(targetRepo, content, sessionID)
        if err != nil {
            wa.SendMessage(msg.Sender, "Error: " + err.Error())
            return
        }

        // Send response
        wa.SendMessage(msg.Sender, result)
    })

    // 6. Keep alive
    log.Println("CodeButler is running...")
    select {} // Block forever
}

func runFirstTimeSetup() {
    fmt.Println("=== CodeButler First Time Setup ===")

    // WhatsApp auth
    wa, err := whatsapp.Connect("./whatsapp-session")
    if err != nil {
        log.Fatal(err)
    }

    // Get personal number
    info := wa.GetInfo()
    personalNumber := info.Wid

    fmt.Printf("✅ Connected as: %s\n\n", personalNumber)

    // Prompt for OpenAI key
    fmt.Print("Enter OpenAI API Key: ")
    var openaiKey string
    fmt.Scanln(&openaiKey)

    fmt.Println("\n🔍 Searching for 'CodeButler Developer' group...")

    // Find or create CodeButler Developer group
    alfredGroup, err := findOrCreateAlfredDeveloperGroup(wa, personalNumber)
    if err != nil {
        log.Fatal("Failed to setup CodeButler Developer group:", err)
    }

    fmt.Printf("✅ CodeButler Developer group ready: %s\n", alfredGroup.Name)

    // Create config
    cfg := config.Config{
        WhatsApp: config.WhatsAppConfig{
            SessionPath:    "./whatsapp-session",
            PersonalNumber: personalNumber,
            AllowedGroups: []config.AllowedGroup{
                {
                    JID:          alfredGroup.JID,
                    Name:         alfredGroup.Name,
                    Enabled:      true,
                    IsDevControl: true, // Special flag for dev control
                },
            },
        },
        OpenAI: config.OpenAIConfig{
            APIKey: openaiKey,
        },
        Claude: config.ClaudeConfig{
            OAuthToken: "from-env-CLAUDE_CODE_OAUTH_TOKEN",
        },
        Sources: config.SourcesConfig{
            RootPath: "./Sources",
        },
    }

    // Save config
    data, _ := json.MarshalIndent(cfg, "", "  ")
    os.WriteFile("config.json", data, 0600)

    // Create .gitignore
    gitignore := `config.json
whatsapp-session/
*.log
claude
`
    os.WriteFile(".gitignore", []byte(gitignore), 0644)

    // Create Sources directory
    os.MkdirAll("Sources", 0755)

    fmt.Println("\n✅ Setup complete!")

    // Send welcome message to CodeButler Developer group
    welcomeMsg := `🤖 CodeButler connected ✅

You can now control your development workflow from here.

Try these commands:
- @codebutler repos
- @codebutler status
- @codebutler help`

    wa.SendMessage(alfredGroup.JID, welcomeMsg)
    fmt.Println("📱 Welcome message sent to CodeButler Developer group")
    fmt.Println("\nRun './butler' to start")

    wa.Disconnect()
}

// findOrCreateAlfredDeveloperGroup finds existing or creates new CodeButler Developer group
func findOrCreateAlfredDeveloperGroup(wa *whatsapp.Client, personalNumber string) (*whatsapp.Group, error) {
    // Get all groups
    groups, err := wa.GetGroups()
    if err != nil {
        return nil, fmt.Errorf("failed to get groups: %w", err)
    }

    // Search for existing "CodeButler Developer" group
    for _, group := range groups {
        if group.Name == "CodeButler Developer" {
            fmt.Println("   Found existing 'CodeButler Developer' group")
            return &group, nil
        }
    }

    // Group doesn't exist, create it
    fmt.Println("   'CodeButler Developer' group not found, creating...")

    groupJID, err := wa.CreateGroup("CodeButler Developer", []string{})
    if err != nil {
        return nil, fmt.Errorf("failed to create group: %w", err)
    }

    fmt.Println("   ✅ Created 'CodeButler Developer' group")

    return &whatsapp.Group{
        JID:  groupJID,
        Name: "CodeButler Developer",
    }, nil
}
```

## Development Workflow

### Building

```bash
# Development build
go build -o claude main.go

# Production build (optimized)
go build -ldflags="-s -w" -o claude main.go

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o claude-linux main.go
```

### Testing

```bash
# Unit tests
go test ./internal/...

# Integration test
go test -tags=integration ./...

# Test with specific repo
./butler --test-repo Sources/my-repo
```

### Debugging

```bash
# Enable debug logging
./butler --debug

# View WhatsApp messages
./butler --debug-whatsapp

# Test Whisper API
./butler --test-whisper /path/to/audio.ogg
```

## Integration Points

### 1. WhatsApp ←→ Go
- Library: `go-whatsapp`
- Protocol: WhatsApp Web (WebSocket)
- Auth: QR code scan (one-time)
- Session: Persistent in `whatsapp-session/`

### 2. Go ←→ Claude Code SDK
- Method: Spawn subprocess (`exec.Command`)
- Communication: stdin/stdout
- Working Dir: Repository path
- Session: Per-repo session IDs

### 3. Go ←→ Whisper API
- API: REST (HTTPS)
- Endpoint: `https://api.openai.com/v1/audio/transcriptions`
- Format: multipart/form-data
- Cost: ~$0.006 per minute

### 4. Go ←→ File System
- Repos: `Sources/` directory scan
- Context: Read `CLAUDE.md` files
- Config: Read/write `config.json`
- Session: Delegate to Claude Code SDK

## Error Handling Strategy

### WhatsApp Connection Errors
```go
// Retry with exponential backoff
for retries := 0; retries < 5; retries++ {
    wa, err := whatsapp.Connect(sessionPath)
    if err == nil {
        return wa, nil
    }

    backoff := time.Duration(math.Pow(2, float64(retries))) * time.Second
    log.Printf("Connection failed, retrying in %v...", backoff)
    time.Sleep(backoff)
}
```

### Claude Code Execution Errors
```go
result, err := claude.Execute(repo, prompt, sessionID)
if err != nil {
    // Send friendly error to user
    errorMsg := fmt.Sprintf(
        "Sorry, I encountered an error:\n\n%s\n\nTry rephrasing your question.",
        err.Error(),
    )
    wa.SendMessage(sender, errorMsg)
    return
}
```

### Whisper API Errors
```go
transcript, err := audio.Transcribe(audioPath, apiKey)
if err != nil {
    // Fallback: ask user to type
    wa.SendMessage(sender,
        "I couldn't transcribe your voice message. Could you type it instead?")
    return
}
```

## Security Considerations

### 1. Config File Protection
- `config.json` has mode 0600 (owner read/write only)
- Never committed to git (in `.gitignore`)
- Contains sensitive tokens

### 2. Access Control
- Default deny all unknown senders
- Personal number has full privileges
- Groups require explicit approval

### 3. Repository Isolation
- Each repo has its own CLAUDE.md context
- Sessions don't leak between repos
- File operations limited to repo directory

### 4. Credential Storage
- OAuth token from environment variable (preferred)
- OpenAI key in config.json (encrypted at rest recommended)
- WhatsApp session encrypted by library

## Performance Optimization

### 1. Concurrent Message Handling
```go
// Use goroutines for parallel processing
go func(msg whatsapp.Message) {
    handleMessage(msg)
}(msg)
```

### 2. Repository Cache
```go
var repoCache struct {
    sync.RWMutex
    repos []Repository
    lastScan time.Time
}

func GetRepos() []Repository {
    repoCache.RLock()
    if time.Since(repoCache.lastScan) < 5*time.Minute {
        defer repoCache.RUnlock()
        return repoCache.repos
    }
    repoCache.RUnlock()

    // Re-scan
    repoCache.Lock()
    defer repoCache.Unlock()
    repoCache.repos = repo.DiscoverRepos("./Sources")
    repoCache.lastScan = time.Now()
    return repoCache.repos
}
```

### 3. Session Reuse
- Keep Claude Code sessions alive
- Avoid re-initialization overhead
- Clear old sessions after 24h

## Future Enhancements

### Near-term
- [ ] Web UI for config management
- [ ] Group chat statistics
- [ ] Rate limiting per sender
- [ ] Scheduled tasks (cron-like)

### Mid-term
- [ ] Multi-language support (Spanish, etc.)
- [ ] Custom tool definitions
- [ ] Repository templates
- [ ] Backup/restore config

### Long-term
- [ ] Plugin system for custom commands
- [ ] Web dashboard for monitoring
- [ ] Multiple AI providers (fallback)
- [ ] Self-hosting documentation

## Troubleshooting

### WhatsApp Won't Connect
```bash
# Delete session and re-auth
rm -rf whatsapp-session/
./butler  # Will show QR again
```

### Claude Code Not Found
```bash
# Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# Verify installation
which claude
```

### Whisper API Errors
```bash
# Test API key
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

### Repository Not Detected
```bash
# Verify Sources/ structure
ls -la Sources/

# Each repo should have .git/
ls -la Sources/your-repo/.git/
```

---

This document should give you (Claude Code) complete context for helping build CodeButler. When implementing features, refer to the component breakdown and code examples above.

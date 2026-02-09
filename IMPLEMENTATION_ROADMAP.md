# CodeButler - Implementation Roadmap

This document breaks down all the tasks needed to build CodeButler from scratch in Go.

---

## Project Setup

### Phase 0: Bootstrap (Est: 2 hours)

- [ ] Create project structure
  ```bash
  mkdir codebutler && cd codebutler
  go mod init github.com/yourusername/codebutler
  mkdir -p internal/{whatsapp,access,audio,repo,claude,config}
  mkdir -p Sources
  touch main.go
  touch README.md
  ```

- [ ] Initialize git repository
  ```bash
  git init
  echo "config.json\nwhatsapp-session/\n*.log\nclaude\n" > .gitignore
  ```

- [ ] Add dependencies to go.mod
  ```bash
  go get github.com/Rhymen/go-whatsapp
  go get github.com/mattn/go-sqlite3
  go get github.com/skip2/go-qrcode  # For QR display
  ```

- [ ] Create basic main.go skeleton
  ```go
  package main

  import "fmt"

  func main() {
      fmt.Println("CodeButler starting...")
  }
  ```

- [ ] Test build: `go build -o claude main.go`

---

## Phase 1: WhatsApp Integration (Est: 8-12 hours)

### 1.1 WhatsApp Client (`internal/whatsapp/client.go`)

- [ ] Create Client struct
  ```go
  type Client struct {
      conn      *whatsapp.Conn
      sessionPath string
  }
  ```

- [ ] Implement Connect() function
  - [ ] Load or create session
  - [ ] Handle QR code generation
  - [ ] Display QR in terminal
  - [ ] Wait for connection

- [ ] Implement Disconnect() function

- [ ] Implement GetInfo() to get personal number

- [ ] Test: Run and scan QR, verify connection

### 1.2 Message Handling (`internal/whatsapp/handler.go`)

- [ ] Create Message struct
  ```go
  type Message struct {
      Sender    string
      Content   string
      Timestamp int64
      IsVoice   bool
      MediaURL  string
  }
  ```

- [ ] Implement OnMessage() callback registration

- [ ] Implement SendMessage() function
  ```go
  func (c *Client) SendMessage(jid string, text string) error
  ```

- [ ] Implement message event handler
  - [ ] Parse text messages
  - [ ] Detect voice messages
  - [ ] Extract media URLs

- [ ] Test: Send/receive messages

### 1.3 Media Download (`internal/whatsapp/media.go`)

- [ ] Implement DownloadAudio() function
  ```go
  func (c *Client) DownloadAudio(msg whatsapp.AudioMessage) (string, error)
  ```

- [ ] Save to temp directory with timestamp
- [ ] Return local file path
- [ ] Test: Download a voice message

### 1.4 Group Management (`internal/whatsapp/groups.go`)

- [ ] Create Group struct
  ```go
  type Group struct {
      JID  string
      Name string
  }
  ```

- [ ] Implement GetGroups() function
  ```go
  func (c *Client) GetGroups() ([]Group, error)
  ```

- [ ] Implement CreateGroup() function
  ```go
  func (c *Client) CreateGroup(name string, participants []string) (string, error)
  ```

- [ ] Implement GetGroupInfo() function

- [ ] Implement AddParticipants() function

- [ ] Implement RemoveParticipants() function

- [ ] Test: List groups, create test group

### 1.5 QR Code Display (`internal/whatsapp/auth.go`)

- [ ] Implement DisplayQR() function
  ```go
  func DisplayQR(qrCode string) error
  ```

- [ ] Use skip2/go-qrcode or print ASCII QR to terminal

- [ ] Test: Display QR and scan with phone

---

## Phase 2: Configuration System (Est: 4-6 hours)

### 2.1 Config Types (`internal/config/types.go`)

- [ ] Define Config struct
  ```go
  type Config struct {
      WhatsApp WhatsAppConfig `json:"whatsapp"`
      OpenAI   OpenAIConfig   `json:"openai"`
      Claude   ClaudeConfig   `json:"claudeCode"`
      Sources  SourcesConfig  `json:"sources"`
  }
  ```

- [ ] Define WhatsAppConfig struct
  ```go
  type WhatsAppConfig struct {
      SessionPath    string         `json:"sessionPath"`
      PersonalNumber string         `json:"personalNumber"`
      AllowedGroups  []AllowedGroup `json:"allowedGroups"`
  }
  ```

- [ ] Define AllowedGroup struct
  ```go
  type AllowedGroup struct {
      JID          string `json:"jid"`
      Name         string `json:"name"`
      Enabled      bool   `json:"enabled"`
      IsDevControl bool   `json:"isDevControl"`
  }
  ```

- [ ] Define OpenAIConfig, ClaudeConfig, SourcesConfig

### 2.2 Config Loading (`internal/config/load.go`)

- [ ] Implement Load() function
  ```go
  func Load(path string) (*Config, error)
  ```

- [ ] Read JSON file

- [ ] Parse with json.Unmarshal

- [ ] Validate required fields

- [ ] Load OAuth token from env if not in config

- [ ] Test: Create sample config.json and load it

### 2.3 Config Saving (`internal/config/save.go`)

- [ ] Implement Save() function
  ```go
  func Save(cfg *Config, path string) error
  ```

- [ ] Marshal to JSON with indentation

- [ ] Write to file with permissions 0600

- [ ] Test: Save and load config

---

## Phase 3: Access Control (Est: 1 hour)

### 3.1 Access Validation (`internal/access/control.go`)

- [ ] Implement IsAllowed() function
  ```go
  func IsAllowed(sender string, cfg *config.Config) bool {
      // Ultra-simple: only check if message is from CodeButler Developer group
      return sender == cfg.WhatsApp.GroupJID
  }
  ```

- [ ] Test: Verify access control logic
  - [ ] Message from CodeButler Developer group → Allowed
  - [ ] Message from personal chat → Denied
  - [ ] Message from other group → Denied

**Note**: This is much simpler than multi-group access control. No arrays, no loops, no enable/disable flags. Just one JID comparison.

---

## Phase 4: Audio Transcription (Est: 4-5 hours)

### 4.1 Whisper API Client (`internal/audio/transcribe.go`)

- [ ] Implement TranscribeAudio() function
  ```go
  func TranscribeAudio(audioPath string, apiKey string) (string, error)
  ```

- [ ] Open audio file

- [ ] Create multipart form data

- [ ] Add file to form

- [ ] Add model field ("whisper-1")

- [ ] Make HTTP POST to OpenAI API

- [ ] Parse JSON response

- [ ] Extract text field

- [ ] Return transcription

- [ ] Test: Transcribe sample audio file

### 4.2 Audio Download Helper (`internal/audio/download.go`)

- [ ] Implement SaveAudio() function

- [ ] Create temp directory if needed

- [ ] Generate unique filename

- [ ] Save audio buffer to file

- [ ] Test: Save and verify audio file

---

## Phase 5: Repository Management (Est: 6-8 hours)

### 5.1 Repository Discovery (`internal/repo/manager.go`)

- [ ] Create Repository struct
  ```go
  type Repository struct {
      Name    string
      Path    string
      Context string // CLAUDE.md content
  }
  ```

- [ ] Implement DiscoverRepos() function
  ```go
  func DiscoverRepos(sourcesPath string) ([]Repository, error)
  ```

- [ ] Read Sources/ directory

- [ ] Check each subdirectory for .git/

- [ ] Load CLAUDE.md if exists

- [ ] Return list of repos

- [ ] Test: Create test repos, verify discovery

### 5.2 Repository Routing (`internal/repo/router.go`)

- [ ] Implement RouteMessage() function
  ```go
  func RouteMessage(message string, repos []Repository) *Repository
  ```

- [ ] Parse explicit routing: "in repo-name: message"

- [ ] Fallback to implicit detection (repo name in message)

- [ ] Default to first repo or ask user

- [ ] Test: Verify routing logic

### 5.3 Context Loading (`internal/repo/context.go`)

- [ ] Implement LoadContext() function
  ```go
  func LoadContext(repoPath string) (string, error)
  ```

- [ ] Read CLAUDE.md file

- [ ] Return content as string

- [ ] Handle missing file gracefully

- [ ] Test: Load context from test repo

---

## Phase 6: Claude Code Executor (Est: 8-10 hours)

### 6.1 Command Execution (`internal/claude/executor.go`)

- [ ] Implement Execute() function
  ```go
  func Execute(repo *Repository, prompt string, sessionID string) (string, error)
  ```

- [ ] Build claude CLI command

- [ ] Set working directory to repo path

- [ ] Set environment variables (CLAUDE_CODE_OAUTH_TOKEN)

- [ ] Execute command with exec.Command

- [ ] Capture stdout/stderr

- [ ] Handle errors

- [ ] Return output

- [ ] Test: Execute simple command

### 6.2 Session Management (`internal/claude/session.go`)

- [ ] Create SessionManager struct

- [ ] Implement GetOrCreateSession() function
  ```go
  func GetOrCreateSession(repoName string) string
  ```

- [ ] Generate session IDs

- [ ] Store session mapping (repo -> sessionID)

- [ ] Persist sessions across restarts

- [ ] Test: Verify session continuity

### 6.3 Tool Handling (`internal/claude/tools.go`)

- [ ] Parse tool use from output (if needed)

- [ ] Handle special tools (future)

- [ ] Test: Verify tool execution

---

## Phase 7: Main Application (Est: 10-12 hours)

### 7.1 First Time Setup (`main.go`)

- [ ] Implement fileExists() helper
  ```go
  func fileExists(path string) bool
  ```

- [ ] Implement runFirstTimeSetup() function
  - [ ] Print banner
  - [ ] Connect to WhatsApp
  - [ ] Display QR code
  - [ ] Get personal number
  - [ ] Prompt for OpenAI API key
  - [ ] Call findOrCreateAlfredDeveloperGroup()
  - [ ] Create config struct
  - [ ] Save config to file
  - [ ] Create .gitignore
  - [ ] Create Sources/ directory
  - [ ] Send welcome message to CodeButler Developer group
  - [ ] Print success message

- [ ] Implement findOrCreateAlfredDeveloperGroup() function
  ```go
  func findOrCreateAlfredDeveloperGroup(wa *whatsapp.Client, personalNumber string) (*whatsapp.Group, error)
  ```

  - [ ] Get all groups
  - [ ] Search for "CodeButler Developer"
  - [ ] If found, return it
  - [ ] If not found, create new group
  - [ ] Return group info

- [ ] Test: Run first-time setup end-to-end

### 7.2 Main Loop (`main.go`)

- [ ] Implement main() function
  - [ ] Check if first run (config.json exists?)
  - [ ] If first run, call runFirstTimeSetup()
  - [ ] Load configuration
  - [ ] Discover repositories
  - [ ] Connect to WhatsApp
  - [ ] Register message handler
  - [ ] Keep alive (select {})

- [ ] Implement message handler
  - [ ] Access control check
  - [ ] Handle voice messages (download + transcribe)
  - [ ] Route to repository
  - [ ] Execute Claude Code
  - [ ] Send response

- [ ] Test: End-to-end message flow

### 7.3 Error Handling

- [ ] Add error handling to all components

- [ ] Log errors appropriately

- [ ] Send error messages to user

- [ ] Implement retry logic where needed

- [ ] Test: Verify graceful error handling

---

## Phase 8: Advanced Features (Est: 6-8 hours)

### 8.1 Dev Control Commands (`internal/devcontrol/commands.go`)

- [ ] Implement IsDevControlGroup() helper

- [ ] Implement handleStatus() command

- [ ] Implement handleRepos() command

- [ ] Implement handleBulkTest() command

- [ ] Test: Verify dev control commands

### 8.2 Bulk Operations (`internal/repo/bulk.go`)

- [ ] Implement RunInAllRepos() function
  ```go
  func RunInAllRepos(repos []Repository, command string) []Result
  ```

- [ ] Execute command in parallel (goroutines)

- [ ] Collect results

- [ ] Format summary

- [ ] Test: Run test command in all repos

### 8.3 System Monitoring (`internal/system/monitor.go`)

- [ ] Implement GetSystemStatus() function

- [ ] Track active sessions

- [ ] Monitor memory usage

- [ ] Track uptime

- [ ] Test: Verify status reporting

---

## Phase 9: Testing (Est: 8-10 hours)

### 9.1 Unit Tests

- [ ] Write tests for internal/config/
  - [ ] Test Load()
  - [ ] Test Save()
  - [ ] Test validation

- [ ] Write tests for internal/access/
  - [ ] Test IsAllowed()
  - [ ] Test group management

- [ ] Write tests for internal/repo/
  - [ ] Test DiscoverRepos()
  - [ ] Test RouteMessage()

- [ ] Write tests for internal/audio/
  - [ ] Mock Whisper API
  - [ ] Test TranscribeAudio()

- [ ] Run all tests: `go test ./internal/...`

### 9.2 Integration Tests

- [ ] Test WhatsApp connection flow

- [ ] Test message send/receive

- [ ] Test first-time setup

- [ ] Test repository discovery

- [ ] Test Claude Code execution

- [ ] Run integration tests: `go test -tags=integration ./...`

### 9.3 End-to-End Tests

- [ ] Test complete flow: message → response

- [ ] Test voice message transcription

- [ ] Test multi-repo routing

- [ ] Test CodeButler Developer group creation

- [ ] Verify all features work together

---

## Phase 10: Documentation (Est: 4-6 hours)

### 10.1 Code Documentation

- [ ] Add godoc comments to all exported functions

- [ ] Add package documentation

- [ ] Generate docs: `go doc -all`

### 10.2 User Documentation

- [ ] Create README.md with:
  - [ ] Installation instructions
  - [ ] Quick start guide
  - [ ] Configuration reference
  - [ ] Command reference

- [ ] Create CONTRIBUTING.md

- [ ] Create examples/

- [ ] Add screenshots (if applicable)

### 10.3 Developer Documentation

- [ ] Document architecture decisions

- [ ] Add inline comments for complex logic

- [ ] Create DEVELOPMENT.md

---

## Phase 11: Build & Deploy (Est: 2-3 hours)

### 11.1 Build Optimization

- [ ] Optimize build flags
  ```bash
  go build -ldflags="-s -w" -o claude main.go
  ```

- [ ] Test binary size

- [ ] Test startup time

### 11.2 Cross-Compilation

- [ ] Build for Linux
  ```bash
  GOOS=linux GOARCH=amd64 go build -o claude-linux main.go
  ```

- [ ] Build for macOS (Intel)
  ```bash
  GOOS=darwin GOARCH=amd64 go build -o claude-macos-intel main.go
  ```

- [ ] Build for macOS (Apple Silicon)
  ```bash
  GOOS=darwin GOARCH=arm64 go build -o claude-macos-arm main.go
  ```

- [ ] Test on each platform

### 11.3 Release

- [ ] Create GitHub repository

- [ ] Push code

- [ ] Create v0.1.0 tag

- [ ] Create GitHub Release

- [ ] Add binaries as release assets

---

## Time Estimates Summary

| Phase | Description | Estimated Hours |
|-------|-------------|-----------------|
| 0 | Project Setup | 2 |
| 1 | WhatsApp Integration | 8-12 |
| 2 | Configuration System | 4-6 |
| 3 | Access Control | 1 |
| 4 | Audio Transcription | 4-5 |
| 5 | Repository Management | 6-8 |
| 6 | Claude Code Executor | 8-10 |
| 7 | Main Application | 10-12 |
| 8 | Advanced Features | 6-8 |
| 9 | Testing | 8-10 |
| 10 | Documentation | 4-6 |
| 11 | Build & Deploy | 2-3 |
| **TOTAL** | | **63-83 hours** |

**Realistic timeline**: 2-3 weeks of full-time work, or 4-6 weeks part-time.

**Note**: Phase 3 is significantly simpler now (1h vs 2-3h) due to single-group access control.

---

## Quick Start Checklist

To get a minimal viable version running quickly, focus on:

### MVP Scope (Est: 23-31 hours)

- [x] Phase 0: Project Setup (2h)
- [ ] Phase 1.1-1.3: Basic WhatsApp (6h)
- [ ] Phase 2: Configuration (5h)
- [ ] Phase 3: Access Control (1h)
- [ ] Phase 5: Repository Discovery (4h)
- [ ] Phase 6.1: Basic Claude Executor (4h)
- [ ] Phase 7.1-7.2: Main App (8h)
- [ ] Basic Testing (3h)

This gets you:
- WhatsApp connection
- Message send/receive
- Repository discovery
- Basic Claude Code execution
- No voice messages (add later)
- No CodeButler Developer group (add later)
- No bulk operations (add later)

---

## Development Tips

### Incremental Development

1. **Start simple**: Get WhatsApp connection working first
2. **Test frequently**: After each component, test it
3. **Use stubs**: Mock external APIs initially
4. **Iterate**: Build MVP, then add features

### Common Pitfalls

1. **WhatsApp QR timeout**: QR expires after 20 seconds
2. **Session corruption**: Delete whatsapp-session/ if issues
3. **Group JID format**: Must be `@g.us` not `@s.whatsapp.net`
4. **Claude Code path**: May not be in PATH, use full path
5. **OAuth token**: Must be set in environment, not just config

### Debugging

```bash
# Enable verbose logging
./butler --debug

# Test WhatsApp connection only
go run main.go --test-whatsapp

# Test repository discovery
go run main.go --list-repos

# Test Whisper API
go run main.go --test-whisper /path/to/audio.ogg
```

---

## Next Steps

1. ✅ Review this roadmap
2. ⬜ Set up development environment
3. ⬜ Complete Phase 0 (Project Setup)
4. ⬜ Work through phases sequentially
5. ⬜ Test each phase before moving on
6. ⬜ Deploy and iterate!

**Good luck building CodeButler!** 🚀

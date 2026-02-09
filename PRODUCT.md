# CodeButler - Multi-Repo Claude Code Controller

## Overview

CodeButler is a minimalist WhatsApp-based assistant built in Go that acts as a specialized controller for Claude Code across multiple code repositories. It enables developers to interact with Claude Code through WhatsApp, managing multiple projects from a single interface.

## Core Philosophy

- **Minimal & Focused**: Built for one purpose - multi-repo code assistance via WhatsApp
- **Security First**: Explicit allow-list for personal chat and approved groups only
- **Multi-Repo Native**: Designed to work across many repositories simultaneously
- **SDK-Based**: Uses Claude Code SDK (not Claude API) to leverage existing subscription
- **Developer-Friendly**: Simple Go binary with JSON configuration

## Key Features

### 1. WhatsApp Integration
- Connect via WhatsApp Web protocol (using Go WhatsApp library)
- QR code authentication on first run
- Persistent session management
- **Auto-creates "CodeButler Developer" group** on first run
  - Private group (only you as member)
  - Used as development control center
  - Auto-enabled in allowed groups
- Access control:
  - Personal chat (1:1 with your number)
  - "CodeButler Developer" group (auto-created)
  - Other explicitly approved groups
  - Interactive group ID discovery via WhatsApp

### 2. Multi-Repository Management
```
project-root/
├── claude                    # Main binary
├── config.json              # WhatsApp auth + tokens (gitignored)
├── Sources/                 # Multi-repo workspace
│   ├── repo-a/
│   │   └── CLAUDE.md        # Repo-specific context
│   ├── repo-b/
│   │   └── CLAUDE.md
│   └── repo-c/
│       └── CLAUDE.md
└── .gitignore
```

### 3. Claude Code SDK Integration
- Uses `@anthropic-ai/claude-code` SDK
- Leverages your existing Claude Pro/Max subscription
- No additional API costs
- Per-repository context isolation via CLAUDE.md

### 4. Audio Transcription
- OpenAI Whisper API integration
- Automatic voice message transcription
- Configured via `OPENAI_API_KEY` in config.json

### 5. Simple CLI
```bash
# First run - interactive setup
./butler

# Displays:
# 1. WhatsApp QR code
# 2. Prompts for OpenAI API key
# 3. Creates/finds "CodeButler Developer" group
# 4. Creates config.json
# 5. Starts listening

# Subsequent runs
./butler

# Reads config.json and starts immediately
```

## Architecture

### High-Level Flow

```
WhatsApp Message
    ↓
Go Server (CodeButler)
    ↓
Access Control Check (personal chat or allowed group?)
    ↓
Audio? → Transcribe with Whisper API
    ↓
Identify Target Repository (via context/routing)
    ↓
Execute Claude Code SDK in repo context
    ↓
Response → WhatsApp
```

### Components

#### 1. WhatsApp Handler (Go)
- Connection management
- Message reception/sending
- Media download (voice messages)
- Group metadata fetching

#### 2. Access Control (Go)
- Validate sender is personal number OR allowed group
- Config-based allow-list
- Interactive group approval flow

#### 3. Audio Processor (Go)
- Download voice messages
- Call OpenAI Whisper API
- Return transcription text

#### 4. Repository Router (Go)
- Determine which repo the request targets
- Context switching between repos
- CLAUDE.md loading

#### 5. Claude Code Executor (Go → SDK)
- Spawn Claude Code SDK process
- Pass repository context
- Stream responses
- Handle tool executions

## Configuration File

### config.json Structure

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

### .gitignore

```
config.json
whatsapp-session/
*.log
claude
```

## User Workflows

### Initial Setup

```
1. Developer runs: ./butler
2. System prompts: "First time setup detected"
3. Shows WhatsApp QR code in terminal
4. Developer scans with phone
5. System prompts: "Enter OpenAI API Key:"
6. Developer enters key
7. System searches for "CodeButler Developer" group
   - If found: Uses existing group
   - If not found: Creates new group with you as only member
8. System creates config.json with "CodeButler Developer" pre-enabled
9. System starts listening
10. Sends welcome message to "CodeButler Developer" group:
    "🤖 CodeButler connected ✅

    You can now control your development workflow from here.
    Try: @codebutler repos"
```

### Adding a Group

```
WhatsApp Conversation:
User: @codebutler list groups
Bot: Available groups:
     1. Team Alpha (123-456@g.us)
     2. Project Beta (789-012@g.us)

User: @codebutler allow 1
Bot: ✅ Team Alpha is now allowed
     Updated config.json
```

### Working with Repositories

```
WhatsApp (in "CodeButler Developer" group):
User: @codebutler repos
Bot: Available repositories:
     1. api-service
     2. frontend-app
     3. mobile-client

User: [voice message: "in the api service, add logging to the user controller"]
Bot: [transcribes audio]
     Working in: api-service
     [Claude Code executes in Sources/api-service/]

     ✅ Added logging to UserController:
     - src/controllers/user.go:45

     Changes:
     ```go
     func (c *UserController) GetUser(id string) {
         log.Info("GetUser called", "id", id)
         // ... existing code
     }
     ```
```

### Using "CodeButler Developer" as Control Center

```
The "CodeButler Developer" group is your personal command center:

1. Development Status:
   User: @codebutler status
   Bot: 📊 System Status
        - Active repos: 3
        - Running sessions: 1 (api-service)
        - Queue: empty
        - Last activity: 2m ago

2. Bulk Operations:
   User: @codebutler run tests in all repos
   Bot: Running tests across 3 repositories...

        ✅ api-service: 45 passed
        ✅ frontend-app: 123 passed
        ❌ mobile-client: 2 failed

3. Workflow Automation:
   User: @codebutler create PR workflow for api-service
   Bot: Created workflow:
        1. Run tests
        2. Run linter
        3. Build Docker image
        4. If all pass, create PR

        Save as scheduled task? (yes/no)
```

## "CodeButler Developer" Group - Your Dev Control Center

### What is it?

"CodeButler Developer" is a special WhatsApp group automatically created during first-time setup. It's a private group (only you as member) that serves as your dedicated development command center.

### Why a Dedicated Group?

**Separation of Concerns:**
- Personal chat: For quick questions, casual use
- CodeButler Developer: For serious development work, workflows, automation

**Better Organization:**
- All dev commands in one place
- Easy to review command history
- No mixing with personal messages

**Enhanced Capabilities:**
- System-wide operations
- Bulk commands across all repos
- Workflow automation
- Debug and monitoring tools

### Auto-Creation Flow

```
First Run Setup:
1. Connect to WhatsApp via QR
2. Search for existing "CodeButler Developer" group
   - If found: Use it ✅
   - If not found: Create new group ✨
3. Add to config.json with isDevControl: true
4. Send welcome message to the group
5. Ready to use!
```

### Special Commands (Dev Control Only)

These commands only work in the CodeButler Developer group:

```bash
# System monitoring
@codebutler status              # Full system status
@codebutler uptime              # How long running
@codebutler memory              # Memory usage

# Bulk operations
@codebutler run tests in all repos
@codebutler update deps in all repos
@codebutler git status in all repos

# Workflow automation
@codebutler create workflow <name>
@codebutler list workflows
@codebutler trigger workflow <name>

# Cross-repo analysis
@codebutler find function <name> in all repos
@codebutler compare <file> between repos
@codebutler dependency graph

# Configuration
@codebutler list groups         # Show all available groups
@codebutler allow group <name>  # Add group to allowed list
@codebutler deny group <name>   # Remove group from allowed list
```

### Example Workflows

**Morning Standup Automation:**
```
User: @codebutler morning standup
Bot: 🌅 Good morning! Here's your dev update:

     Yesterday's commits:
     - api-service: 3 commits, 145 lines changed
     - frontend-app: 1 commit, 23 lines changed

     Open PRs:
     - api-service: "Add user authentication" (#42)

     Failed tests: None ✅

     TODOs from CLAUDE.md:
     - [ ] api-service: Add rate limiting
     - [ ] frontend-app: Fix responsive layout

     Ready to code! 💪
```

**Pre-Commit Check:**
```
User: @codebutler pre-commit check api-service
Bot: Running pre-commit checks...

     ✅ Tests: 45 passed
     ✅ Linter: No issues
     ✅ Format: All files formatted
     ✅ Security: No vulnerabilities
     ✅ Build: Success

     All checks passed! Safe to commit. 🚀
```

**Deploy Workflow:**
```
User: @codebutler deploy api-service to staging
Bot: Deploying api-service to staging...

     Step 1/5: Running tests... ✅
     Step 2/5: Building Docker image... ✅
     Step 3/5: Pushing to registry... ✅
     Step 4/5: Updating k8s deployment... ✅
     Step 5/5: Health check... ✅

     Deployment complete! 🎉
     URL: https://api-staging.example.com
     Version: v1.2.3
```

### Multi-Repo Operations



```
User: @codebutler diff api-service frontend-app for auth flow
Bot: Analyzing authentication flow across repos...

     API Service (Sources/api-service/):
     - Uses JWT with RS256
     - Token expiry: 24h
     - Refresh tokens: Yes

     Frontend App (Sources/frontend-app/):
     - Stores tokens in localStorage
     - Auto-refresh on 401
     - Missing: Token expiry check before requests

     Recommendation: Add expiry check in frontend
```

## Security Model

### Access Control Layers

1. **Network Level**: WhatsApp Web protocol (encrypted)
2. **Application Level**: Personal number + allowed groups only
3. **Repository Level**: Each repo isolated with own CLAUDE.md
4. **Credential Storage**: config.json never committed (gitignored)

### Permissions Matrix

| Sender Type | Read Repos | Write Repos | Bulk Ops | Workflows | Modify Config | Special Commands |
|-------------|------------|-------------|----------|-----------|---------------|------------------|
| Personal Number | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| CodeButler Developer | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Other Allowed Groups | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Unknown Senders | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

## Technical Stack

### Language & Framework
- **Go 1.21+** - Main language
- **go-whatsapp** - WhatsApp Web client library
- **Claude Code SDK** - AI execution (Node.js called from Go)

### Dependencies
```go
// go.mod
module github.com/yourusername/codebutler

go 1.21

require (
    github.com/Rhymen/go-whatsapp v0.1.1  // WhatsApp client
    github.com/mattn/go-sqlite3 v1.14.18  // Message storage
    github.com/spf13/viper v1.18.2        // Config management
    gopkg.in/yaml.v3 v3.0.1               // YAML parsing
)
```

### External APIs
- **OpenAI Whisper API** - Audio transcription
- **Claude Code SDK** - AI execution (via OAuth token, not API key)

## File Structure

```
codebutler/
├── main.go                   # Entry point, CLI
├── go.mod
├── go.sum
├── config.json              # Generated, gitignored
├── .gitignore
│
├── internal/
│   ├── whatsapp/
│   │   ├── client.go        # WhatsApp connection
│   │   ├── auth.go          # QR auth flow
│   │   ├── handler.go       # Message handlers
│   │   └── media.go         # Media download
│   │
│   ├── access/
│   │   ├── control.go       # Access validation
│   │   └── groups.go        # Group management
│   │
│   ├── audio/
│   │   ├── transcribe.go    # Whisper API integration
│   │   └── download.go      # Audio file handling
│   │
│   ├── repo/
│   │   ├── manager.go       # Repository discovery
│   │   ├── router.go        # Route messages to repos
│   │   └── context.go       # Load CLAUDE.md
│   │
│   ├── claude/
│   │   ├── executor.go      # Claude Code SDK execution
│   │   ├── session.go       # Session management
│   │   └── tools.go         # Tool handling
│   │
│   └── config/
│       ├── load.go          # Config loading
│       └── types.go         # Config structs
│
├── Sources/                 # Multi-repo workspace
│   ├── .gitkeep
│   └── README.md           # Instructions for adding repos
│
└── README.md
```

## Commands Reference

### CLI Commands

```bash
# First time setup
./butler

# Normal operation
./butler

# With debug logging
./butler --debug

# Config validation
./butler --validate-config

# List repositories
./butler --list-repos
```

### WhatsApp Commands

When chatting with CodeButler:

```
@codebutler help                           # Show help
@codebutler repos                          # List available repos
@codebutler groups                         # List all WhatsApp groups
@codebutler allow <group-id>               # Allow a group
@codebutler deny <group-id>                # Deny a group
@codebutler status                         # System status
@codebutler context <repo-name>            # Show repo context (CLAUDE.md)

# Implicit repo selection (via context):
<your message>                            # Analyzed, repo auto-selected

# Explicit repo selection:
in <repo-name>: <your message>            # Explicit repo target
```

## Development Roadmap

### Phase 1: MVP (Core Functionality)
- [x] WhatsApp connection via QR
- [x] Personal chat validation
- [x] Basic message routing
- [x] Claude Code SDK integration
- [x] Single repository support
- [x] Config file management

### Phase 2: Multi-Repo (Current Focus)
- [ ] Sources/ directory scanning
- [ ] CLAUDE.md loading per repo
- [ ] Repository routing logic
- [ ] Context switching
- [ ] Multi-repo commands

### Phase 3: Voice Support
- [ ] OpenAI Whisper API integration
- [ ] Audio message detection
- [ ] Automatic transcription
- [ ] Transcribed text routing

### Phase 4: Group Management
- [ ] Group discovery via WhatsApp
- [ ] Interactive group approval
- [ ] Group-based permissions
- [ ] Config persistence

### Phase 5: Advanced Features
- [ ] Cross-repo analysis
- [ ] Diff between repos
- [ ] Batch operations
- [ ] Scheduled tasks
- [ ] Webhook support

## Use Cases

### 1. Multi-Project Developer
**Scenario**: Developer maintains 5 different microservices
**Solution**: All repos in Sources/, ask questions across all services via WhatsApp

### 2. Team Collaboration
**Scenario**: Team needs shared access to Claude Code assistant
**Solution**: Add team WhatsApp group, all members can interact

### 3. Mobile-First Coding
**Scenario**: Developer wants to make quick fixes while away from computer
**Solution**: Send voice messages from phone, CodeButler transcribes and executes

### 4. Code Review Assistant
**Scenario**: Review PRs across multiple repos
**Solution**: Ask CodeButler to analyze changes, explain diffs, suggest improvements

### 5. Learning Multiple Codebases
**Scenario**: New team member onboarding
**Solution**: Ask questions about architecture, flow, conventions across all repos

## Performance Considerations

### Go Advantages
- Fast startup time (~10ms)
- Low memory footprint (~20MB)
- Concurrent message handling (goroutines)
- Single binary deployment

### Optimizations
- Repository context caching
- Claude Code session reuse
- Lazy loading of CLAUDE.md files
- Parallel Whisper API calls for multiple audio messages

### Resource Usage
```
Idle:     ~20MB RAM, 0% CPU
Active:   ~100MB RAM, 5-10% CPU (per Claude Code session)
Audio:    ~50MB RAM spike during transcription
```

## Comparison to NanoClaw/OpenClaw

| Feature | CodeButler | NanoClaw | OpenClaw |
|---------|-----------|----------|----------|
| **Language** | Go | Node.js | Node.js |
| **Size** | ~1k LOC | ~200 LOC | ~103k LOC |
| **Focus** | Multi-repo dev | General assistant | Universal framework |
| **Channels** | WhatsApp only | WhatsApp only | 13+ channels |
| **AI Model** | Claude Code SDK | Claude Code SDK | Multi-model |
| **Repos** | Multi-repo native | Single project | Single project |
| **Voice** | Whisper API | No (optional skill) | Voice Wake + TTS |
| **Isolation** | Per-repo CLAUDE.md | Per-group containers | Session-based |
| **Deployment** | Single binary | Node + containers | Distributed |
| **Control Center** | CodeButler Developer group (auto) | Main channel (manual) | Multiple channels |
| **Bulk Operations** | Yes (across all repos) | No | No |
| **Workflow Automation** | Yes (dev control group) | Scheduled tasks | Cron + webhooks |

## Getting Started

### Prerequisites

```bash
# Install Go 1.21+
brew install go

# Install Node.js (for Claude Code SDK)
brew install node

# Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# Get OpenAI API key
# Visit: https://platform.openai.com/api-keys
```

### Installation

```bash
# Clone repository
git clone https://github.com/yourusername/codebutler
cd codebutler

# Build binary
go build -o claude main.go

# First run (interactive setup)
./butler

# Scan QR with WhatsApp
# Enter OpenAI API key when prompted

# Test connection
# Send message to yourself: "hello"
```

### Adding Your First Repository

```bash
# Navigate to Sources directory
cd Sources

# Clone your repo
git clone https://github.com/yourorg/your-repo

# Create CLAUDE.md
cat > your-repo/CLAUDE.md << EOF
# Your Repo

## Overview
This is a REST API built with Go and PostgreSQL.

## Structure
- cmd/server/ - Main application
- internal/handlers/ - HTTP handlers
- internal/models/ - Data models

## Conventions
- Use structured logging (zerolog)
- Follow Go standard project layout
- Tests in *_test.go files
EOF

# Go back to root
cd ..

# Test it
./butler --list-repos
# Should show "your-repo"
```

### Example Interaction

```
You (WhatsApp): @codebutler in your-repo: explain the auth flow

CodeButler: Analyzing auth flow in your-repo...

Based on CLAUDE.md and code:

1. Client sends POST /auth/login with credentials
2. handlers/auth.go validates against DB
3. JWT token generated (24h expiry)
4. Token returned in response

Files involved:
- internal/handlers/auth.go:45
- internal/models/user.go:23
- internal/middleware/jwt.go:12

Would you like me to show the code or explain any specific part?
```

## FAQ

### Why Go instead of Node.js?
- Single binary deployment (no node_modules)
- Lower resource usage
- Better concurrency for message handling
- Faster startup time

### How is this different from just using Claude Code directly?
- Multi-repo workspace management
- WhatsApp remote access (mobile-friendly)
- Voice message support
- Group collaboration features

### Does this violate WhatsApp ToS?
- Uses WhatsApp Web protocol (like Baileys)
- Personal use is typically safe
- Business/commercial use may violate ToS
- Use at your own risk

### Can I use Claude API instead of SDK?
- Yes, but you'll pay per-token
- SDK is free with Claude Pro/Max subscription
- See CLAUDE.md for modification instructions

### How do I backup my config?
```bash
# Config is in config.json (gitignored)
# To backup:
cp config.json config.json.backup

# Store securely (encrypted):
gpg -c config.json
```

## License

MIT License - See LICENSE file

## Contributing

Contributions welcome! See CONTRIBUTING.md for guidelines.

## Support

- GitHub Issues: https://github.com/yourusername/codebutler/issues
- Discussions: https://github.com/yourusername/codebutler/discussions

---

**Built with ❤️ for developers who want Claude Code everywhere**

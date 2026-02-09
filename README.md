# CodeButler

**Your code's personal butler** - Multi-repository development assistant via WhatsApp

Built with Go • Powered by Claude Code SDK

---

## What is CodeButler?

CodeButler is a WhatsApp-based development assistant built in Go that helps you manage multiple code repositories through natural conversation. It acts as your personal butler, handling development tasks across all your projects from your phone.

### Key Features

- 🤖 **WhatsApp Integration** - Control your development workflow from your phone
- 📁 **Multi-Repo Native** - Work across multiple repositories simultaneously
- 🎙️ **Voice Messages** - Transcribe voice to text with Whisper API
- 🔒 **Secure by Design** - Explicit access control for personal chat and approved groups
- ⚡ **Go Performance** - Fast startup, low memory, single binary deployment
- 🎯 **Claude Code SDK** - Leverages your existing Claude Pro/Max subscription (no API costs)

### Core Concept

```
WhatsApp ←→ CodeButler (Go) ←→ Claude Code SDK (per-repo)
                ↓
         Sources/ directory
         ├── api-service/
         ├── frontend-app/
         └── mobile-client/
```

## Quick Start

### Prerequisites

```bash
# Install Go 1.21+
brew install go

# Install Node.js (for Claude Code SDK)
brew install node

# Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# Get OpenAI API key (for voice transcription)
# Visit: https://platform.openai.com/api-keys
```

### Installation

```bash
# Clone repository
git clone git@github.com:leandrotocalini/CodeButler.git
cd CodeButler

# Build
go build -o butler main.go

# First run (interactive setup)
./butler
```

### First Run Setup

1. **Scan QR** with WhatsApp on your phone
2. **Enter OpenAI API Key** when prompted
3. **CodeButler Developer group** is automatically created or found
4. **Start coding** from WhatsApp!

### Usage

```
# In WhatsApp (personal chat or CodeButler Developer group):

@butler repos
→ Lists all repositories in Sources/

@butler status
→ Shows system status and active sessions

in api-service: add logging to user controller
→ Executes in specific repository

[voice message]: "explain the auth flow in frontend"
→ Transcribes and processes your voice message
```

## Documentation

- [Product Specification](PRODUCT.md) - What CodeButler does and why
- [Technical Documentation](CLAUDE.md) - Architecture and implementation details
- [CodeButler Developer Group](CODEBUTLER_DEVELOPER_GROUP.md) - Your development control center
- [Implementation Roadmap](IMPLEMENTATION_ROADMAP.md) - Development plan and tasks

## Project Structure

```
CodeButler/
├── main.go                    # Entry point
├── go.mod                     # Dependencies
│
├── internal/
│   ├── whatsapp/              # WhatsApp client & handlers
│   ├── access/                # Access control logic
│   ├── audio/                 # Whisper API integration
│   ├── repo/                  # Repository management
│   ├── claude/                # Claude Code SDK executor
│   └── config/                # Configuration system
│
├── Sources/                   # Your repositories go here
│   ├── repo-a/
│   │   └── CLAUDE.md          # Repo-specific context
│   ├── repo-b/
│   │   └── CLAUDE.md
│   └── repo-c/
│       └── CLAUDE.md
│
└── docs/                      # Additional documentation
```

## Configuration

After first run, configuration is stored in `config.json` (gitignored):

```json
{
  "whatsapp": {
    "personalNumber": "1234567890@s.whatsapp.net",
    "allowedGroups": [
      {
        "jid": "120363123456789012@g.us",
        "name": "CodeButler Developer",
        "enabled": true,
        "isDevControl": true
      }
    ]
  },
  "openai": {
    "apiKey": "sk-..."
  },
  "claudeCode": {
    "oauthToken": "from-env-CLAUDE_CODE_OAUTH_TOKEN"
  }
}
```

## Why CodeButler?

### The Problem

Modern developers often work on:
- Multiple microservices
- Frontend + Backend repositories
- Shared libraries
- Different client projects

Switching contexts between repos is tedious and breaks flow.

### The Solution

CodeButler lets you:
- Ask questions across all repositories
- Make changes from your phone
- Use voice messages while away from computer
- Run bulk operations (tests, status checks) across all repos
- Keep development context in one place

### Why Go?

- **Single binary** - Deploy one file, no dependencies
- **Fast startup** - ~10ms vs Node.js ~200ms
- **Low memory** - ~20MB idle vs Node.js ~50MB
- **Great concurrency** - Goroutines for parallel operations
- **Cross-platform** - Easy to build for macOS, Linux, Windows

### Why Claude Code SDK?

- **No API costs** - Uses your Claude Pro/Max subscription
- **Better context** - Full understanding of code structure
- **Rich tools** - File operations, bash, web access
- **Session continuity** - Persistent conversation history

## Commands

### Basic Commands (All Chats)

```bash
@butler repos          # List all repositories
@butler help           # Show help message
in <repo>: <message>   # Target specific repository
```

### Dev Control Commands (CodeButler Developer Group Only)

```bash
@butler status                         # System status
@butler run tests in all repos         # Bulk operations
@butler compare <file> between repos   # Cross-repo analysis
@butler list groups                    # Group management
@butler allow group <name>             # Add group to allowed list
```

## Development

### Building

```bash
# Development build
go build -o butler main.go

# Production build (optimized)
go build -ldflags="-s -w" -o butler main.go

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o butler-linux main.go
```

### Testing

```bash
# Run unit tests
go test ./internal/...

# Run integration tests
go test -tags=integration ./...

# Run with debug logging
./butler --debug
```

## Security

- **Access Control** - Only personal number and explicitly allowed groups can use CodeButler
- **Config Protection** - `config.json` has 0600 permissions (owner read/write only)
- **WhatsApp Encryption** - All messages encrypted by WhatsApp end-to-end
- **Repository Isolation** - Each repo has independent context via CLAUDE.md
- **Credential Management** - Sensitive tokens stored securely, never committed to git

## FAQ

### Does this violate WhatsApp ToS?

CodeButler uses the WhatsApp Web protocol (like web.whatsapp.com). Personal use is typically safe, but business/commercial use may violate ToS. Use at your own risk.

### How much does it cost?

- CodeButler itself: Free (open source)
- Claude Code SDK: Included with Claude Pro/Max subscription ($20-$40/month)
- OpenAI Whisper API: ~$0.006 per minute of audio transcription
- WhatsApp: Free

### Can I use it with Claude API instead of SDK?

Yes, but you'll pay per-token. The SDK is included with your Claude subscription and has no additional costs.

### What happens if my phone is offline?

CodeButler needs WhatsApp Web connection. If your phone loses internet, messages won't be received until it reconnects.

### Can multiple people use the same CodeButler instance?

Yes! Add their WhatsApp groups to the allowed list. However, each group will have the same access to your repositories.

## Roadmap

- [x] Project planning and architecture
- [ ] Phase 1: WhatsApp integration
- [ ] Phase 2: Configuration system
- [ ] Phase 3: Repository management
- [ ] Phase 4: Claude Code executor
- [ ] Phase 5: Audio transcription
- [ ] Phase 6: Dev control commands
- [ ] Phase 7: Bulk operations
- [ ] Phase 8: Testing and documentation
- [ ] v1.0.0 Release

See [IMPLEMENTATION_ROADMAP.md](IMPLEMENTATION_ROADMAP.md) for detailed tasks.

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - See [LICENSE](LICENSE) file

## Support

- GitHub Issues: [github.com/leandrotocalini/CodeButler/issues](https://github.com/leandrotocalini/CodeButler/issues)
- Discussions: [github.com/leandrotocalini/CodeButler/discussions](https://github.com/leandrotocalini/CodeButler/discussions)

---

**Built with ❤️ for developers who want their personal code butler**

# CodeButler

WhatsApp-to-Claude bridge. Run it in any repo and talk to Claude Code from your phone.

## How it works

```
You (WhatsApp) → CodeButler daemon → claude -p → response back to WhatsApp
```

Each repo gets its own WhatsApp group, its own config, its own Claude session.
No cloud, no server — runs locally on your machine.

## Prerequisites

- **Go 1.24+**
- **Claude Code** — `npm install -g @anthropic-ai/claude-code`
- **OpenAI API key** (optional) — for voice message transcription and image generation

### Installing Go

Download and install from [go.dev/dl](https://go.dev/dl/), then add Go's bin directory to your PATH:

```bash
# macOS / Linux — add to ~/.zshrc or ~/.bashrc
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$(go env GOPATH)/bin
```

Reload your shell (`source ~/.zshrc`) and verify with `go version`.

## Install

```bash
go install github.com/leandrotocalini/CodeButler/cmd/codebutler@latest
```

This installs the `codebutler` binary to `$(go env GOPATH)/bin/`.

Or build from source:

```bash
git clone https://github.com/leandrotocalini/CodeButler.git
cd CodeButler
go build -o codebutler ./cmd/codebutler/
```

## Setup

```bash
cd ~/Source/my-project
codebutler
```

First run walks you through setup:

1. **Scan QR code** with WhatsApp (Settings → Linked Devices → Link a Device)
2. **Pick a group** from your existing WhatsApp groups (or create one)
3. **Set bot prefix** (default `[BOT]`) — all bot replies start with this
4. **Enter OpenAI key** (optional) — enables voice transcription and `/create-image`

That's it. The daemon starts listening.

## Usage

Send a message in the WhatsApp group and Claude responds with full repo context:

```
You:  what does the auth middleware do?
[BOT] The auth middleware in middleware/auth.go validates JWT tokens...
```

### Voice messages

Send a voice note — CodeButler transcribes it with Whisper and sends it to Claude.

### Commands

| Command | Description |
|---------|-------------|
| `/help` | List available commands |
| `/create-image <prompt>` | Generate an image with AI |
| `/create-image <prompt> <url>` | Edit an image from a URL |
| Photo + caption `/create-image <prompt>` | Edit an attached image |

All other `/commands` (`/compact`, `/new`, `/think`, etc.) are passed to Claude as skills.

### Multi-repo

Run CodeButler in multiple repos simultaneously. Each gets its own group:

```
~/Source/blog/  → codebutler → group "Blog Dev"  → Claude with blog context
~/Source/api/   → codebutler → group "API Team"  → Claude with API context
```

Visible in WhatsApp → Linked Devices as `CodeButler:<repo-name>`.

## Message queueing

Messages are processed one at a time. The active conversation has absolute priority.

- **Cold mode** (idle): first message triggers a 3s accumulation window to batch rapid messages
- **Active conversation**: follow-ups are processed immediately with `--resume`, queued messages wait
- **60s of silence** ends the conversation, then queued messages get processed

## Web dashboard

Each daemon serves a live dashboard at `http://localhost:3000` (auto-increments if port is busy).
Shows WhatsApp status, Claude status, and real-time logs via SSE.

## Storage

Everything lives in `<repo>/.codebutler/` (auto-added to `.gitignore`):

```
.codebutler/
├── config.json          # Group, bot prefix, Claude settings
├── store.db             # Messages + Claude session IDs
├── images/              # Saved generated images
└── whatsapp-session/    # WhatsApp pairing data
```

No global config. No shared state between repos.

## Reconfigure

```bash
codebutler --setup    # re-scan QR, pick different group, etc.
```

## License

MIT

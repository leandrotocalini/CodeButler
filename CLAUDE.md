# CodeButler - WhatsApp Bridge for Claude Code

> **WhatsApp agent that invokes Claude Code directly**

## How It Works

```
WhatsApp message → Go Agent → exec("claude -p task") → output → WhatsApp
```

1. User sends a message in the configured WhatsApp group
2. Go agent receives it via whatsmeow
3. Agent spawns `claude -p "the message" --output-format text`
4. Claude Code runs with full codebase access (reads, edits, runs commands)
5. Output is sent back to the WhatsApp group

Tasks are serialized — one Claude instance runs at a time.

## Setup

### First time:

```bash
./butler.sh
```

Opens web UI at `http://localhost:3000`:
1. Scan QR code with WhatsApp
2. Configure group name, bot prefix, sources directory
3. Agent starts automatically

### Already configured:

```bash
./butler.sh
```

Loads `config.json` and starts the agent. Messages flow automatically.

## Configuration

`config.json`:

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "groupJID": "120363...@g.us",
    "groupName": "CodeButler Developer",
    "botPrefix": "[BOT]"
  },
  "openai": {
    "apiKey": "sk-..."
  },
  "sources": {
    "rootPath": "./Sources"
  },
  "claude": {
    "command": "claude",
    "workDir": "/path/to/project",
    "maxTurns": 10
  }
}
```

### Claude config options

| Field | Default | Description |
|-------|---------|-------------|
| `command` | `"claude"` | Path to claude CLI binary |
| `workDir` | `sources.rootPath` | Directory where Claude runs tasks |
| `maxTurns` | `10` | Max agentic turns per task |

## MCP Mode (optional)

The agent can also run as an MCP server for when Claude Code is the initiator:

```bash
./codebutler --mcp
```

This exposes tools: `send_message`, `ask_question`, `get_pending`, `get_status`.

Configured via `.mcp.json` (auto-created during setup).

## Project Structure

```
CodeButler/
├── CLAUDE.md                    # This file
├── butler.sh                    # Build & run script
├── .mcp.json                    # MCP server config (auto-created)
│
├── ButlerAgent/                 # Go source
│   ├── cmd/codebutler/          # Unified binary
│   │   ├── main.go              # Agent + Web UI + MCP server
│   │   └── templates/
│   │       ├── setup.html       # Setup wizard
│   │       └── dashboard.html   # Dashboard
│   └── internal/
│       ├── whatsapp/            # WhatsApp client (whatsmeow)
│       ├── mcp/                 # MCP server implementation
│       ├── config/              # Config types and persistence
│       ├── access/              # Group-based access control
│       ├── audio/               # Voice transcription (OpenAI Whisper)
│       └── protocol/            # Legacy JSON file protocol
│
├── config.json                  # Runtime config (gitignored)
├── whatsapp-session/            # WhatsApp session (gitignored)
└── Sources/                     # User's repos (gitignored)
```

## Build

```bash
cd ButlerAgent
go build -o ../codebutler ./cmd/codebutler/
```

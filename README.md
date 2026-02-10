# CodeButler

**WhatsApp bridge for Claude Code** - MCP Server that gives Claude native WhatsApp tools.

```
You (WhatsApp) ←→ CodeButler (MCP) ←→ Claude Code
```

## Quick Start

```bash
git clone https://github.com/leandrotocalini/CodeButler.git
cd CodeButler
claude
```

That's it. Claude handles the rest.

## How It Works

1. **You run `claude`** in the CodeButler directory
2. **Claude runs `./butler.sh`** to start the web setup
3. **You scan QR code** and configure in browser (http://localhost:3000)
4. **Setup creates `.claude/mcp.json`** automatically
5. **You restart Claude** - now it has WhatsApp tools!

### After Setup

Claude has these native tools:

| Tool | What it does |
|------|-------------|
| `send_message` | Send message to WhatsApp |
| `ask_question` | Ask user with options (1/2/3) |
| `get_pending` | Get unread WhatsApp messages |
| `get_status` | Check connection status |

## Example

```
You (WhatsApp): "add authentication to the API"

Claude:
1. Sees message via get_pending()
2. Works on the code
3. Needs input → ask_question("Which library?", ["passport", "jose"])
4. You reply "2" in WhatsApp
5. Continues with jose
6. send_message("✅ Authentication added!")

You (WhatsApp): "✅ Authentication added!"
```

## Project Structure

```
CodeButler/
├── butler.sh              # Build & run (Claude calls this)
├── codebutler             # Binary (built by butler.sh)
├── CLAUDE.md              # Instructions for Claude
├── .claude/
│   └── mcp.json           # MCP config (auto-created)
├── config.json            # WhatsApp config (gitignored)
├── whatsapp-session/      # Session data (gitignored)
└── ButlerAgent/           # Go source code
    ├── cmd/codebutler/    # Main binary
    └── internal/
        ├── mcp/           # MCP server
        ├── whatsapp/      # WhatsApp client
        ├── config/        # Configuration
        └── ...
```

## Requirements

- Go 1.21+
- Claude Code CLI
- WhatsApp account

## Configuration

After setup, `config.json` contains:

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "groupJID": "120363...@g.us",
    "groupName": "CodeButler Developer",
    "botPrefix": "[BOT]"
  },
  "openai": {
    "apiKey": ""
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

## Voice Messages

Enable voice transcription by adding your OpenAI API key during setup. Voice messages are transcribed with Whisper and processed as text.

## Security

- Only messages from configured WhatsApp group are processed
- Bot messages are prefixed to avoid loops
- Config file is gitignored (contains no secrets by default)
- WhatsApp session uses end-to-end encryption

## Development

```bash
cd ButlerAgent
go build -o ../codebutler ./cmd/codebutler/

# Run as web UI
../codebutler

# Run as MCP server
../codebutler --mcp
```

## License

MIT

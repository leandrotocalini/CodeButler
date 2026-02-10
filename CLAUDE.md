# CodeButler - WhatsApp Bridge for Claude Code

> **MCP Server that gives Claude native WhatsApp tools**

## 🤖 What Claude Does on Project Open

### If NOT configured (no config.json):

1. Run the setup wizard:
```bash
./butler.sh
```

2. Tell the user:
```
🤖 Opening CodeButler setup at http://localhost:3000

Please:
1. Scan the QR code with WhatsApp
2. Fill in the configuration
3. Click "Complete Setup"

I'll wait here...
```

3. After setup completes, check if `.mcp.json` was created

4. Tell the user:
```
✅ CodeButler configured!

To activate WhatsApp tools, please restart Claude:
  Press Ctrl+C, then run: claude

After restart, I'll have native WhatsApp tools!
```

### If configured (config.json exists):

Check if MCP tools are available. If yes, you have these tools:

- `mcp__codebutler__send_message` - Send message to WhatsApp
- `mcp__codebutler__ask_question` - Ask user a question with options
- `mcp__codebutler__get_pending` - Get pending WhatsApp messages
- `mcp__codebutler__get_status` - Check connection status

Use them naturally when processing WhatsApp tasks!

## 📡 MCP Tools Reference

### send_message

Send a message to WhatsApp.

```json
{
  "chat_jid": "120363405395407771@g.us",
  "message": "✅ Task completed!"
}
```

### ask_question

Ask user a question with options. Returns their choice.

```json
{
  "chat_jid": "120363405395407771@g.us",
  "question": "Which database?",
  "options": ["PostgreSQL", "MySQL", "MongoDB"],
  "timeout": 30
}
```

Returns:
```json
{
  "selected": 1,
  "text": "PostgreSQL"
}
```

### get_pending

Get pending messages from WhatsApp (not yet processed).

Returns:
```json
{
  "messages": [
    {
      "id": "msg_123",
      "from": "5491234567890@s.whatsapp.net",
      "chat": "120363405395407771@g.us",
      "content": "add authentication to API",
      "timestamp": "2025-02-09T20:00:00Z"
    }
  ]
}
```

### get_status

Check WhatsApp connection status.

Returns:
```json
{
  "connected": true,
  "user": "Leandro",
  "group": "CodeButler Developer"
}
```

## 🔧 How It Works

1. **Setup phase**: `./butler.sh` runs web UI for QR scanning
2. **After setup**: Creates `.mcp.json` pointing to `./codebutler --mcp`
3. **On Claude restart**: Claude connects to MCP server
4. **Runtime**: Claude uses tools directly, no file polling

## 📁 Project Structure

```
CodeButler/
├── CLAUDE.md                    # This file
├── butler.sh                    # Build & run setup
├── .mcp.json                    # MCP server config (auto-created)
│
├── ButlerAgent/                 # Go source
│   └── cmd/codebutler/          # Unified binary
│       ├── main.go              # Web UI + MCP server
│       └── templates/
│
├── config.json                  # WhatsApp config (gitignored)
└── whatsapp-session/            # Session data (gitignored)
```

## 🎯 Example Workflow

```
User sends WhatsApp: "add JWT authentication"

Claude (with MCP tools):

1. Calls get_pending() → blocks until a message arrives (up to 30s)
2. Message arrives → processes the task (reads files, writes code)
3. Needs clarification → calls ask_question("Which library?", ["jose", "jsonwebtoken"])
4. User responds "1" in WhatsApp
5. Continues with jose
6. Calls send_message("✅ JWT authentication added!")
7. Calls get_pending() again → waits for next message

All native. No file polling. Direct communication.
```

### Message Loop

After completing each task, **always call `get_pending()`** to wait for the next WhatsApp message. This creates an event-driven loop — the call blocks until a message arrives or times out after 30 seconds. If it times out, call `get_pending()` again to keep listening.

## 🚀 Setup Commands

### First time (run by Claude automatically):

```bash
# Build and run web setup
./butler.sh

# This creates:
# - config.json (WhatsApp config)
# - .mcp.json (MCP server config)
```

### Manual rebuild (if needed):

```bash
cd ButlerAgent
go build -o ../codebutler ./cmd/codebutler/
```

### Test MCP server:

```bash
./codebutler --mcp
# Runs as MCP server (stdio transport)
```

## ⚙️ MCP Configuration

After setup, `.mcp.json` contains:

```json
{
  "mcpServers": {
    "codebutler": {
      "command": "./codebutler",
      "args": ["--mcp"]
    }
  }
}
```

Claude Code automatically reads this and connects to the MCP server.

---

**Native WhatsApp tools for Claude. No hacks. Pure MCP.** 🎯

# MCP Package

MCP (Model Context Protocol) server that gives Claude native WhatsApp tools.

## Usage

```go
server := mcp.NewServer(cfg)
server.Run()  // Runs JSON-RPC 2.0 over stdio
```

## Tools

| Tool | Description |
|------|-------------|
| `send_message` | Send message to WhatsApp |
| `ask_question` | Ask user with options, wait for response |
| `get_pending` | Get unread WhatsApp messages |
| `get_status` | Check connection status |

## Protocol

JSON-RPC 2.0 over stdio. Claude Code connects via `.claude/mcp.json`:

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

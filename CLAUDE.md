# CodeButler

WhatsApp-to-Claude bridge. A Go daemon that monitors a WhatsApp group and
spawns `claude -p` to process messages, giving Claude full repo context.

## Quick Start

```bash
go build -o codebutler ./cmd/codebutler/
./codebutler          # first run: setup wizard (QR + group) then daemon
./codebutler          # subsequent: daemon starts directly
./codebutler --setup  # force reconfigure
```

## Architecture

```
WhatsApp <-> whatsmeow <-> Go daemon <-> spawns claude -p <-> repo context
                               |
                           SQLite DB
                      (messages + sessions)
```

Each repo is fully self-contained: its own WhatsApp pairing, group, config,
and database. Visible in WhatsApp > Linked Devices as `CodeButler:<repo>`.

## Project Structure

```
cmd/codebutler/main.go          # Setup wizard (QR + group selection) + daemon entrypoint
internal/
  agent/agent.go                 # Claude CLI wrapper: spawn, parse JSON result, resume sessions
  config/                        # Per-repo config: load/save .codebutler/config.json, defaults
  daemon/
    daemon.go                    # Core poll loop, conversation state machine, message processing
    logger.go                    # Ring buffer logger (web dashboard) + ANSI TUI output (stderr)
    web.go                       # Web dashboard: /api/status, /api/logs, SSE stream, HTML UI
    imagecmd.go                  # /create-image command: OpenAI image gen/edit, confirmation flow
  imagegen/generate.go           # OpenAI gpt-image-1 API: generate and edit images
  store/
    store.go                     # SQLite messages table: insert, get pending, ack
    sessions.go                  # SQLite sessions table: per-chat Claude session tracking
  transcribe/whisper.go          # OpenAI Whisper API for voice message transcription
  whatsapp/
    client.go                    # whatsmeow wrapper: connect, QR, presence, connection state
    handler.go                   # Message parsing (text/voice/image), send text/image
    groups.go                    # Group CRUD: list, create, get info, manage participants
    auth.go                      # QR code display (terminal + image viewer fallback)
```

## Storage

Everything lives in `<repo>/.codebutler/` (gitignored):

```
.codebutler/
  config.json                    # Group JID, bot prefix, Claude settings, OpenAI key
  store.db                       # Messages + Claude session IDs (SQLite)
  images/                        # Generated images (optional)
  whatsapp-session/session.db    # WhatsApp pairing session
```

## Config (`config.json`)

```json
{
  "whatsapp": { "groupJID": "...@g.us", "groupName": "...", "botPrefix": "[BOT]" },
  "claude":   { "maxTurns": 10, "timeout": 30, "permissionMode": "bypassPermissions" },
  "openai":   { "apiKey": "sk-..." }
}
```

- `botPrefix`: prepended to outgoing messages so the bot can filter its own replies
- `timeout`: minutes before Claude process is killed
- `openai.apiKey`: optional, enables voice transcription and image generation

## Message Flow

1. WhatsApp message arrives via whatsmeow event
2. Daemon filters: group match, bot prefix, special commands (`/help`, `/create-image`)
3. Voice messages get transcribed via Whisper (if OpenAI key configured)
4. Message persisted to SQLite + poll loop notified
5. Poll loop applies conversation state machine (see below)
6. `agent.Run()` spawns `claude -p <prompt> --output-format json [--resume <id>]`
7. Response sent back to WhatsApp, session ID saved for future `--resume`

## Conversation State Machine

Messages are processed **one Claude at a time**. The active conversation
has absolute priority.

**Cold mode** (idle): first message triggers a 3s accumulation window.
Messages in that window get batched into one prompt.

**Active conversation** (after Claude responds): messages split by timestamp:
- **Follow-ups** (after Claude's response) -> immediate `--resume`
- **Queued** (arrived during processing) -> held until conversation ends

60s of silence ends the conversation, then queued messages get processed.

## Logger Design

Two output channels:
- **Ring buffer + SSE** (`Info/Warn/Error/Debug`): feeds web dashboard at `/api/logs/stream`
- **Stderr-only TUI** (`Header/UserMsg/BotStart/BotResult/BotText/Status`): clean
  conversational output with ANSI colors, does NOT go to the web dashboard

## Web Dashboard

Served on port 3000+ (auto-increments if busy). Endpoints:
- `GET /` — HTML dashboard with live updates
- `GET /api/status` — JSON: repo, group, WhatsApp state, Claude busy, conversation active
- `GET /api/logs` — JSON array of log entries
- `GET /api/logs/stream` — SSE stream of new log entries

## Special Commands

- `/help` — list commands
- `/create-image <prompt>` — generate image (OpenAI gpt-image-1)
- `/create-image <prompt> <url>` — edit image from URL
- Photo + caption `/create-image <prompt>` — edit attached image
- Other `/commands` (`/compact`, `/new`, `/think`) — passed through to Claude

## Development

```bash
go build -o codebutler ./cmd/codebutler/    # build
go test ./...                                # run tests
```

All code and comments are in English. PRs only to `main` (direct push disabled).

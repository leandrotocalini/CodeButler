# CodeButler

WhatsApp-to-Claude bridge. A Go daemon that listens for WhatsApp messages
and spawns `claude -p` to process them.

## Install

```bash
go install github.com/leandrotocalini/CodeButler/cmd/codebutler@latest
```

## Usage

```bash
cd ~/Source/my-project
codebutler          # first run: setup (QR + pick group) → daemon starts
codebutler          # after: daemon starts directly
codebutler --setup  # force reconfigure
```

## Architecture

```
WhatsApp ←→ whatsmeow ←→ Go daemon ←→ spawns claude -p ←→ repo context
                              ↓
                          SQLite DB
                     (messages + sessions)
```

Each repo is fully self-contained — its own WhatsApp pairing, its own group,
its own config. Visible in WhatsApp > Linked Devices as `CodeButler:<repo>`.

```
~/Source/blog/       → codebutler → group "Blog Dev"   → Claude with blog context
~/Source/api/        → codebutler → group "API Team"   → Claude with API context
```

## Storage

Everything lives in `<repo>/.codebutler/` (gitignored):

```
<repo>/.codebutler/
├── config.json              # Group, bot prefix, Claude settings
├── store.db                 # Messages + Claude session IDs
└── whatsapp-session/        # WhatsApp pairing (per-repo)
    └── session.db
```

No global config. No shared state between repos.

## Project Structure

```
cmd/codebutler/main.go       # Setup wizard + daemon entrypoint
internal/
├── agent/                    # Claude CLI wrapper (spawn, parse, resume)
├── config/                   # Per-repo config (.codebutler/config.json)
├── daemon/                   # Poll loop, conversation priority, web dashboard
├── imagegen/                 # OpenAI image generation (gpt-image-1)
├── store/                    # SQLite: messages + Claude sessions
├── transcribe/               # OpenAI Whisper voice transcription
└── whatsapp/                 # WhatsApp client (whatsmeow)
```

## Message Queueing

Messages are processed **one Claude at a time**. The active conversation
has **absolute priority** — queued messages never interrupt it.

**Cold mode** (idle): first message triggers a 3s accumulation window.
Messages in that window get batched into one prompt.

**Active conversation** (after Claude responds): messages split by timestamp:
- **Follow-ups** (after response) → immediate `--resume`
- **Queued** (during processing) → held until conversation ends

**No timer limits the conversation.** It stays active as long as the user
replies. 60s of silence ends it, then queued messages get processed.

## Web Dashboard

Each daemon serves a dashboard on port 3000+ (auto-increments if busy).
Live logs via SSE, WhatsApp/Claude/conversation status.

## Build

```bash
go build -o codebutler ./cmd/codebutler/
```

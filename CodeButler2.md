# CodeButler 2

CodeButler evolution plan: WhatsApp → Slack migration + new features.

**Status**: Planning (implementation not started)

---

## 1. Motivation

Migrate the messaging backend from WhatsApp (whatsmeow) to Slack, keeping
the same core logic: a daemon that monitors a channel and spawns `claude -p`.

---

## 2. Concept Mapping

| WhatsApp | Slack | Notes |
|----------|-------|-------|
| Group JID (`...@g.us`) | Channel ID (`C0123ABCDEF`) | Channel identifier |
| User JID (`...@s.whatsapp.net`) | User ID (`U0123ABCDEF`) | User identifier |
| QR code pairing | OAuth App + Bot Token | Authentication |
| whatsmeow events | Slack Socket Mode / Events API | Message reception |
| `SendMessage(jid, text)` | `chat.postMessage(channel, text)` | Send text |
| `SendImage(jid, png, caption)` | `files.upload` + message | Send images |
| Read receipts (`MarkRead`) | No direct equivalent | Can omit or use reactions |
| Typing indicator (`SendPresence`) | No native bot typing | Can omit |
| Voice messages (Whisper) | Audio files in Slack → Whisper | Same flow, different download |
| Bot prefix `[BOT]` | Bot messages have `bot_id` | Slack filters bots natively |
| Linked Devices (device name) | App name in workspace | Visible in Apps |
| `whatsapp-session/session.db` | Bot token (string) | No persistent session |
| Group creation | `conversations.create` | Private/public channel |

---

## 3. Current vs Proposed Architecture

### Current
```
WhatsApp <-> whatsmeow <-> Go daemon <-> spawns claude -p <-> repo context
                               |
                           SQLite DB
                      (messages + sessions)
```

### Proposed
```
Slack <-> slack-go SDK <-> Go daemon <-> spawns claude -p <-> repo context
                               |
                           SQLite DB
                      (messages + sessions)
```

---

## 4. Dependencies

### Remove
- `go.mau.fi/whatsmeow` (and all subdependencies: protobuf, signal protocol, etc.)
- `github.com/skip2/go-qrcode` (QR no longer needed)
- `github.com/mdp/qrterminal/v3` (QR terminal)

### Add
- `github.com/slack-go/slack` — Official Slack SDK for Go
  - Socket Mode (WebSocket, no public endpoint needed)
  - Events API
  - Web API (chat.postMessage, files.upload, etc.)

---

## 5. Slack App Setup (prerequisites)

Before the daemon works, the user needs to create a Slack App:

1. Go to https://api.slack.com/apps → Create New App
2. Configure Bot Token Scopes (OAuth & Permissions):
   - `channels:history` — read public channel messages
   - `channels:read` — list channels
   - `chat:write` — send messages
   - `files:read` — download attachments (audio, images)
   - `files:write` — upload files (generated images)
   - `groups:history` — read private channel messages
   - `groups:read` — list private channels
   - `reactions:write` — (optional) confirm read with reaction
   - `users:read` — resolve usernames
3. Enable Socket Mode (Settings → Socket Mode → Enable)
   - Generates an App-Level Token (`xapp-...`)
4. Enable Events (Event Subscriptions → Enable):
   - Subscribe to bot events: `message.channels`, `message.groups`
5. Install to Workspace → copy Bot Token (`xoxb-...`)

### Required tokens
- **Bot Token** (`xoxb-...`): API operations (send, read, etc.)
- **App Token** (`xapp-...`): Socket Mode connection (WebSocket)

---

## 6. Config Changes

### Current (`config.json`)
```json
{
  "whatsapp": { "groupJID": "...@g.us", "groupName": "...", "botPrefix": "[BOT]" },
  "claude":   { "maxTurns": 10, "timeout": 30, "permissionMode": "bypassPermissions" },
  "openai":   { "apiKey": "sk-..." }
}
```

### Proposed: Global + Per-Repo Config

Two config levels. Global holds shared keys, per-repo holds only channel-specific
settings. Per-repo can override global values.

**Global** (`~/.codebutler/config.json`) — configured once:
```json
{
  "slack": {
    "botToken": "xoxb-...",
    "appToken": "xapp-..."
  },
  "openai": { "apiKey": "sk-..." },
  "kimi":   { "apiKey": "..." }
}
```

**Per-repo** (`<repo>/.codebutler/config.json`) — one per repo:
```json
{
  "slack": {
    "channelID": "C0123ABCDEF",
    "channelName": "codebutler-myrepo"
  },
  "claude": { "maxTurns": 10, "timeout": 30, "permissionMode": "bypassPermissions" }
}
```

**Merge strategy**: per-repo overrides global (field by field).
If per-repo defines `slack.botToken`, that takes precedence over global.

**Changes vs current:**
- `whatsapp` → `slack`
- `groupJID` → `channelID`
- `groupName` → `channelName`
- `botPrefix` → **removed** (Slack identifies bots by `bot_id`)
- New: `botToken`, `appToken` (in global)
- `openai.apiKey` moves to global (shared across repos)
- New: `kimi.apiKey` in global

---

## 7. Storage Changes

### Directories

```
~/.codebutler/
  config.json                    # Global: Slack tokens, OpenAI key

<repo>/.codebutler/
  config.json                    # Per-repo: channelID, Claude settings
  store.db                       # Messages + Claude session IDs (SQLite) — UNCHANGED
  images/                        # Generated images — UNCHANGED
```

### SQLite `messages` table

```sql
-- Current
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    from_jid    TEXT NOT NULL,        -- → rename to from_id
    chat        TEXT NOT NULL,        -- → rename to channel_id
    content     TEXT NOT NULL,
    timestamp   TEXT NOT NULL,
    is_voice    INTEGER DEFAULT 0,
    acked       INTEGER DEFAULT 0,
    wa_msg_id   TEXT DEFAULT ''       -- → rename to platform_msg_id
);
```

**Minimal changes**: rename columns to be platform-agnostic, or keep them
internally and only change the code that populates them.

### SQLite `sessions` table

The key insight: **each Slack thread IS a Claude session**. The primary key
changes from `chat_jid` (one session per chat) to `thread_ts` (one session
per thread). This enables N concurrent conversations in the same channel.

```sql
-- Current
CREATE TABLE sessions (
    chat_jid   TEXT PRIMARY KEY,      -- one session per chat
    session_id TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Proposed
CREATE TABLE sessions (
    thread_ts  TEXT PRIMARY KEY,      -- one session per Slack thread
    channel_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

**Behavior**:
- New top-level message → create thread → new Claude session → store `thread_ts → session_id`
- Reply in thread → lookup `thread_ts` → `--resume session_id`
- Multiple threads can be active simultaneously (no global busy lock)

---

## 8. Files to Modify/Create/Delete

### Delete
| File | Reason |
|------|--------|
| `internal/whatsapp/client.go` | Replaced by Slack client |
| `internal/whatsapp/handler.go` | Replaced by Slack event handler |
| `internal/whatsapp/groups.go` | Replaced by channel operations |
| `internal/whatsapp/auth.go` | QR not applicable, Slack uses tokens |

### Create
| File | Purpose |
|------|---------|
| `internal/slack/client.go` | Socket Mode connection, state, disconnect |
| `internal/slack/handler.go` | Event parsing, send messages/images |
| `internal/slack/channels.go` | List/create channels, get info |
| `internal/slack/snippets.go` | Code block extraction, smart formatting, file upload |
| `internal/history/history.go` | PR detection, thread fetch, summary generation, commit |
| `internal/llm/client.go` | OpenAI-compatible client for cheap models (Kimi, GPT-4o-mini, DeepSeek) |
| `internal/preflight/preflight.go` | Pre-Claude enrichment: grep repo, read files, build focused prompt |
| `internal/router/router.go` | Message classifier: question vs code_task vs clarify |

### Modify
| File | Changes |
|------|---------|
| `cmd/codebutler/main.go` | Setup wizard: prompt tokens instead of QR, select channel instead of group |
| `internal/config/types.go` | `WhatsAppConfig` → `SlackConfig`, separate `GlobalConfig` and `RepoConfig` |
| `internal/config/load.go` | Load global (`~/.codebutler/`) + per-repo, merge, save both |
| `internal/daemon/daemon.go` | Replace `whatsapp.Client` with `slack.Client`, delete state machine (~300 lines), add thread dispatch (~50 lines) |
| `internal/daemon/imagecmd.go` | `SendImage` → Slack `files.upload` |
| `internal/daemon/web.go` | Change "WhatsApp state" to "Slack state" in status API |
| `internal/store/store.go` | Rename columns: `from_id`, `channel_id`, `platform_msg_id` |
| `go.mod` / `go.sum` | New dependencies |

### Unchanged
| File | Reason |
|------|--------|
| `internal/agent/agent.go` | Claude spawn is messaging-independent |
| `internal/imagegen/generate.go` | OpenAI API is independent |
| `internal/transcribe/whisper.go` | Whisper API is independent |
| `internal/store/sessions.go` | PK changes: `chat_jid` → `thread_ts`, add `channel_id` column |
| `internal/daemon/logger.go` | Logger is independent |

---

## 9. New `internal/slack/` — Interface Design

### `client.go`

```go
package slack

type ConnectionState int

const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected
)

type Client struct {
    api       *slack.Client        // Web API (xoxb token)
    socket    *socketmode.Client   // Socket Mode (xapp token)
    state     ConnectionState
    botUserID string               // Bot's own user ID (to filter its own messages)
}

// Connect starts Socket Mode and waits for connection
func Connect(botToken, appToken string) (*Client, error)

// Disconnect closes the connection
func (c *Client) Disconnect()

// GetState returns the current connection state
func (c *Client) GetState() ConnectionState

// IsConnected returns true if connected
func (c *Client) IsConnected() bool

// GetBotUserID returns the bot's user ID
func (c *Client) GetBotUserID() string
```

### `handler.go`

```go
// Message abstracts a message (equivalent to whatsapp.Message)
type Message struct {
    ID        string
    From      string    // User ID
    FromName  string    // Display name (resolved via users.info)
    Channel   string    // Channel ID
    Content   string
    Timestamp string    // Slack ts (e.g., "1234567890.123456")
    IsFromMe  bool      // From the bot
    IsVoice   bool      // Audio file attachment
    IsImage   bool      // Image file attachment
    FileURL   string    // File URL (if any)
    ThreadTS  string    // Thread timestamp (for replying in thread)
}

type MessageHandler func(Message)

// OnMessage registers a callback for new messages
func (c *Client) OnMessage(handler MessageHandler)

// SendMessage sends text to a channel
func (c *Client) SendMessage(channelID, text string) error

// SendImage uploads and sends an image to a channel
func (c *Client) SendImage(channelID string, pngData []byte, caption string) error

// DownloadFile downloads a file from Slack
func (c *Client) DownloadFile(fileURL string) ([]byte, error)
```

### `channels.go`

```go
type Channel struct {
    ID   string
    Name string
}

// GetChannels lists channels where the bot is present
func (c *Client) GetChannels() ([]Channel, error)

// CreateChannel creates a new channel
func (c *Client) CreateChannel(name string) (string, error)

// GetChannelInfo gets info about a channel
func (c *Client) GetChannelInfo(channelID string) (*Channel, error)
```

---

## 10. Setup Wizard — New Flow

### Current (WhatsApp)
```
1. Show QR code
2. User scans with phone
3. List groups → select or create
4. Set bot prefix
5. (Optional) OpenAI API key
6. Save config
```

### Proposed (Slack) — with global config

**First time (no `~/.codebutler/config.json`):**
```
1. Prompt: "Bot Token (xoxb-...):"
2. Prompt: "App Token (xapp-...):"
3. Validate tokens (api.AuthTest)
4. (Optional) Prompt: "OpenAI API key:"
5. (Optional) Prompt: "Kimi API key:"
6. Save → ~/.codebutler/config.json (global)
7. Connect Socket Mode
8. List channels → select or create
9. Save → <repo>/.codebutler/config.json (per-repo)
```

**Subsequent repos (global already exists):**
```
1. Load ~/.codebutler/config.json → tokens already configured
2. Connect Socket Mode
3. List channels → select or create
4. Save → <repo>/.codebutler/config.json (per-repo)
```

**Key difference**: tokens and API keys are requested once and stored
in `~/.codebutler/`. Each repo only configures its channel.

---

## 11. Message Flow — Event-Driven Threads

The conversation state machine (`AccumulationWindow`, `ReplyWindow`,
`convActive`, `pollLoop`) is **eliminated entirely**. Slack threads
provide natural conversation boundaries.

### Architecture Change

```
BEFORE (WhatsApp):
  1 global poll loop → 1 conversation at a time → state machine
  AccumulationWindow (3s) → ReplyWindow (60s) → cold/hot modes

AFTER (Slack):
  Event-driven → 1 goroutine per thread → N concurrent conversations
  No accumulation, no reply window, no state machine
```

### Reception
```
Slack WebSocket (Socket Mode)
    ↓ socketmode.EventTypeEventsAPI
    ↓ EventTypeMessageChannels
Parse: user, channel, text, thread_ts, files
    ↓
Filter: channel match, not from bot
    ↓
Audio file? → Download → Whisper transcribe
    ↓
Determine thread context:
    ├─ thread_ts == "" → new top-level message → spawn goroutine
    └─ thread_ts != "" → reply in thread → spawn goroutine with --resume
    ↓
goroutine:
    1. Lock thread (prevent double-processing)
    2. Lookup session_id for thread_ts
    3. agent.Run(prompt, session_id)
    4. Reply in thread (slack.MsgOptionTS(thread_ts))
    5. Store session_id for thread_ts
    6. Unlock thread
```

### Concurrency Model

```go
type Daemon struct {
    // ...
    activeMu sync.Mutex
    active   map[string]bool  // thread_ts → currently processing
}
```

Each thread is processed independently. Multiple threads can run Claude
concurrently. The `active` map prevents double-processing if multiple
messages arrive in the same thread while Claude is still working —
those messages are queued per-thread and processed after the current
run completes.

### Sending
```
agent.Run() result
    ↓
Format response (code snippets, markdown)
    ↓
slack.Client.SendMessage(channelID, text, thread_ts)
    ↓ api.PostMessage(channelID,
        slack.MsgOptionText(text, false),
        slack.MsgOptionTS(threadTS))
Slack API
```

### Own message filtering
```
// Current WhatsApp: compare botPrefix in content
if strings.HasPrefix(msg.Content, cfg.WhatsApp.BotPrefix) { skip }

// New Slack: compare bot user ID
if msg.BotID != "" || msg.User == c.botUserID { skip }
```

**Advantage**: Slack identifies bots natively, no prefix needed.

### What Gets Deleted from daemon.go

```go
// ALL of this goes away:
const AccumulationWindow = 3 * time.Second   // deleted
const ReplyWindow = 60 * time.Second         // deleted
const ColdPollInterval = 2 * time.Second     // deleted

convMu       sync.Mutex       // deleted
convActive   bool             // deleted
convResponse time.Time        // deleted

func pollLoop()               // deleted
func handleNewMessages()      // deleted — replaced by event handler
func isConversationActive()   // deleted
func startConversation()      // deleted
func endConversation()        // deleted
func getConversationResponseTime() // deleted
```

**~200 lines of state machine code replaced by ~50 lines of thread dispatch.**

---

## 12. Features that Change

### Bot Prefix → Removed
- WhatsApp needed `[BOT]` to filter own messages
- Slack identifies bots by `bot_id` in the event
- Bot messages are sent without prefix (cleaner)

### Read Receipts → Reactions
- WhatsApp: `MarkRead()` shows blue ticks
- Slack: use reactions as visual feedback
  - 👀 (`eyes`) when processing starts
  - ✅ (`white_check_mark`) when Claude finishes responding

### Typing Indicator → Removed
- WhatsApp: `SendPresence(composing=true)` shows "typing..."
- Slack: bots cannot show typing indicator
- Can be omitted without functional impact

### Threads = Claude Sessions (core design change)

This is the **central architectural decision** of CodeButler2.

- **Each Slack thread IS a Claude session** (1:1 mapping)
- New message in channel → bot replies in a new thread → new `claude -p` session
- Reply in that thread → `claude -p --resume <session_id>` with full context
- Multiple threads can run concurrently (no global lock)
- Thread history is visible in Slack (natural conversation UI)
- No accumulation window, no reply window, no state machine
- Session ends naturally when the user stops replying to that thread

**Why this is better than WhatsApp groups:**

| WhatsApp (v1) | Slack Threads (v2) |
|---|---|
| 1 conversation at a time (global lock) | N concurrent threads |
| State machine: cold/hot modes, timers | Event-driven, no state machine |
| ~300 lines of state machine code | ~50 lines of thread dispatch |
| Messages queued during processing | Each thread independent |
| 60s silence = conversation end | Thread never "expires" |
| All messages in one flat chat | Each task in its own thread |
| Hard to reference past conversations | Threads are permanent, searchable |

### Voice Messages
- WhatsApp: inline voice, download with `DownloadAudio()`
- Slack: audio as file attachment, download with `files.info` + HTTP GET with auth
- Same Whisper pipeline after download

### Image Messages
- WhatsApp: inline image with `DownloadImage()`
- Slack: image as file attachment
- Send: `files.upload` instead of protobuf with media upload

---

## 13. Decisions Made

- [x] **Threads = Sessions**: each Slack thread IS a Claude session (1:1 mapping)
- [x] **No state machine**: event-driven thread dispatch replaces cold/hot modes
- [x] **Concurrent threads**: multiple threads can run Claude simultaneously
- [x] **Reactions**: yes, use 👀 when processing starts and ✅ when done
- [x] **SQLite column names**: rename to `from_id`, `channel_id`, `platform_msg_id`
- [x] **Sessions key**: `thread_ts` replaces `chat_jid` as primary key
- [x] **Multiple channels**: no, one channel per repo (like WhatsApp)
- [x] **Bot mention**: respond to all channel messages, no @mention required
- [x] **Message length**: split into multiple ~4000 char messages in thread
- [x] **Markdown**: convert Claude output (standard Markdown) to Slack mrkdwn before sending
- [x] **Code snippets**: short (<20 lines) inline, long (>=20 lines) as file upload
- [x] **Knowledge sharing**: CLAUDE.md committed to branches, shared via PR merge
- [x] **memory.md coexistence**: local memory.md (Kimi) + shared CLAUDE.md (git) both exist
- [x] **Thread history**: auto-generate `history/<threadId>.md` when PR is opened, committed to the PR branch
- [x] **Multi-model**: Claude executes code, cheap models (Kimi/GPT-4o-mini) orchestrate around it
- [x] **Smart routing**: Kimi classifies messages — questions answered directly, code tasks go to Claude
- [x] **Pre-flight enrichment**: Kimi scans repo + memory before Claude runs, builds focused prompt
- [x] **Workflow planning**: Kimi generates execution plans for complex tasks, user approves before Claude runs

---

## 14. Implementation Order

1. **Config**: `SlackConfig` + load/save
2. **Slack client**: Socket Mode connection, state
3. **Slack handler**: receive messages, send text
4. **Daemon integration**: replace whatsapp.Client with slack.Client
5. **Setup wizard**: token flow + channel selection
6. **Image support**: `files.upload` for `/create-image`
7. **Voice support**: audio file download → Whisper
8. **Cleanup**: delete `internal/whatsapp/`, update `go.mod`
9. **Testing**: manual end-to-end test
10. **Docs**: update CLAUDE.md

---

## 15. Risks

| Risk | Mitigation |
|------|------------|
| Slack rate limiting (1 msg/s) | Implement queue with backoff |
| Messages > 4000 chars | Split into multiple messages |
| Socket Mode requires app-level token | Document well in setup |
| Files API changed in 2024+ | Use updated SDK |
| Bot can't see messages without being invited to channel | Document in setup wizard |

---

## 16. Auto-Memory (Kimi)

The daemon automatically extracts learnings at the end of each conversation
and persists them in `memory.md`. Uses Kimi (cheap and fast) instead of Claude.

### File

```
<repo>/.codebutler/memory.md
```

Injected as context into the Claude prompt on each new conversation.

### Trigger

When a conversation ends (60s of silence), the daemon:

1. Reads current `memory.md`
2. Builds a summary of the conversation that just ended
3. Calls Kimi with both
4. Applies the operations Kimi returns

### Kimi Prompt

```
You manage a memory file. Given the current memory and a conversation
that just ended, respond with a JSON array of operations.

Each operation is one of:
- {"op": "none"}        — nothing new to remember
- {"op": "append", "line": "- ..."}  — add a new learning
- {"op": "replace", "old": "exact existing line", "new": "merged line"}
                        — merge new info into an existing entry

Rules:
- Use "replace" when new info can be combined with an existing line
  (e.g., "cats are carnivores" + learning "dogs are carnivores"
   → replace with "cats and dogs are carnivores")
- Use "append" only for genuinely new knowledge
- Keep each line concise (1 line max)
- Only record useful decisions, conventions, gotchas — not trivia
- Return [{"op": "none"}] if there is nothing worth remembering
- You can return as many operations as needed

Current memory:
---
{contents of memory.md}
---

Conversation:
---
{conversation messages}
---
```

### Expected Response

```json
[
  {"op": "replace", "old": "- Cats are carnivores", "new": "- Cats and dogs are carnivores"},
  {"op": "append", "line": "- Always deploy with --force in staging"}
]
```

Or if nothing new:

```json
[{"op": "none"}]
```

### Implementation

- **File**: `internal/memory/memory.go`
- **Functions**:
  - `Load(path) string` — read memory.md (or "" if doesn't exist)
  - `Apply(content string, ops []Op) string` — apply operations to content
  - `Save(path, content string)` — write memory.md
- **Kimi client**: `internal/kimi/client.go`
  - OpenAI-compatible API (chat completions)
  - Only used for auto-memory
  - Requires `kimi.apiKey` in global config
- **Daemon integration**: at the end of `endConversation()`, launch a
  goroutine that calls Kimi and updates memory.md (non-blocking)

### Config

```json
// ~/.codebutler/config.json (global)
{
  "kimi": { "apiKey": "..." }
}
```

If no Kimi API key is configured, auto-memory is silently disabled.

---

## 17. Logging — Plain Structured Logs

Replace the dual system (ring buffer + TUI with ANSI) with a single channel
of plain, structured logs with good information.

### Format

```
2026-02-14 15:04:05 INF  slack connected
2026-02-14 15:04:08 MSG  leandro: "fix the login bug"
2026-02-14 15:04:08 MSG  leandro: "and check the CSS too"
2026-02-14 15:04:11 CLD  processing 2 messages (new session)
2026-02-14 15:04:45 CLD  done · 34s · 3 turns · $0.12
2026-02-14 15:04:45 RSP  "Fixed the login bug and adjusted the CSS..."
2026-02-14 15:05:45 INF  conversation ended (60s silence)
2026-02-14 15:05:46 MEM  kimi: append "Login uses bcrypt, not md5"
2026-02-14 15:06:00 WRN  slack reconnecting...
2026-02-14 15:06:01 ERR  kimi API timeout after 10s
```

### Tags

| Tag | Meaning |
|-----|---------|
| `INF` | System info: connection, config, state |
| `WRN` | Warnings: reconnections, recoverable timeouts |
| `ERR` | Errors: API failures, recovered crashes |
| `DBG` | Debug: only if verbose mode is enabled |
| `MSG` | Incoming user message |
| `CLD` | Claude activity: start, done, resume |
| `RSP` | Response sent to channel |
| `MEM` | Auto-memory operations |

### What Gets Removed

- `Clear()` — no more screen clearing
- `Header()` — no more banners with separators
- `UserMsg()` — replaced by `MSG`
- `BotStart()` / `BotResult()` / `BotText()` — replaced by `CLD` and `RSP`
- `Status()` — replaced by `INF`
- ANSI escape codes — everything plain
- `go-isatty` dependency — no longer needed

### What Stays

- **Ring buffer + SSE** for the web dashboard (same mechanics, new format)
- **Subscribers** (`Subscribe()` / `Unsubscribe()`)

### Implementation

A single internal `log(tag, format, args...)` method that:
1. Formats: `{datetime} {TAG}  {message}`
2. Writes to stderr
3. Stores in ring buffer
4. Notifies subscribers

Public methods: `Inf()`, `Wrn()`, `Err()`, `Dbg()`, `Msg()`, `Cld()`, `Rsp()`, `Mem()`

---

## 18. Service Install — macOS + Linux

Run CodeButler as a system service. On macOS uses **LaunchAgent**,
on Linux uses **systemd user service**. Both run in the user's session,
which gives access to:

- Xcode toolchain (`xcodebuild test`, `swift test`, `xcrun`)
- User keychain
- PATH with developer tools
- Homebrew binaries
- User environment variables

A LaunchDaemon (macOS) or system-level systemd service would run as root
without a session and wouldn't have access to any of this.

### macOS: LaunchAgent Plist

```xml
<!-- ~/Library/LaunchAgents/com.codebutler.<repo>.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.codebutler.myrepo</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/codebutler</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/Users/leandro/projects/myrepo</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/leandro/.codebutler/logs/myrepo.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/leandro/.codebutler/logs/myrepo.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Applications/Xcode.app/Contents/Developer/usr/bin</string>
    </dict>
</dict>
</plist>
```

### Linux: systemd User Service

```ini
# ~/.config/systemd/user/codebutler-myrepo.service
[Unit]
Description=CodeButler - myrepo
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/codebutler
WorkingDirectory=/home/leandro/projects/myrepo
Restart=always
RestartSec=5
StandardOutput=append:/home/leandro/.codebutler/logs/myrepo.log
StandardError=append:/home/leandro/.codebutler/logs/myrepo.log

[Install]
WantedBy=default.target
```

```bash
# To make user services start without login:
sudo loginctl enable-linger leandro
```

`enable-linger` allows user services to start at boot without requiring
login. Without linger, they start at login (like LaunchAgent).

### CLI Commands

```bash
codebutler --install     # generate plist/unit + load/enable
codebutler --uninstall   # unload/disable + delete plist/unit
codebutler --status      # show if the service is running
codebutler --logs        # tail -f the log file
```

### `--install` does:

1. Detect current repo (`pwd`) and name (basename)
2. Detect binary path of `codebutler`
3. Detect OS (`runtime.GOOS`)
4. Create `~/.codebutler/logs/` if it doesn't exist
5. **macOS**: generate plist → `~/Library/LaunchAgents/` → `launchctl load`
6. **Linux**: generate unit → `~/.config/systemd/user/` → `systemctl --user enable --now`

### Multiple repos

Each repo is an independent service:

```
# macOS
~/Library/LaunchAgents/
  com.codebutler.myapp.plist
  com.codebutler.backend.plist
  com.codebutler.ios-app.plist

# Linux
~/.config/systemd/user/
  codebutler-myapp.service
  codebutler-backend.service
  codebutler-ios-app.service
```

Each with its own `WorkingDirectory`, log file, and Slack channel.

### Behavior

- macOS: `RunAtLoad` + `KeepAlive` → starts at login, restarts on crash
- Linux: `enable` + `Restart=always` → same behavior
- Linux with `enable-linger`: starts at boot without requiring login
- Logs go to `~/.codebutler/logs/<repo>.log` (plain format, section 17)
- Web dashboard remains available on its port (auto-increments if busy)

---

## 19. Claude Sandboxing — System Prompt

The system prompt that CodeButler passes to `claude -p` must start with
clear restrictions to jail the agent inside the repo.

### Mandatory prompt prefix

```
RESTRICTIONS — READ FIRST:
- You MUST NOT install software, packages, or dependencies (no brew, apt,
  npm install, pip install, go install, etc.)
- You MUST NOT leave the current working directory or access files outside
  this repository
- You MUST NOT modify system files, configs outside the repo, or
  environment variables
- You MUST NOT make network requests except through tools already available
  in the repo
- You MUST NOT run destructive commands (rm -rf, git push --force,
  DROP TABLE, etc.)
- If a task requires any of the above, explain what is needed and STOP

You are working in: {repo_path}
```

### Why

Since Claude runs with `permissionMode: bypassPermissions`, it has full
shell access. Without these prompt restrictions, Claude could:
- Install packages that break the system
- Navigate outside the repo and read/modify other files
- Run `git push --force` or delete branches
- Execute destructive commands

The prompt sandboxing is a software defense layer (not a real OS sandbox),
but in practice Claude respects these instructions consistently.

### Implementation

In `internal/agent/agent.go`, the prompt is assembled as:

```go
prompt := sandboxPrefix + "\n\n" + memoryContext + "\n\n" + userMessages
```

Where `sandboxPrefix` is a constant with the restrictions.

---

## 20. Code Snippets — Smart Formatting

Claude's responses often contain code blocks. Slack supports both inline
code blocks and file uploads with syntax highlighting. CodeButler2
automatically picks the best format.

### Strategy

| Code block size | Format | Why |
|---|---|---|
| < 20 lines | Inline ` ```lang ` | Readable in-thread, no extra clicks |
| >= 20 lines | `files.uploadV2` as snippet | Collapsible, syntax highlighted, downloadable |

### Response Processing Pipeline

```
Claude response (markdown)
    ↓
Parse: extract code blocks (```lang\n...\n```)
    ↓
For each code block:
    ├─ < 20 lines → keep inline as Slack ```lang block
    └─ >= 20 lines → extract, upload as snippet file
    ↓
Reassemble message:
    - Text portions → single message
    - Long code blocks → separate file uploads in same thread
```

### Slack File Upload for Snippets

```go
// Upload a code snippet to a Slack thread
func (c *Client) UploadSnippet(channelID, threadTS, code, lang, filename string) error {
    _, err := c.api.UploadFileV2(slack.UploadFileV2Parameters{
        Channel:        channelID,
        Content:        code,
        Filename:       filename,      // e.g., "fix.go", "query.sql"
        FileType:       lang,          // e.g., "go", "python", "sql"
        Title:          filename,
        ThreadTimestamp: threadTS,
    })
    return err
}
```

### Filename Inference

Generate meaningful filenames from context:
- If Claude mentions a file path → use that filename (e.g., `handler.go`)
- If only language is known → use `snippet.{ext}` (e.g., `snippet.py`)
- Multiple snippets in one response → number them (`snippet-1.go`, `snippet-2.go`)

### Implementation

- **File**: `internal/slack/snippets.go`
- **Functions**:
  - `FormatResponse(text string) (message string, snippets []Snippet)`
  - `ExtractCodeBlocks(markdown string) []CodeBlock`
- **Integration**: called in daemon before sending response to Slack

---

## 21. Knowledge Sharing via memory.md + PR Merge

Each thread works in isolation. Knowledge is shared across threads only
when a PR is merged — through git, not through any custom sync mechanism.

### The Flow

```
Thread A: "fix the login bug"
    → Claude works on branch fix/login
    → Claude learns: "auth uses bcrypt, sessions expire after 24h"
    → These learnings go into CLAUDE.md on the branch
    → PR created → reviewed → merged to main
    → CLAUDE.md changes now in main ✓

Thread B: "add password reset" (started after merge)
    → Claude reads CLAUDE.md from main (or its branch base)
    → Already knows: "auth uses bcrypt, sessions expire after 24h"
    → Builds on existing knowledge ✓

Thread C: "refactor the API" (started BEFORE merge)
    → Still working on its branch, doesn't see Thread A's learnings
    → Gets the knowledge on next rebase/merge from main
```

### Why This Is Elegant

1. **No custom sync** — git is the knowledge transport
2. **Isolation by default** — threads can't pollute each other's context
3. **Review gate** — learnings go through PR review before becoming shared
4. **Conflict resolution** — git merge handles conflicting CLAUDE.md edits
5. **Audit trail** — every knowledge addition is a commit with context

### How It Differs from memory.md (Section 16)

Section 16 describes auto-memory via Kimi that updates `.codebutler/memory.md`
(gitignored, local). This section describes knowledge in `CLAUDE.md` (committed,
shared).

| | `.codebutler/memory.md` (Sec 16) | `CLAUDE.md` (Sec 21) |
|---|---|---|
| **Scope** | Local to this daemon instance | Shared across all developers |
| **Written by** | Kimi (automatic) | Claude (during work) |
| **Gitignored** | Yes | No — committed |
| **Sharing** | Never shared | Shared via PR merge |
| **Content** | Operational learnings | Codebase knowledge, conventions |
| **Review** | No review | PR review gate |

Both can coexist: memory.md for local operational memory, CLAUDE.md for
shared codebase knowledge.

---

## 22. Why This Architecture Is Better

### Simplicity

The conversation state machine was the most complex part of CodeButler v1
(~300 lines, 4 states, 3 timers, subtle edge cases). Slack threads
eliminate it entirely. The new daemon is **event-driven with thread dispatch**:

```go
// v1: 300 lines of state machine
func pollLoop()
func handleNewMessages()   // cold mode vs hot mode
func isConversationActive()
func startConversation()
func endConversation()
func getConversationResponseTime()
// AccumulationWindow, ReplyWindow, ColdPollInterval
// convActive, convResponse, convMu

// v2: ~20 lines of dispatch
func onMessage(msg slack.Message) {
    threadTS := msg.ThreadTS
    if threadTS == "" {
        threadTS = msg.Timestamp  // new top-level → becomes the thread
    }
    go d.processThread(threadTS, msg)
}
```

### Concurrency

v1 processed one conversation at a time. While Claude was thinking,
all other messages were queued. If Claude took 2 minutes, users waited.

v2 runs one goroutine per thread. User A asks about a bug in thread 1,
user B asks about a feature in thread 2 — both get responses
simultaneously.

### Natural UX

WhatsApp groups are flat — all messages in one stream. You can't tell
where one conversation ends and another begins. The bot prefix `[BOT]`
is a hack to filter bot messages.

Slack threads are structured — each task lives in its own thread.
Bot messages are identified natively (no prefix needed). You can
collapse threads you don't care about. You can reference old threads.

### Persistence & Searchability

WhatsApp conversations are ephemeral from the bot's perspective
(stored in local SQLite, not easily searchable). Slack threads are
permanent, indexed, and searchable by the entire team.

### Knowledge Flow

```
v1: Knowledge is local, trapped in one WhatsApp session
    Claude learns things → session ends → knowledge lost (unless memory.md)

v2: Knowledge flows through git
    Claude learns things → writes to CLAUDE.md → PR merge → shared with all threads
    Natural review gate: bad learnings get caught in PR review
    Natural conflict resolution: git merge
```

### Team Scale

v1 was designed for one person talking to one bot in one WhatsApp group.

v2 naturally supports teams:
- Multiple people can create threads in the same channel
- Each thread is independent — no stepping on each other
- Shared knowledge via CLAUDE.md in the repo
- Slack's permission model handles access control

### Summary

| Aspect | v1 (WhatsApp) | v2 (Slack Threads) |
|---|---|---|
| State machine | ~300 lines, 4 states, 3 timers | None |
| Concurrency | 1 conversation at a time | N concurrent threads |
| UX | Flat chat, `[BOT]` prefix | Structured threads, native bot identity |
| Knowledge sharing | Local memory.md | CLAUDE.md via PR merge |
| Searchability | Local SQLite | Slack search (team-wide) |
| Team support | Single user | Multi-user native |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |
| Authentication | QR code + phone linking | Bot token (one-time setup) |
| Setup per repo | QR scan + group create | Just pick a channel |

---

## 23. Thread History — Development Journal per PR

When Claude opens a PR from a thread, it generates a summary of the entire
thread conversation and commits it as `history/<threadId>.md`. This file
is part of the PR, giving reviewers full visibility into the development
process: what was asked, what was tried, what decisions were made.

### The Flow

```
Thread 1732456789.123456: "fix the login bug"
    ↓ user: "fix the login bug"
    ↓ claude: "I see the issue, the session check..."
    ↓ user: "also check the remember me checkbox"
    ↓ claude: "Done. Opening PR..."
    ↓
Daemon detects PR creation in Claude's output
    ↓
Generate thread summary (via Kimi or Claude)
    ↓
Write to: history/1732456789.123456.md
    ↓
git add + amend into PR commit (or new commit)
    ↓
File is now part of the PR ✓
```

### File Format

```markdown
<!-- history/1732456789.123456.md -->
# Fix login bug and remember-me checkbox

**Thread**: 1732456789.123456
**Channel**: #codebutler-backend
**Date**: 2026-02-14
**PR**: #42

## Conversation

**leandro** (15:04): fix the login bug

**claude** (15:04): I found the issue in `auth/session.go`. The session
validation was checking expiry with `time.Now()` but the stored timestamp
was in UTC while the comparison used local time...

**leandro** (15:06): also check the remember me checkbox

**claude** (15:07): The remember-me checkbox wasn't persisting because
the cookie `MaxAge` was set to 0 (session cookie) instead of 30 days...

## Changes Made

- `auth/session.go`: Fixed timezone comparison in session validation
- `auth/login.go`: Set cookie MaxAge to 30 days when remember-me is checked
- `auth/session_test.go`: Added test for UTC vs local time edge case

## Decisions

- Used 30 days for remember-me duration (standard practice)
- Kept session cookies (MaxAge=0) for non-remember-me logins
```

### Detection: When to Generate

The daemon watches Claude's response for PR creation signals:

```go
// Detect PR URL in Claude's response
prURLPattern := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)

// Or detect `gh pr create` in Claude's tool calls
ghPRPattern := regexp.MustCompile(`gh pr create`)
```

When detected:
1. Fetch full thread history from Slack (`conversations.replies`)
2. Generate summary (Kimi for cost, or Claude for quality)
3. Write `history/<threadTS>.md`
4. Commit and push to the PR branch

### Summary Generation Prompt

```
You are writing a development journal entry for a PR.

Given a Slack thread conversation between a developer and an AI assistant,
produce a markdown document with:

1. **Title**: one-line summary of what was done
2. **Conversation**: key exchanges (skip noise, keep decisions and reasoning)
3. **Changes Made**: list of files changed and what was done to each
4. **Decisions**: architectural or implementation decisions made during the thread

Keep it concise but useful for a PR reviewer who wants to understand
the "why" behind the changes.

Thread:
---
{thread messages}
---
```

### Directory Structure

```
history/
  1732456789.123456.md    # Thread: fix login bug → PR #42
  1732460000.654321.md    # Thread: add password reset → PR #43
  1732470000.111111.md    # Thread: refactor API → PR #44
```

### Not Loaded by Default

History files are **not injected into Claude's context**. They exist for
humans, not for the AI:

- Claude already has full context via `--resume` within a thread
- Loading history into the prompt would bloat context with past conversations
- The `history/` folder can grow large over time (one file per PR)

**When history IS useful**:
- A user asks in Slack: "why did we change the auth flow?" → a human (or
  Claude, if asked) can `grep history/ -l "auth"` to find the relevant thread
- PR reviewers click into `history/` in the PR diff to understand the conversation
- Post-mortems: trace a bug back to the thread + decisions that introduced it
- Onboarding: new devs browse `history/` to understand project evolution

The history folder is a **passive archive** — always there, never in the way.

### Why This Is Useful

1. **PR reviewers** see the full development context — not just the diff,
   but the conversation that led to the decisions
2. **Future developers** can search `history/` to understand why something
   was built a certain way
3. **Onboarding** — new team members can read through history to understand
   the project's evolution
4. **Accountability** — every change has a traceable conversation thread
5. **Post-mortems** — if a bug is introduced, trace back to the thread
   that created it

### Relationship to Other Knowledge Features

```
history/<threadId>.md  →  "What happened" (passive archive, for humans)
CLAUDE.md              →  "What we know" (loaded by Claude, shared knowledge)
.codebutler/memory.md  →  "What the bot remembers" (loaded by Claude, local)
```

Three complementary layers with different audiences:

| Layer | Loaded by Claude | Audience | Purpose |
|---|---|---|---|
| `history/` | No (passive) | Humans: reviewers, devs, onboarding | What happened and why |
| `CLAUDE.md` | Yes (auto) | Claude + humans | Codebase conventions, shared knowledge |
| `memory.md` | Yes (auto) | Claude only | Operational memory, local learnings |

### Implementation

- **File**: `internal/history/history.go`
- **Functions**:
  - `DetectPR(claudeResponse string) (prNumber int, found bool)`
  - `FetchThread(slackClient, channelID, threadTS string) []Message`
  - `GenerateSummary(messages []Message, prNumber int) string`
  - `WriteAndCommit(repoDir, threadTS, summary string) error`
- **Integration**: called in daemon after `processBatch` when PR is detected

---

## 24. Multi-Model Orchestration — Claude Executes, Cheap Models Organize

CodeButler is Claude Code with extras. Claude stays as the sole code executor.
But the "extras" — everything that happens before, around, and after Claude —
can be powered by cheaper, faster models. The principle:

```
Cheap models (Kimi, GPT-4o-mini) = orchestrators    (~$0.001/call)
Claude                            = executor          (~$0.10-1.00/call)
OpenAI                            = media specialist   (Whisper, gpt-image-1)
```

**The goal is NOT to replace Claude. It's to make every Claude call maximally
effective by doing the cheap work before and after.**

### Model Roles

| Model | Role | Cost | Used For |
|---|---|---|---|
| **Claude** (claude -p) | Code executor | $$$ | Write code, fix bugs, refactor, create PRs |
| **Kimi** (OpenAI-compat API) | Orchestrator | ¢ | Triage, enrich prompts, extract memory, summarize |
| **GPT-4o-mini** | Orchestrator alt | ¢ | Same as Kimi, interchangeable |
| **Whisper** | Transcription | ¢ | Voice → text |
| **gpt-image-1** | Image generation | ¢¢ | /create-image |

### 24.1 Pre-flight: Enrich Before Claude Runs

When a user sends "fix the login bug", Claude wastes expensive turns
exploring the codebase to find what "login" means. Kimi can do this
cheaper and faster.

```
User: "fix the login bug"
    ↓
Kimi pre-flight (cheap, fast):
    1. grep repo for "login" → finds auth/login.go, auth/session.go
    2. Read those files (or summaries)
    3. Check recent git log for login-related changes
    4. Check memory.md for known login conventions
    5. Build enriched prompt:
       "Fix the login bug. Relevant files:
        - auth/login.go (handles POST /login, bcrypt check)
        - auth/session.go (session creation, 24h expiry)
        - Recent: commit abc123 changed session timeout
        - Memory: auth uses bcrypt, sessions expire after 24h"
    ↓
Claude receives focused, enriched prompt
    → Fewer exploration turns → faster, cheaper
```

**Implementation**:
```go
// internal/preflight/preflight.go

type Context struct {
    RelevantFiles []FileInfo   // paths + summaries
    RecentCommits []string     // related git log entries
    MemoryHits    []string     // relevant memory.md lines
    EnrichedPrompt string     // final prompt for Claude
}

func Enrich(kimiClient, repoDir, userMessage, memory string) (*Context, error)
```

### 24.2 Smart Routing: Not Everything Needs Claude

Some messages don't need a $1 Claude call. Kimi can handle simple
questions directly, and only escalate to Claude when code changes
are needed.

```
User message
    ↓
Kimi classifier (1 API call, ~$0.001):
    ├─ "question"  → Kimi answers directly (grep + read + respond)
    ├─ "code_task" → Claude (full agent with tools)
    ├─ "clarify"   → Kimi asks follow-up question
    └─ "off_topic" → Kimi politely redirects
```

**Examples**:

| Message | Classification | Handler |
|---|---|---|
| "fix the login bug" | code_task | Claude |
| "what does the auth middleware do?" | question | Kimi (read file + summarize) |
| "how do we deploy?" | question | Kimi (check README/docs) |
| "refactor the API to use REST" | code_task | Claude |
| "is the CI passing?" | question | Kimi (check gh status) |
| "login" | clarify | Kimi: "Can you describe the issue?" |

**Cost impact**: If 40% of messages are questions, that's 40% fewer Claude
calls. At ~$0.50/Claude call, this adds up fast.

**Implementation**:
```go
// internal/router/router.go

type Route string
const (
    RouteCode    Route = "code_task"
    RouteQuestion Route = "question"
    RouteClarify  Route = "clarify"
)

func Classify(kimiClient, message string) (Route, error)
func AnswerQuestion(kimiClient, repoDir, question string) (string, error)
```

### 24.3 Thread Workflow: Plan → Approve → Execute

For complex tasks, Kimi creates a structured work plan before Claude
executes anything. The user reviews and approves the plan first.

```
User: "add user registration with email verification"
    ↓
Kimi workflow planner:
    1. Analyze request complexity → "complex, multi-file"
    2. Scan codebase for existing patterns (auth, models, routes)
    3. Generate execution plan:

        📋 *Work Plan*
        1. Create `models/user.go` — User struct + DB schema
        2. Create `auth/register.go` — POST /register endpoint
        3. Create `auth/verify.go` — GET /verify?token=... endpoint
        4. Create `email/send.go` — verification email sender
        5. Update `routes.go` — add new endpoints
        6. Add tests for registration flow

        Estimated: ~5 Claude turns
        Reply *yes* to execute, or describe changes.
    ↓
User: "yes" (or "skip email verification for now")
    ↓
Claude executes the approved plan (or adjusted version)
```

**Why this is better than Claude planning**:
- Kimi plan costs ~$0.002. Claude plan costs ~$0.50.
- User gets to review before expensive execution starts
- If user says "no, different approach" → saved a full Claude call
- Claude gets a structured plan as input → more focused execution

### 24.4 Post-flight: After Claude Responds

After Claude finishes, cheap models handle the aftermath:

```
Claude response arrives
    ↓ (parallel, non-blocking)
    ├─ Kimi: extract memory/learnings → update memory.md
    ├─ Kimi: detect PR creation → generate history/<thread>.md
    ├─ Kimi: summarize for Slack (if response is very long)
    └─ Kimi: detect if Claude left TODO/FIXME → warn in thread
```

### 24.5 Multi-Thread Conflict Detection

When multiple threads are active, Kimi monitors for conflicts:

```
Thread A: "refactor auth module" → Claude working on auth/
Thread B: "fix login timeout"    → Claude working on auth/

Kimi detects overlap:
    → Posts in Thread B: "⚠️ Thread A is also modifying auth/.
       Consider waiting for Thread A to finish, or coordinate."
```

**Implementation**:
```go
// Track which files each thread is touching
type ThreadScope struct {
    ThreadTS string
    Files    []string  // files modified by Claude in this thread
}

// On each Claude response, Kimi extracts modified files
// and checks against active threads
func DetectConflicts(activeThreads []ThreadScope, newThread ThreadScope) []Conflict
```

### 24.6 Cost Dashboard

Track and display cost per model, per thread, per day:

```
Thread 1732456789.123456: "fix login bug"
    Kimi:   3 calls  ·  $0.003
    Claude: 2 calls  ·  $0.84
    Total:            ·  $0.843

Daily: Claude $12.40 · Kimi $0.15 · Whisper $0.02 · Total $12.57
```

Exposed in the web dashboard (`/api/costs`) and optionally posted
to Slack weekly.

### 24.7 The Full Pipeline

Putting it all together — a single message flows through this pipeline:

```
┌─────────────────────────────────────────────────────┐
│                   USER MESSAGE                       │
└──────────────────────┬──────────────────────────────┘
                       ↓
              ┌────────────────┐
              │  Kimi: CLASSIFY │  ~$0.001, <1s
              └───────┬────────┘
                      │
         ┌────────────┼────────────┐
         ↓            ↓            ↓
    [question]   [code_task]   [clarify]
         │            │            │
         ↓            ↓            ↓
   Kimi answers  ┌─────────┐  Kimi asks
   directly      │  Kimi:   │  follow-up
   (~$0.002)     │ ENRICH   │
                 │ + PLAN   │  ~$0.003
                 └────┬─────┘
                      ↓
              ┌───────────────┐
              │ User approves │  (for complex tasks)
              │   the plan    │
              └───────┬───────┘
                      ↓
              ┌───────────────┐
              │    Claude:    │
              │   EXECUTE     │  ~$0.50-2.00
              │  (code agent) │
              └───────┬───────┘
                      ↓
         ┌────────────┼────────────┐
         ↓            ↓            ↓
   Kimi: extract  Kimi: detect  Kimi: format
   memory         PR → history  response
   (~$0.001)      (~$0.002)     (~$0.001)
         ↓            ↓            ↓
└─────────────────────────────────────────────────────┐
│                SLACK THREAD RESPONSE                  │
└──────────────────────────────────────────────────────┘
```

### 24.8 Model Client Abstraction

All cheap models use OpenAI-compatible APIs. One client handles them all:

```go
// internal/llm/client.go

type Client struct {
    baseURL string
    apiKey  string
    model   string
}

// Kimi, GPT-4o-mini, DeepSeek — all OpenAI-compatible
func NewKimi(apiKey string) *Client {
    return &Client{baseURL: "https://api.moonshot.cn/v1", apiKey: apiKey, model: "moonshot-v1-8k"}
}

func NewGPT4oMini(apiKey string) *Client {
    return &Client{baseURL: "https://api.openai.com/v1", apiKey: apiKey, model: "gpt-4o-mini"}
}

func (c *Client) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error)
func (c *Client) ChatJSON(ctx context.Context, systemPrompt, userMessage string, out interface{}) error
```

### 24.9 Config

```json
// ~/.codebutler/config.json (global)
{
  "orchestrator": {
    "provider": "kimi",           // or "openai-mini", "deepseek"
    "apiKey": "...",
    "enablePreflight": true,      // enrich prompts before Claude
    "enableRouting": true,        // classify messages, skip Claude for questions
    "enablePlanning": true,       // generate plans for complex tasks
    "enableConflictDetection": true
  }
}
```

If no orchestrator is configured, all features are bypassed and messages
go directly to Claude (current v1 behavior). This keeps the system
functional without the cheap model — it just costs more per call.

### 24.10 What This Means for CodeButler's Identity

CodeButler remains **Claude Code with extras**. The core loop is unchanged:

```
message → claude -p → response
```

The orchestration layer is invisible to the user. They still talk to
"the bot" in Slack. They don't know (or care) that Kimi triaged their
message, enriched the prompt, and extracted memory afterward.

Claude is still the only model that touches code. The cheap models
never write code, never run tools, never modify files. They only
read, classify, summarize, and plan.

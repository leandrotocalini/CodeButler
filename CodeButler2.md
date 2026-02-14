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

### System Requirements
- `gh` (GitHub CLI) — **required**, must be installed and authenticated
  - Used by Claude to create PRs, push branches, manage issues
  - Used by the daemon to check PR status, detect merges, fetch PR diffs
  - Auth: `gh auth login` (one-time setup, stored in `~/.config/gh/`)
  - The setup wizard (`--setup`) verifies `gh auth status` and prompts if not configured

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
| `internal/github/github.go` | PR detection, merge polling, PR description updates via `gh` |
| `internal/models/interfaces.go` | ProductManager, Artist, Coder interfaces + shared types |
| `internal/provider/openai/client.go` | Shared OpenAI HTTP client (auth, rate limiting, retries) |
| `internal/provider/openai/product_manager.go` | Thin adapter: shared client → ProductManager interface |
| `internal/provider/openai/artist.go` | Thin adapter: shared client → Artist interface |
| `internal/provider/openai/coder.go` | Thin adapter: shared client → Coder interface (future) |
| `internal/provider/claude/coder.go` | Claude CLI Coder (exec.Command, not HTTP) |
| `internal/tools/definition.go` | PM tool definitions (ReadFile, Grep, ListFiles, etc.) + PMTools() factory |
| `internal/tools/executor.go` | Sandboxed tool execution: read files, grep, git ops — all read-only |
| `internal/tools/loop.go` | Provider-agnostic tool-calling loop (ChatFunc + Executor → final response) |
| `internal/router/router.go` | Message classifier: question vs code_task vs clarify |
| `internal/conflicts/tracker.go` | Thread lifecycle tracking, file overlap detection, merge order |
| `internal/conflicts/notify.go` | Slack notifications for conflicts, rebase reminders, merge order |

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
    ├─ thread_ts == "" → new top-level message → KIMI STARTS (always)
    └─ thread_ts != "" → reply in existing thread → route to Kimi or Claude
```

### Thread Phases: Kimi First, Claude Second

Every thread goes through two phases. Kimi **always** starts.
Claude **never** starts without user approval.

```
PHASE 1 — KIMI (definition & planning)
    User sends message
        ↓
    Kimi responds: asks questions, proposes approach, shows plan
        ↓
    User replies: refines, adjusts, adds details
        ↓
    Kimi updates plan, shows final proposal
        ↓
    User says "yes" / "dale" / "go" / approves
        ↓
PHASE 2 — CLAUDE (implementation)
    Daemon creates worktree + branch
        ↓
    Claude receives: approved plan + context + relevant files
        ↓
    Claude implements (can ask implementation questions via --resume)
        ↓
    Claude opens PR → thread lifecycle continues
```

**Kimi defines WHAT to build. Claude decides HOW to build it.**

Kimi resolves all product/scope questions before Claude starts. By the
time Claude runs, the feature/bug/task is well-defined. Claude can still
ask the user questions, but they should be **implementation questions**
(e.g., "should I use a middleware or a handler for this?", "the test
suite uses testify but this module uses stdlib — which do you prefer?"),
not **requirements questions** (e.g., "what do you mean by login bug?",
"what fields should the user have?"). Those were already resolved with Kimi.

### Thread State in the Daemon

```go
type ThreadPhase string
const (
    PhaseKimi   ThreadPhase = "kimi"    // Kimi is talking to the user
    PhaseClaude ThreadPhase = "claude"  // User approved, Claude is working
)

type Thread struct {
    ThreadTS    string
    Phase       ThreadPhase
    KimiHistory []Message     // conversation so far (for Kimi context)
    Plan        string        // Kimi's approved plan (passed to Claude)
    Branch      string        // set when Phase transitions to claude
    SessionID   string        // Claude session ID (for --resume)
    Images      []GeneratedImage // images created by Kimi during Phase 1
}

type GeneratedImage struct {
    LocalPath   string // path on disk (e.g., .codebutler/images/favicon.png)
    SlackFileID string // Slack file ID after upload
    Prompt      string // generation prompt used
    RepoPath    string // target path in repo (set if user wants to push)
}
```

### Message Routing

```go
func (d *Daemon) onMessage(msg Message) {
    thread := d.getOrCreateThread(msg.ThreadTS)

    switch thread.Phase {
    case PhaseKimi:
        // Check if user is approving Kimi's plan (code task → Claude)
        if isApproval(msg.Text) && thread.Plan != "" {
            thread.Phase = PhaseClaude
            d.startClaude(thread)
            return
        }
        // Otherwise, Kimi continues the conversation.
        // This includes: asking questions, scanning code, generating
        // images, iterating on images, pushing assets, or ending
        // the thread (all handled within Kimi's phase).
        d.runKimi(thread, msg)

    case PhaseClaude:
        // User replied after Claude started — resume Claude
        // These are implementation interactions: answering Claude's
        // questions, requesting adjustments, providing feedback
        d.resumeClaude(thread, msg)
    }
}

func isApproval(text string) bool {
    approvals := []string{"yes", "si", "sí", "dale", "go", "do it", "proceed", "ok", "lgtm"}
    lower := strings.ToLower(strings.TrimSpace(text))
    for _, a := range approvals {
        if lower == a { return true }
    }
    return false
}
```

### Concurrency Model

```go
type Daemon struct {
    threads  map[string]*Thread  // thread_ts → thread state
    threadMu sync.Mutex
}
```

Multiple threads can be in different phases simultaneously:
- Thread A: Kimi is discussing with user (no worktree, no Claude)
- Thread B: User just approved, Claude is coding (worktree active)
- Thread C: Kimi is asking clarifying questions (no worktree)
- Thread D: Claude finished, PR opened, waiting for merge

Kimi threads are cheap (~$0.001/message). Many can run in parallel.
Claude threads are expensive. Limited by `maxConcurrentThreads`.

### Sending
```
Kimi/Claude response
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
- [x] **memory.md with approval**: after PR creation, Kimi proposes memory updates in thread, user approves/edits, committed to PR branch
- [x] **Kimi self-improvement**: Kimi analyzes what Claude asked → adds "Planning Notes" so it handles it next time
- [x] **memory.md via git flow**: memory.md committed to repo, follows PR flow, merges to main with the PR
- [x] **PR as journal**: thread summary goes in PR description (via `gh pr edit`), no files committed
- [x] **Multi-model**: Claude executes code, cheap models (Kimi/GPT-4o-mini) orchestrate around it
- [x] **Swappable providers**: three interfaces (ProductManager, Artist, Coder), all configurable via config.json
- [x] **Kimi first, always**: Kimi starts every thread. Scans repo, asks questions, proposes plan. Claude never starts without approval
- [x] **Approval gate**: user must explicitly approve before Claude runs. "yes"/"dale"/"go" triggers Phase 2
- [x] **Questions never reach Claude**: Kimi answers questions directly (reads files, checks docs). Thread ends without Claude
- [x] **Thread = Branch = PR**: each thread creates exactly one branch, one PR. Non-negotiable 1:1:1 mapping
- [x] **User closes thread**: user says "merge"/"done" → CodeButler writes summary, commits memory, merges PR, deletes branch
- [x] **Cross-thread references**: link old threads/PRs in new thread for read-only context. Rule stays: 1 thread = 1 branch = 1 PR
- [x] **PR ↔ Thread link**: PR description includes Slack thread URL, thread shows PR URL. Bidirectional
- [x] **`gh` CLI required**: all GitHub operations (PRs, merge detection, diffs) via `gh`. No API tokens needed
- [x] **Merge coordination**: Kimi suggests merge order + notifies threads to rebase after PR merge

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

## 16. Memory with User Approval (Kimi)

The daemon extracts learnings after creating a PR and **shows them to the
user for approval** before committing to `memory.md`. The user controls what
gets remembered. Uses Kimi (cheap and fast) instead of Claude.

### File

```
<repo>/memory.md
```

Committed to the repo — not gitignored. Injected as context into the Claude
and Kimi prompts on each thread. Lives at the repo root alongside `CLAUDE.md`.

### The Git Flow

memory.md follows the same PR flow as code:

1. **After PR creation**: Kimi extracts learnings → proposes in thread → user approves
2. **Kimi commits** the approved changes to `memory.md` on the PR branch and pushes
3. **memory.md update is part of the PR** — visible in the diff, reviewable
4. **On merge**: memory.md lands on main → available to all future threads

This means memory is:
- **Versioned** — full git history of every learning
- **Reviewable** — part of the PR diff, reviewer can see what the bot learned
- **Branch-isolated** — thread B doesn't see thread A's learnings until merge
- **Conflict-resolved** — git merge handles concurrent memory updates

### Trigger

Memory extraction triggers after a PR is created:

1. **PR created by Coder** — main path for code tasks
2. **PR created by Kimi** — for image/asset tasks pushed via `gh pr create`
3. **User says "done"** — for threads that don't produce a PR (questions,
   images shown in Slack only) → committed directly to main

When triggered, the daemon:

1. Reads current `memory.md` from the PR branch
2. Reads the full thread conversation (whatever phases occurred)
3. Calls Kimi to analyze and propose memory updates
4. **Posts the proposed changes in the Slack thread** for user review
5. User approves, edits, or adds more learnings
6. Daemon commits approved changes to `memory.md` on the PR branch and pushes

### The Feedback Loop

Kimi doesn't just extract general learnings — it specifically analyzes
what Claude asked during implementation. If Claude asked something that
Kimi should have resolved in Phase 1, that becomes a learning for Kimi.

```
Thread conversation:
  [Kimi] Plan: 1. Create user model  2. Add POST /register  3. Tests
  [User] yes
  [Claude] auth/login.go uses JWT but auth/session.go uses cookies.
           Which pattern for registration?
  [User] JWT, always JWT
  [Claude] Done. PR opened.
  → PR merged

Kimi's analysis:
  Claude asked an implementation question that Kimi could have
  pre-resolved by reading auth/login.go and auth/session.go.

Kimi proposes in thread:
  📝 *Proposed memory updates:*
  1. ➕ `- Auth pattern: always use JWT, never cookies (auth/login.go is the reference)`
  2. ➕ `- When planning auth-related tasks, check auth/login.go for the JWT pattern`

  Reply *yes* to save, or suggest changes.
```

### What the User Sees in the Thread

```
[Bot] 📝 PR #42 merged! Here's what I'd like to remember:

  1. ➕ Auth pattern: always use JWT, never cookies
  2. ➕ When planning auth tasks, always reference auth/login.go for patterns
  3. 📎 Kimi learning: next time a task touches auth, pre-check JWT vs cookie pattern before asking Claude

  Reply *yes* to save all, or tell me what to change.
```

**For image-only threads** (no Claude, no PR):
```
[Bot] 📝 Thread done! Here's what I'd like to remember:

  1. ➕ Logo style: minimalist, blue #2563EB, no text
  2. ➕ User prefers "lost astronaut" motif over robots for error pages
  3. 📎 Planning: when generating images for this project, use blue #2563EB as primary color

  Reply *yes* to save all, or tell me what to change.
```

The user can:
- **"yes"** → save all proposed changes
- **"remove 3"** → save 1 and 2, skip 3
- **"add: we use bcrypt for passwords, never md5"** → add a custom learning
- **"change 1 to: Auth uses JWT everywhere except WebSocket handlers"** → edit before saving
- **"no"** → discard all, save nothing

### Kimi Prompt (Memory Extraction)

```
You analyze completed conversations to extract learnings for memory.
You receive the full thread, which may include any combination of:
Kimi's planning phase, image generation, user interactions,
and Claude's implementation phase.

Your job has TWO parts:

PART 1 — General learnings:
Extract useful decisions, conventions, and gotchas worth remembering.
For image threads: style preferences, color choices, motifs, formats.

PART 2 — Kimi self-improvement:
- If Claude ran: look at what Claude asked. If it could have been
  answered during planning, propose a planning note.
- If images were generated: note style preferences, prompt adjustments
  the user made, preferred formats/sizes. Next time Kimi generates
  images for this project, it should use these preferences by default.

Respond with a JSON array of operations:
- {"op": "append", "line": "- ...", "category": "project"}
    — project decision, convention, or pattern
- {"op": "append", "line": "- ...", "category": "planning"}
    — something Kimi should check/resolve during Phase 1 next time
- {"op": "replace", "old": "exact existing line", "new": "merged line"}
    — update existing knowledge with new info
- {"op": "none"}
    — nothing worth remembering

Rules:
- Distinguish between project knowledge and planning improvements
- "planning" learnings tell Kimi what to pre-check next time
- Keep each line concise (1 line max)
- Only record genuinely useful knowledge — not trivia
- Use "replace" to merge with existing entries when possible

Current memory:
---
{contents of memory.md}
---

Thread conversation:
---
{full thread: kimi planning + user messages + claude implementation}
---
```

### Expected Response

```json
[
  {"op": "append", "line": "- Auth: always JWT, never cookies. Reference: auth/login.go", "category": "project"},
  {"op": "append", "line": "- Planning: when task touches auth, pre-read auth/login.go and auth/session.go to identify JWT vs cookie pattern", "category": "planning"},
  {"op": "replace", "old": "- Registration endpoint exists at /register", "new": "- Registration at POST /register, uses JWT (same as login)"}
]
```

### memory.md Format

```markdown
# Project Knowledge
- Auth: always JWT, never cookies. Reference: auth/login.go
- Registration at POST /register, uses JWT (same as login)
- Tests use testify, not stdlib testing
- Deploy: make build → docker push → kubectl apply
- Visual style: blue #2563EB primary, flat design, rounded corners
- Error pages use "lost astronaut" motif (not robots)
- Icons: 24px, 2px stroke, outlined style (see static/icons/)

# Planning Notes
- When task touches auth, pre-read auth/login.go and auth/session.go
- When task involves models, check existing models/ for GORM conventions
- Always mention the test framework (testify) in plans so Claude doesn't ask
- Image generation: default to blue #2563EB, flat design, transparent background
- When generating icons, reference existing set in static/icons/ for consistency
```

Two sections: **Project Knowledge** (what the codebase does) and
**Planning Notes** (what Kimi should check during Phase 1). Both are
injected into prompts, but Planning Notes specifically help Kimi
produce better plans over time.

### Implementation

- **File**: `internal/memory/memory.go`
- **Functions**:
  - `Load(path) string` — read memory.md (or "" if doesn't exist)
  - `Apply(content string, ops []Op) string` — apply approved operations
  - `Save(path, content string)` — write memory.md
  - `FormatProposal(ops []Op) string` — format ops as Slack message for user review
  - `ParseUserResponse(text string, ops []Op) []Op` — process user edits/approvals
- **ProductManager**: uses `models.ProductManager` interface
  - Any provider that implements `Chat()` and `ChatJSON()`
  - Requires productManager config in global config
- **Daemon integration**: after PR creation, launch goroutine:
  1. Call Kimi for memory analysis
  2. Post proposal in thread
  3. Wait for user response (with timeout — if no response in 24h, discard)
  4. Commit approved changes to memory.md on the PR branch
  5. `git push` to update the PR

### The Virtuous Cycle

```
Thread N (code task):
  Kimi plans → Claude implements → Claude asks "JWT or cookies?"
  → User: "JWT" → PR created
  → Kimi proposes: "always JWT" + "pre-check auth pattern"
  → User: "yes"
  → memory.md committed to PR branch + pushed
  → PR merged → memory.md lands on main ✓

Thread N+1 (touches auth, branched from main after merge):
  Kimi reads memory.md from main → knows JWT pattern → includes it in plan
  → Claude never asks → faster, cheaper, better

Thread N+2 (image task):
  User: "create an icon for settings"
  → Kimi shows prompt with blue #2563EB (from memory)
  → User: "perfect, go"
  → Kimi generates, user says "push it"
  → PR created → Kimi learns: "settings icon is a gear" → committed to PR
  → PR merged → learning on main ✓

Thread N+3 (code + image):
  User: "add a loading spinner to the dashboard"
  → Kimi generates spinner icon (blue, flat, matching existing style —
    all from memory) → shows prompt, user approves
  → Kimi plans: "save spinner + update dashboard template"
  → Claude implements → no style questions needed

Thread N+4:
  Kimi catches something else Claude or itself would have asked
  → Another planning note added → committed to PR → merged to main
  → System keeps improving
```

Over time, Kimi's plans get more complete and its image prompts get
more accurate because memory accumulates the patterns and decisions
that matter. The user drives this process — nothing gets remembered
without their approval. And because memory follows git flow, it's
versioned, reviewable, and merge-safe.

---

## 17. Logging — Plain Structured Logs

Replace the dual system (ring buffer + TUI with ANSI) with a single channel
of plain, structured logs with good information.

### Format

```
2026-02-14 15:04:05 INF  slack connected
2026-02-14 15:04:08 MSG  leandro: "fix the login bug"
2026-02-14 15:04:09 KMI  thread 1707.123 → kimi responding
2026-02-14 15:04:11 RSP  kimi: "I see auth/login.go. What's the symptom?"
2026-02-14 15:04:30 MSG  leandro: "session expires too fast"
2026-02-14 15:04:32 KMI  thread 1707.123 → kimi responding
2026-02-14 15:04:35 RSP  kimi: "Found it. Plan: fix session.go:42. Yes?"
2026-02-14 15:04:40 MSG  leandro: "dale"
2026-02-14 15:04:40 INF  thread 1707.123 → approved, starting claude
2026-02-14 15:04:41 CLD  thread 1707.123 → claude running (new session)
2026-02-14 15:05:15 CLD  thread 1707.123 → done · 34s · 3 turns · $0.12
2026-02-14 15:05:15 RSP  claude: "Fixed session expiry. PR #42 opened."
2026-02-14 15:05:16 MEM  thread 1707.123 → proposing 2 memory updates
2026-02-14 15:05:17 RSP  kimi: "📝 Here's what I'd remember: ..."
2026-02-14 15:10:00 MSG  leandro: "yes"
2026-02-14 15:10:01 MEM  thread 1707.123 → committed memory.md to PR branch
2026-02-14 15:10:02 INF  thread 1707.123 → pushed to origin/fix/session-expiry
...
2026-02-14 16:20:00 MSG  leandro: "dale merge"
2026-02-14 16:20:01 INF  thread 1707.123 → generating PR summary
2026-02-14 16:20:03 INF  thread 1707.123 → PR #42 merged (squash)
2026-02-14 16:20:04 INF  thread 1707.123 → branch deleted, worktree removed
2026-02-14 16:20:04 INF  thread 1707.123 → closed
```

### Tags

| Tag | Meaning |
|-----|---------|
| `INF` | System info: connection, config, state |
| `WRN` | Warnings: reconnections, recoverable timeouts |
| `ERR` | Errors: API failures, recovered crashes |
| `DBG` | Debug: only if verbose mode is enabled |
| `MSG` | Incoming user message |
| `KMI` | Kimi activity: responding, planning |
| `IMG` | Image generation: prompt shown, generating, uploaded |
| `CLD` | Claude activity: start, done, resume |
| `RSP` | Response sent to channel (from Kimi or Claude) |
| `MEM` | Memory operations: propose, approve, save |

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

ALLOWED TOOLS:
- `gh` (GitHub CLI) — use for PRs, issues, checks. Already authenticated.
- `git` — commit, push, branch operations on YOUR branch only.
- Build/test tools native to this project (go test, npm test, xcodebuild, etc.)

WHEN CREATING A PR:
- Always use `gh pr create`
- Include this thread link in the PR description: {slack_thread_url}
- Format: ## Slack Thread\n{slack_thread_url}

You are working in: {worktree_path}
Your branch: {branch_name}
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

`memory.md` is committed to the repo and follows the same PR flow as code.
This is the single knowledge sharing mechanism — no separate local/shared
split needed.

### The Flow

```
Thread A: "fix the login bug"
    → Claude works on branch fix/login
    → PR created
    → Kimi extracts learnings → user approves → committed to memory.md on branch
    → PR merged to main
    → memory.md now on main ✓

Thread B: "add password reset" (started after merge)
    → Branches from main → memory.md already has Thread A's learnings
    → Kimi reads memory → knows "auth uses bcrypt, sessions expire after 24h"
    → Builds on existing knowledge ✓

Thread C: "refactor the API" (started BEFORE Thread A merged)
    → Still working on its branch, doesn't see Thread A's learnings
    → Gets the knowledge on next rebase from main
```

### Why This Is Elegant

1. **No custom sync** — git is the knowledge transport
2. **Isolation by default** — threads can't pollute each other's context
3. **Review gate** — memory updates are visible in the PR diff
4. **Conflict resolution** — git merge handles concurrent memory.md edits
5. **Audit trail** — every learning is a commit with full thread context
6. **Single source of truth** — one file, one mechanism, no local/shared split

### How memory.md Relates to CLAUDE.md

| | `memory.md` | `CLAUDE.md` |
|---|---|---|
| **Written by** | Kimi (automatic, with user approval) | Humans (manual) |
| **Content** | Learnings from conversations: patterns, decisions, planning notes | Codebase conventions, dev setup, architecture |
| **Updates** | Every PR (Kimi proposes, user approves) | Occasionally (by developers) |
| **Read by** | Kimi + Claude (injected into prompts) | Claude (auto-loaded) |
| **Committed** | Yes — via PR flow | Yes — manual commits |

Both live at the repo root. `CLAUDE.md` is the human-curated project guide.
`memory.md` is the bot's evolving knowledge, growing with every thread.

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
    Claude learns things → session ends → knowledge lost

v2: Knowledge flows through git
    Thread ends → Kimi extracts learnings → commits to memory.md on PR branch
    → PR merged → memory.md on main → all future threads inherit it
    Natural review gate: memory updates visible in PR diff
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
| Knowledge sharing | Local memory.md | memory.md via PR merge (git flow) |
| Searchability | Local SQLite | Slack search (team-wide) |
| Team support | Single user | Multi-user native |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |
| Authentication | QR code + phone linking | Bot token (one-time setup) |
| Setup per repo | QR scan + group create | Just pick a channel |

---

## 23. PR Description as Development Journal

No extra files. No `history/` folder. The **PR description IS the history**.
When a PR is created, the daemon generates a summary of the Slack thread
and puts it in the PR body via `gh pr edit`. GitHub keeps it forever.

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
Fetch thread messages (conversations.replies)
    ↓
Kimi generates summary (~$0.002)
    ↓
gh pr edit #42 --body "$(updated description)"
    ↓
PR description now has: summary + thread link + decisions ✓
```

### PR Description Format

```markdown
## Summary
Fixed timezone comparison in session validation and remember-me cookie.

## Changes
- `auth/session.go`: Fixed UTC vs local time comparison in session expiry
- `auth/login.go`: Set cookie MaxAge to 30 days when remember-me is checked
- `auth/session_test.go`: Added test for timezone edge case

## Decisions
- 30 days for remember-me duration (standard practice)
- Kept session cookies (MaxAge=0) for non-remember-me logins

## Participants
- @leandro — requested the fix, approved the plan
- @maria — suggested checking the remember-me checkbox

## Slack Thread
https://myworkspace.slack.com/archives/C0123ABCDEF/p1732456789123456
```

The **Participants** section logs everyone who interacted in the thread.
Authorization is simple: **anyone in the Slack channel can interact**.
The channel membership IS the access control — Slack handles it.

### Why PR Description Instead of Files

| Approach | Pros | Cons |
|---|---|---|
| `history/<threadId>.md` (old idea) | Searchable via grep, part of repo | Clutters repo, extra commits, grows forever |
| **PR description** (new) | Zero files, already where reviewers look, GitHub search works | Not in the repo (lives on GitHub) |

The PR description is the natural place:
- Reviewers **already read it** before reviewing code
- GitHub **indexes it** for search
- It's **permanent** — PRs are never deleted
- **Zero repo clutter** — no extra files, no extra commits
- The Slack thread link gives full conversation if the summary isn't enough

### GitHub Operations via `gh`

The daemon uses `gh` directly for all GitHub operations:

```go
// internal/github/github.go

// IsMerged checks if a PR has been merged
func IsMerged(prNumber int) (bool, error) {
    out, err := exec.Command("gh", "pr", "view", strconv.Itoa(prNumber),
        "--json", "state,mergedAt").Output()
    // parse JSON: state == "MERGED"
}

// UpdatePRDescription appends the thread summary to the PR body
func UpdatePRDescription(prNumber int, summary string) error {
    return exec.Command("gh", "pr", "edit", strconv.Itoa(prNumber),
        "--body", summary).Run()
}

// GetPRDiff fetches the diff for cross-thread references
func GetPRDiff(prNumber int) (string, error) {
    return exec.Command("gh", "pr", "diff", strconv.Itoa(prNumber)).Output()
}

// WatchPRs polls open PRs for merge status (runs every 60s)
func WatchPRs(tracker *conflicts.Tracker, onMerge func(threadTS string, pr int)) {
    for _, scope := range tracker.GetPRScopes() {
        merged, _ := IsMerged(scope.PRNumber)
        if merged {
            onMerge(scope.ThreadTS, scope.PRNumber)
        }
    }
}
```

No GitHub API tokens needed — `gh` handles authentication via its own
config (`~/.config/gh/`). One less secret to manage.

### Detection: When to Update PR Description

The daemon watches Claude's response for PR creation signals:

```go
// Detect PR URL in Claude's response
prURLPattern := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)

// Or detect `gh pr create` in Claude's tool calls
ghPRPattern := regexp.MustCompile(`gh pr create`)
```

When detected:
1. Fetch full thread history from Slack (`conversations.replies`)
2. Generate summary via Kimi (~$0.002)
3. `gh pr edit <number> --body <summary + thread link>`

### Summary Generation Prompt (Kimi)

```
Given a Slack thread conversation between a developer and an AI assistant,
generate a PR description with these sections:

## Summary
1-3 sentences describing what was done and why.

## Changes
Bullet list of files changed and what was done to each.

## Decisions
Bullet list of architectural/implementation decisions made during the thread.

Keep it concise. A PR reviewer should understand the "why" in 30 seconds.

Thread:
---
{thread messages}
---
```

### Bidirectional Links

```
Slack thread                          GitHub PR
┌─────────────────┐                  ┌─────────────────────┐
│ user: "fix the  │                  │ ## Summary           │
│   login bug"    │                  │ Fixed timezone...    │
│                 │    PR link       │                      │
│ claude: "Fixed. │ ──────────────→  │ ## Slack Thread      │
│   PR #42: url"  │                  │ https://slack.com/.. │
│                 │  ←────────────── │                      │
└─────────────────┘    thread link   └─────────────────────┘
```

From Slack: click the PR URL to see the diff.
From GitHub: click the thread URL to see the conversation.

### Knowledge Layers (Simplified)

No more `history/` folder. Two layers instead of three:

| Layer | Loaded by Claude | Audience | Purpose |
|---|---|---|---|
| `CLAUDE.md` | Yes (auto) | Claude + humans | Codebase conventions, project guide |
| `memory.md` | Yes (injected) | Kimi + Claude | Learnings from conversations, planning notes |
| PR description | No | Humans (reviewers) | What happened and why (lives on GitHub) |

### Implementation

- **File**: `internal/github/github.go`
- **Functions**:
  - `DetectPR(claudeResponse string) (prNumber int, found bool)`
  - `UpdatePRDescription(prNumber int, summary string) error`
  - `IsMerged(prNumber int) (bool, error)`
  - `GetPRDiff(prNumber int) (string, error)`
  - `WatchPRs(tracker, onMerge)`
- **Summary generation**: via `models.ProductManager` interface (Kimi, GPT-4o-mini, etc.)
- **Thread fetching**: via Slack `conversations.replies` in `internal/slack/handler.go`
- **Integration**: called in daemon after PR is detected in Claude's response

---

## 24. Multi-Model Orchestration — Claude Executes, Cheap Models Organize

CodeButler is Claude Code with extras. Claude stays as the sole code executor.
But the "extras" — everything that happens before, around, and after Claude —
can be powered by cheaper, faster models. The principle:

```
ProductManager (Kimi, GPT-4o-mini, ...) = brain          (~$0.001/call)
Coder (Claude CLI)                     = hands          (~$0.10-1.00/call)
Artist (gpt-image-1, DALL-E, ...)      = eyes           (~$0.01-0.04/image)
Whisper                                = ears           (~$0.006/min)
```

**The goal is NOT to replace Claude. It's to make every Claude call maximally
effective by doing the cheap work before and after.**

All three roles are defined as Go **interfaces** (section 24.8). Providers
are swappable via config without touching daemon code.

### Model Roles

| Interface | Default Provider | Role | Cost | Used For |
|---|---|---|---|---|
| **Coder** | Claude CLI (`claude -p`) | Code executor | $$$ | Write code, fix bugs, refactor, create PRs |
| **ProductManager** | Kimi (OpenAI-compat API) | Brain | ¢ | Triage, plan, memory, summarize, coordinate |
| **ProductManager** | GPT-4o-mini (alt) | Brain | ¢ | Same as Kimi, interchangeable |
| **Artist** | gpt-image-1 | Image creator | ¢¢ | Generate/edit images during Phase 1 |
| *(direct)* | Whisper | Transcription | ¢ | Voice → text |

### 24.1 PM Tools — Autonomous Codebase Exploration

The ProductManager doesn't receive pre-digested context. Instead, it gets
**tools** — read-only functions it can call autonomously during conversation.
This is fundamentally different from the old "preflight enrichment" approach:

```
OLD (pre-flight — hardcoded, brittle):
    User: "fix the login bug"
    → CodeButler code greps for "login", reads files, checks git log
    → Builds a fixed prompt and sends it to Kimi
    → Kimi only sees what CodeButler decided to fetch
    → If CodeButler missed relevant context, Kimi can't explore further

NEW (PM tools — autonomous, flexible):
    User: "fix the login bug"
    → Kimi receives the message + available tools
    → Kimi decides: "I should search for login-related files"
    → Kimi calls Grep(pattern="login|Login", glob="*.go")
    → Kimi sees results, decides: "auth/login.go looks relevant"
    → Kimi calls ReadFile(path="auth/login.go")
    → Kimi calls ReadFile(path="auth/session.go")
    → Kimi calls GitLog(path="auth/")
    → Kimi: "Found the issue. auth/session.go:42 hardcodes 1h..."
    → Kimi proposes plan with full context it gathered itself
```

The PM is the one deciding what to explore, not CodeButler. This makes the
system adaptive — different questions trigger different exploration patterns.

#### Tool-Calling Flow

The ProductManager uses the standard OpenAI function-calling protocol.
This works with any OpenAI-compatible API (Kimi, GPT-4o-mini, DeepSeek, etc).

```
┌──────────┐     messages + tool_defs      ┌──────────┐
│          │ ──────────────────────────────→ │          │
│ CodeButler│                               │  Kimi    │
│ (Go)     │ ←──── finish_reason: tool_calls│  (LLM)   │
│          │       tool_calls: [{           │          │
│          │         name: "Grep",          │          │
│          │         arguments: {...}       │          │
│          │       }]                       │          │
│          │                                │          │
│ execute  │     messages + tool_results    │          │
│ locally  │ ──────────────────────────────→│          │
│          │                                │          │
│          │ ←──── finish_reason: tool_calls│          │
│          │       (more tools...)          │          │
│          │                                │          │
│          │     messages + tool_results    │          │
│          │ ──────────────────────────────→│          │
│          │                                │          │
│          │ ←──── finish_reason: stop      │          │
│          │       content: "Found it..."  │          │
└──────────┘                                └──────────┘
```

Each round-trip is ~100-300 tokens extra. A typical exploration is 3-5 tool
calls. Total cost per PM interaction: ~$0.002-0.005 (still 100x cheaper
than one Claude turn).

#### Tool Definitions

All PM tools are **strictly read-only** and **sandboxed to the repo directory**.
The PM never writes files, never executes arbitrary commands, never escapes
the repo root.

**`ReadFile`** — Read file contents with line numbers

```json
{
  "name": "ReadFile",
  "description": "Read the contents of a file in the repository. Returns file text with line numbers. Use offset/limit for large files.",
  "parameters": {
    "type": "object",
    "properties": {
      "path":   { "type": "string",  "description": "Relative path from repo root (e.g. 'internal/daemon/daemon.go')" },
      "offset": { "type": "integer", "description": "Start from this line number (1-based). Omit for beginning." },
      "limit":  { "type": "integer", "description": "Max lines to return. Omit to read whole file (capped at 500)." }
    },
    "required": ["path"]
  }
}
```

Example output:
```
   1│ package daemon
   2│
   3│ import (
   4│     "context"
   5│     "sync"
  ...
  42│     sessionExpiry = 1 * time.Hour  // BUG: should use config value
  ...
```

**`Grep`** — Search for patterns across the codebase

```json
{
  "name": "Grep",
  "description": "Search for a regex pattern across files. Returns matching lines with file paths and line numbers. Max 100 results.",
  "parameters": {
    "type": "object",
    "properties": {
      "pattern": { "type": "string", "description": "Regex pattern (e.g. 'func.*Login', 'TODO', 'session.*expir')" },
      "glob":    { "type": "string", "description": "File filter (e.g. '*.go', '*.ts'). Omit for all files." },
      "path":    { "type": "string", "description": "Subdirectory to search. Omit for entire repo." }
    },
    "required": ["pattern"]
  }
}
```

Example output:
```
auth/login.go:23:func HandleLogin(w http.ResponseWriter, r *http.Request) {
auth/login.go:58:    http.Redirect(w, r, "/dashboard", http.StatusFound)
auth/session.go:42:    sessionExpiry = 1 * time.Hour
auth/middleware.go:15:func RequireLogin(next http.Handler) http.Handler {
```

**`ListFiles`** — Find files by glob pattern

```json
{
  "name": "ListFiles",
  "description": "List files matching a glob pattern. Returns paths relative to repo root. Max 200 results.",
  "parameters": {
    "type": "object",
    "properties": {
      "pattern": { "type": "string", "description": "Glob pattern (e.g. '*.go', '*.md', '*.test.ts')" },
      "path":    { "type": "string", "description": "Subdirectory to search. Omit for entire repo." }
    },
    "required": ["pattern"]
  }
}
```

Example output:
```
auth/login.go
auth/middleware.go
auth/session.go
cmd/server/main.go
models/user.go
routes.go
```

**`GitLog`** — Recent commit history

```json
{
  "name": "GitLog",
  "description": "Show recent git commits. Returns hash, author, date, and message.",
  "parameters": {
    "type": "object",
    "properties": {
      "n":    { "type": "integer", "description": "Number of commits (default 10, max 50)." },
      "path": { "type": "string",  "description": "Only commits touching this file/dir. Omit for all." }
    }
  }
}
```

Example output:
```
a1b2c3d Fix session timeout configuration
e4f5g6h Add login rate limiting
i7j8k9l Refactor auth middleware
```

**`GitDiff`** — Uncommitted or ref-based changes

```json
{
  "name": "GitDiff",
  "description": "Show git diff (stat format). Uncommitted changes or between refs.",
  "parameters": {
    "type": "object",
    "properties": {
      "ref":  { "type": "string", "description": "Diff against this ref (e.g. 'HEAD~3', 'main'). Omit for uncommitted." },
      "path": { "type": "string", "description": "Only this file/dir. Omit for all changes." }
    }
  }
}
```

**`ReadMemory`** — Access the project memory

```json
{
  "name": "ReadMemory",
  "description": "Read the project's memory.md file. Contains learned conventions, decisions, and context from previous conversations.",
  "parameters": {
    "type": "object",
    "properties": {
      "section": { "type": "string", "description": "Only return this section (e.g. 'auth', 'deployment'). Omit for full file." }
    }
  }
}
```

**`ListThreads`** — See active work across threads

```json
{
  "name": "ListThreads",
  "description": "List active threads with their current phase, branch, and files being modified. Useful for conflict detection.",
  "parameters": {
    "type": "object",
    "properties": {
      "status": { "type": "string", "description": "Filter: 'active', 'pending_review', 'all'. Default: 'active'." }
    }
  }
}
```

Example output:
```
Thread ts_001 [Phase: Claude] branch: feat/user-auth
  Files: models/user.go, auth/register.go, routes.go
Thread ts_002 [Phase: Kimi] branch: (not yet created)
  Plan touches: auth/middleware.go
```

**`GHStatus`** — GitHub PR/issue context

```json
{
  "name": "GHStatus",
  "description": "Check GitHub PR or issue status using the gh CLI. Returns title, state, checks, and recent comments.",
  "parameters": {
    "type": "object",
    "properties": {
      "type":   { "type": "string", "description": "'pr' or 'issue'" },
      "number": { "type": "integer", "description": "PR or issue number" }
    },
    "required": ["type", "number"]
  }
}
```

#### Tool Execution — Safety Guarantees

All tools run in the CodeButler Go process (not shelled out) with these
invariants:

1. **Path sandboxing**: Every `path` parameter is resolved relative to
   `repoDir` and validated. Absolute paths rejected. Symlinks resolved
   and checked. `../../etc/passwd` → error.

2. **Read-only**: No tool can write, delete, or modify anything. The PM
   observes, never acts.

3. **Output limits**: Each tool caps its output (ReadFile: 500 lines,
   Grep: 100 matches, ListFiles: 200 files, GitLog: 50 commits,
   GitDiff: 300 lines). Results over 8KB are truncated before sending
   back to the LLM to avoid blowing up context.

4. **Iteration limit**: The tool loop runs at most 15 round-trips. After
   that, one final call without tools forces a text response. In practice,
   PMs converge in 3-7 iterations.

5. **No shell execution**: Grep uses `os/exec` with fixed command
   arguments (no shell interpolation). Git commands use explicit args.
   No `sh -c` anywhere.

6. **Timeout**: The entire tool loop inherits the context timeout. If the
   PM takes too long exploring, the context cancels everything cleanly.

```go
// internal/tools/executor.go

type Executor struct {
    repoDir string
}

func NewExecutor(repoDir string) *Executor

// Execute dispatches a tool call to the appropriate handler.
// Returns content string + error flag. Never panics.
func (e *Executor) Execute(ctx context.Context, call models.ToolCall) models.ToolResult

// safePath resolves and validates a relative path stays within repoDir.
func (e *Executor) safePath(rel string) (string, error)
```

#### Tool Loop Implementation

The loop is provider-agnostic. It takes a `ChatFunc` (any function that
does one LLM completion with tools) and an `Executor` (runs tool calls).

```go
// internal/tools/loop.go

const MaxToolIterations = 15

// ChatFunc: one LLM completion call. Returns text or tool calls.
type ChatFunc func(ctx context.Context, system string, messages []LoopMessage,
    toolDefs []ToolDef) (*LoopResponse, error)

// RunLoop executes the tool-calling loop until the LLM produces text.
func RunLoop(ctx context.Context, chatFn ChatFunc, executor *Executor,
    system string, messages []models.Message, tools []models.Tool,
    maxIter int) (string, error)
```

The loop handles:
- Converting `models.Tool` → OpenAI `function` format
- Appending assistant tool_calls messages
- Executing tools and appending tool result messages
- Truncating large tool outputs (>8KB)
- Final call without tools if iterations exhausted

#### What This Replaces

The old `internal/preflight/preflight.go` concept is **removed**. There is
no separate enrichment step. The PM does its own enrichment via tools
during natural conversation. The `preflight` package does not exist.

```
OLD architecture:
    message → preflight.Enrich() → enriched prompt → Kimi Chat() → response
                  ↑ hardcoded                             ↑ passive

NEW architecture:
    message → Kimi ChatWithTools() → [Grep] → [ReadFile] → [GitLog] → response
                                       ↑ PM decides what to explore
```

### 24.2 Kimi as First Responder — The Core Flow

Kimi **always** starts every thread. This is not routing — it's the
fundamental interaction model. The user talks to Kimi first, defines
what they want, and only when they approve does Claude execute.

#### Why Kimi First, Always

```
WITHOUT Kimi (current v1):
    User: "fix the login bug"
    → Claude starts immediately ($0.50+)
    → Claude explores the codebase, asks itself questions
    → Maybe fixes the wrong thing → another $0.50 call
    → Total: $1.00+ for a vague request

WITH Kimi first (v2):
    User: "fix the login bug"
    → Kimi: "I see auth/login.go and auth/session.go. What's the
       symptom? Timeout? Wrong credentials? Session expiry?" ($0.001)
    → User: "the session expires too fast"
    → Kimi: "Found it. auth/session.go:42 sets expiry to 1h. The
       config says 24h. Looks like a hardcoded override.
       Plan: fix line 42 to use config value, add test.
       Say *yes* to start." ($0.002)
    → User: "yes"
    → Claude executes with perfect context ($0.30)
    → Total: $0.30 for a precise fix
```

#### What Kimi Does in Phase 1

1. **Understands the request**: asks clarifying questions if vague
2. **Scans the codebase**: grep, read files, check git log (cheap operations)
3. **Answers questions directly**: "what does X do?" → Kimi reads the file and explains (no Claude needed)
4. **Proposes a plan for code tasks**: lists files to change, approach, estimated complexity
5. **Generates images**: if the task involves creating visual assets (icons, logos, mockups), Kimi generates them via OpenAI gpt-image-1 — then asks what to do next (see below)
6. **Detects conflicts**: checks if other threads touch the same files
7. **Waits for approval**: user must say "yes" before Claude starts

#### Conversation Examples

**Simple question (Kimi handles entirely, Claude never runs):**
```
User: "how do we deploy?"
Kimi: "Based on README.md and the Makefile, deployment is:
  1. make build
  2. docker push
  3. kubectl apply -f k8s/
  The CI pipeline in .github/workflows/deploy.yml does this automatically on merge to main."
```

**Vague request (Kimi clarifies before proposing):**
```
User: "login is broken"
Kimi: "I see several login-related files:
  - auth/login.go (POST /login endpoint)
  - auth/session.go (session management)
  - auth/middleware.go (auth middleware)
  What's happening? Error message? Timeout? Wrong redirect?"
User: "it redirects to a 404 after login"
Kimi: "Found it. auth/login.go:58 redirects to /dashboard but routes.go
  defines it as /app/dashboard. The redirect path is wrong.
  Plan: change redirect in auth/login.go:58 from /dashboard to /app/dashboard.
  Say *yes* to fix it."
User: "yes"
→ Claude executes
```

**Complex task (Kimi plans in detail):**
```
User: "add user registration with email verification"
Kimi: "That's a multi-file feature. Here's my plan:

  📋 *Work Plan*
  1. Create `models/user.go` — User struct + DB migration
  2. Create `auth/register.go` — POST /register endpoint
  3. Create `auth/verify.go` — GET /verify?token=... endpoint
  4. Create `email/send.go` — verification email via SMTP
  5. Update `routes.go` — add new endpoints
  6. Add tests for the registration flow

  I see the project uses GORM for models and chi for routing.
  Estimated: ~5 Claude turns.
  Say *yes* to start, or tell me what to change."
User: "skip email verification for now, just basic registration"
Kimi: "Updated plan:
  1. Create `models/user.go` — User struct + DB migration
  2. Create `auth/register.go` — POST /register with bcrypt
  3. Update `routes.go` — add POST /register
  4. Add tests
  Say *yes*."
User: "dale"
→ Claude executes the refined plan
```

**Image generation — just show it (thread ends, no Claude):**
```
User: "create a logo for the app, minimalist, blue tones"
Kimi: (reads README, checks existing assets/ folder, notes the app name)
Kimi: "I'd send this prompt to the image generator:
  'Minimalist logo for CodeButler, a developer tool. Clean lines,
   blue tones (#2563EB), no text, abstract butler/code motif,
   SVG-friendly flat design'
  Want me to adjust anything?"
User: "go"
Kimi: generates via gpt-image-1
Kimi: [uploads image] "Here it is. What do you want to do?
   ✅ Done  ·  🔄 Adjust  ·  📁 Push to repo  ·  🔧 Use with Claude"
User: "looks good"
→ User: "done" → memory extraction. Kimi learns: "logo style: minimalist, blue #2563EB"
```

**Image generation — iterate then push to repo:**
```
User: "need a favicon for the web dashboard"
Kimi: (reads internal/daemon/web.go, sees HTML template, checks static/)
Kimi: "I'd generate with this prompt:
  'Favicon 32x32px, flat design, blue (#2563EB) on transparent
   background, matches existing icon style in static/icons/'
  Adjust?"
User: "looks good"
Kimi: generates via gpt-image-1
Kimi: [uploads image] "What do you think?"
User: "make it rounder"
Kimi: "Updated prompt: '...rounded corners, circular shape...' Generating."
Kimi: [uploads new image] "Better?"
User: "yes, push it"
Kimi: creates branch, adds favicon to static/, commits, opens PR
→ PR created → memory extraction → Kimi learns: "favicon is round, blue"
→ User: "dale merge" → summary + merge + cleanup
```

**Image generation — then hand off to Claude for implementation:**
```
User: "create an error illustration for the 404 page"
Kimi: (reads routes, finds 404 handler, checks templates)
Kimi: "I'd generate: 'Friendly 404 error illustration, cartoon style,
  confused robot, matches the blue/white theme in templates/base.html'
  Want to adjust the prompt?"
User: "make it a lost astronaut instead of a robot"
Kimi: "Updated: '...lost astronaut floating in space, friendly...'"
User: "dale"
Kimi: generates via gpt-image-1
Kimi: [uploads image] "Here it is. What next?"
User: "use it in the 404 page, replace the current text-only error"
Kimi: "Plan:
  1. Save illustration to static/images/404.png
  2. Update templates/error.html to show the image
  3. Update the CSS for centered layout
  Say *yes* to start Claude."
User: "yes"
→ Phase 2: Claude gets the plan + the generated image in worktree
```

**Image with user reference — Kimi asks for inspiration:**
```
User: "need icons for the settings page"
Kimi: (checks static/icons/, sees existing icon style)
Kimi: "I see existing icons in static/icons/ use a flat, 24px style.
  Do you have a reference image for the style you want? Or should I
  match the existing set?"
User: [sends screenshot of Material Design icons]
Kimi: "Got it. I'd generate: 'Settings gear icon, Material Design
  style, 24x24px, matching the attached reference, outlined, 2px
  stroke, #333 on transparent'
  Adjust?"
User: "perfect"
Kimi: generates using edit API with reference
Kimi: [uploads result] "What next?"
```

**Image editing — user sends a photo directly:**
```
User: [attaches screenshot] "remove the watermark and use this as the hero image"
Kimi: "I'd edit with this prompt: 'Remove watermark, clean up
  artifacts, maintain original colors and composition'
  Adjust?"
User: "also make it wider, 1920x1080"
Kimi: "Updated: '...remove watermark, extend to 1920x1080,
  fill edges naturally...' Generating."
Kimi: [uploads edited image] "What next?"
User: "push it and have Claude add it to the landing page"
Kimi: "Plan:
  1. Save to static/images/hero.png (branch + PR)
  2. Claude updates templates/landing.html to use the new hero
  Say *yes*."
User: "dale"
→ Kimi pushes image first, then Claude implements the template change
```

#### Kimi's System Prompt

```
You are an AI development assistant working in a code repository.
Your role is to UNDERSTAND, DEFINE, and PLAN — never to write code.

You have tools to explore the codebase autonomously:

EXPLORATION TOOLS (read-only, use freely):
- ReadFile(path, offset?, limit?) — read file contents with line numbers
- Grep(pattern, glob?, path?) — search for regex patterns across files
- ListFiles(pattern, path?) — find files by glob pattern
- GitLog(n?, path?) — see recent commits
- GitDiff(ref?, path?) — see uncommitted or ref-based changes
- ReadMemory(section?) — read project memory (conventions, decisions)
- ListThreads(status?) — see active threads and their files (conflict detection)
- GHStatus(type, number) — check GitHub PR/issue status

ACTION TOOLS (use when the user requests):
- GenerateImage(prompt, size?) — create image via gpt-image-1
- EditImage(prompt, image, size?) — edit an existing image
- UploadImage(path) — send image to Slack
- CreateBranch(name) — create git branch
- CommitAndPush(files, message) — commit files and push
- OpenPR(title, body) — open a pull request

RULES:
- Use exploration tools proactively. Don't guess — look.
- When a user asks about code, READ the actual files before answering.
- When planning a code task, GREP and READ to find existing patterns.
- When detecting conflicts, LIST active threads and compare file lists.
- Never make claims about code you haven't read with ReadFile.
- Never propose changes to files you haven't read.

Your job is to fully define the task before handing it to Claude.
Claude should NEVER need to ask "what do you mean?" or "what should
this do?". By the time Claude starts, the task must be unambiguous.

Workflow:
1. Understand what the user wants — ask questions until it's clear
2. Explore the codebase with your tools to build context:
   - ListFiles to understand project structure
   - Grep to find relevant code
   - ReadFile to understand implementations
   - GitLog to see recent changes
   - ReadMemory for project conventions
3. If it's a question → answer it directly (you have the tools to read
   any file and search anything — no need for Claude)
4. If it involves creating images/visual assets:
   a. Scan the codebase for context (Grep for existing assets, ReadFile
      for styles, check where images are used)
   b. Generate/edit the image with a context-aware prompt
   c. Show the result in Slack
   d. Ask the user what to do next:
      - Done → user says "done" → memory extraction → thread closed
      - Iterate → adjust and re-generate
      - Push to repo → create branch, commit image, open PR
      - Continue with Claude → build a plan that uses the image
   e. The user drives — you don't assume which path
5. If it's a code task → explore with tools, then propose a plan:
   - Which files will be created/modified (cite line numbers from ReadFile)
   - What specifically changes in each file
   - What patterns to follow (reference actual code you read)
   - What edge cases to handle
   - What tests to add
6. If the request is vague, ask follow-up questions. Be specific:
   BAD:  "Can you give more details?"
   GOOD: "I see auth/login.go:58 returns a JWT (I just read it).
          Should registration also return a JWT, or redirect to /login?"
7. Present the plan and wait for approval
8. If the user adjusts, update the plan and re-present

Image generation can happen at any point during Phase 1. You might
generate an image as part of planning (e.g., "here's the icon, now
here's the plan for adding it to the app") or as the entire task.

Your plan must be detailed enough that an engineer (Claude) can
implement it without asking requirements questions. Implementation
questions ("bcrypt or argon2?") are fine — scope questions ("what
fields should User have?") mean your plan wasn't complete enough.

Repository: {repo_path}

```

#### Implementation

```go
// internal/productmanager/responder.go

// Respond handles a message in the product manager (Kimi) phase.
// Uses ChatWithTools — the PM explores the repo autonomously via tools
// (ReadFile, Grep, GitLog, etc.) before responding.
func Respond(ctx context.Context, pm models.ProductManager, repoDir string,
    history []models.Message, newMessage string) (response string, plan *Plan, err error) {

    tools := tools.PMTools(repoDir)   // read-only tools sandboxed to repoDir
    msgs := append(history, models.Message{Role: "user", Content: newMessage})
    text, err := pm.ChatWithTools(ctx, kimiSystemPrompt, msgs, tools)
    // Parse plan from response if present...
}

type Plan struct {
    Summary   string   // one-line description
    Steps     []string // what Claude (Coder) will do
    Files     []string // files that will be touched
    Images    []string // images generated during planning (local paths)
    Estimated string   // "~3 Claude turns"
}
```

Note: the `memory` parameter is gone from `Respond()` — the PM reads memory
itself via the `ReadMemory` tool when it needs it, instead of always receiving
the full file. This means the PM only loads memory when relevant, saving tokens.

### 24.3 The Approval Gate

The transition from Kimi to Claude is explicit. The user must approve.
This is not just a cost optimization — it's a **control mechanism**.
The user stays in charge of what Claude does.

#### What Counts as Approval

```go
var approvalPatterns = []string{
    "yes", "si", "sí", "dale", "go", "do it", "proceed",
    "ok", "lgtm", "ship it", "approved", "let's go",
}
```

Kimi can also detect approval in natural language:
- "yes but change X first" → Kimi adjusts plan, re-proposes
- "yes, and also do Y" → Kimi adds to plan, re-proposes
- "no" / "wait" / "actually..." → Kimi continues conversation

#### What Happens on Approval

```
User: "yes"
    ↓
1. Daemon transitions thread from PhaseKimi → PhaseClaude
2. Create worktree: git worktree add .codebutler/branches/<slug>
3. Init worktree (pod install, npm ci, etc.)
4. Build Claude prompt:
    - Sandbox prefix (section 19)
    - Kimi's approved plan
    - Relevant file contents (from Kimi's pre-flight)
    - Memory context
5. Spawn Claude in worktree: claude -p <prompt>
6. React with 👀 in thread
7. Claude works...
8. Post response in thread
9. React with ✅
```

#### The Prompt Claude Receives

```
{sandbox prefix}

TASK (well-defined, approved by user):
---
Add basic user registration without email verification.

Plan:
1. Create models/user.go — User struct with GORM tags
2. Create auth/register.go — POST /register with bcrypt
3. Update routes.go — add POST /register route
4. Add tests for registration
---

RELEVANT CONTEXT:
---
models/post.go (existing model example):
  type Post struct { ID uint; Title string; ... }

routes.go (current routes):
  r.Post("/login", auth.Login)
  r.Get("/posts", posts.List)

auth/login.go (existing auth pattern):
  func Login(w http.ResponseWriter, r *http.Request) { ... }
---

INTERACTION RULES:
- The task above has been defined and approved by the user. Do NOT
  ask questions about scope, requirements, or what to build — that's
  already decided.
- You CAN ask implementation questions if you hit a genuine ambiguity
  (e.g., "auth/login.go uses JWT but auth/session.go uses cookies —
  which pattern should I follow for registration?").
- Prefer making reasonable decisions over asking. Only ask when the
  wrong choice would be costly to undo.
- When done, commit, push, and open a PR.

{memory context}
```

Claude gets a **well-defined task** with a clear plan and relevant code.
The *what* is settled. Claude focuses on the *how*.

If images were generated during Phase 1, they're included in the prompt:

```
GENERATED ASSETS (from planning phase):
---
Image: static/images/404.png (already in worktree)
  Prompt used: "minimalist 404 error illustration, blue tones"
  User approved this design.
---
```

### 24.3b Image Generation in Phase 1

Image generation is a **Kimi capability**, not a separate phase. It happens
within Phase 1 whenever the conversation involves visual assets. Kimi uses
the OpenAI gpt-image-1 API directly — same API the v1 `/create-image` used,
but now orchestrated by Kimi with project context.

#### Why Kimi Handles Images

- Kimi already has the repo context (project name, existing assets, styles)
- Image generation is cheap (~$0.01-0.04 per image)
- No need for Claude — generating images doesn't require code execution
- Kimi can iterate with the user before deciding what to do with the result

#### How It Works

```
User message mentions images/visuals/icons/logos/etc.
    ↓
Kimi: scan repo for context
  - check existing assets (static/, images/, public/)
  - read any design tokens / CSS variables
  - note project name, branding, existing style
    ↓
Kimi: build context-aware prompt, SHOW IT to the user
  "I'd send this to the image generator:
   'Minimalist 32x32 favicon, flat design, blue (#2563EB) on
    transparent background, matching the existing icon set in
    static/icons/ which uses rounded corners and 2px stroke'
   Want me to adjust anything before generating?"
    ↓
User: "looks good" / "make it green instead" / adjusts prompt
    ↓
gpt-image-1: generate/edit with approved prompt
    ↓
Kimi: upload to Slack thread + ask user what to do next
    ↓
User chooses:
  ├─ "looks good"/"done" → memory extraction → thread closed
  ├─ "make it rounder"   → Kimi adjusts prompt, shows it, re-generates
  ├─ "push it"           → Kimi creates branch + PR with asset
  ├─ "use it in the app" → Kimi builds plan → user approves → Phase 2
  └─ (sends a photo)     → Kimi uses it as input for edit/inspiration
```

**The prompt preview is important.** Users learn how prompt engineering
works by seeing Kimi's enriched prompt. They can adjust it, and those
adjustments become memory ("user prefers green tones over blue").

#### Image Input: Reference Images

Kimi handles three cases for input images:

1. **Kimi asks** — when the task would benefit from a reference, Kimi
   proactively asks: "Do you have a reference image?"
2. **User sends unprompted** — user attaches a photo with their message.
   Kimi uses it as input for the gpt-image-1 edit API.
3. **No reference** — Kimi generates purely from the enriched text prompt.

In all cases, Kimi shows the prompt before generating.

#### Thread Outcomes After Image Generation

The key insight: image generation can **resolve the thread entirely**
or be a **stepping stone to Claude**. Kimi asks, the user decides.

| User says | What happens | Phase 2? | PR? |
|---|---|---|---|
| "looks good" / "done" | Thread ends | No | No |
| "make it X" / "try again" | Re-generate, loop | No (yet) | No |
| "push it to the repo" | Kimi: branch + commit + PR | No | Yes |
| "push it and implement" | Kimi: push asset, then plan for Claude | Yes | Yes |
| "use it in the app" | Kimi: plan includes the image | Yes | Yes |
| [sends new image] | Kimi: use as reference, re-generate | No (yet) | No |

#### Kimi Pushing Assets (Without Claude)

When the user says "push it", Kimi handles it directly:

```
1. Create branch: git checkout -b asset/<slug>
2. Copy image to target path (Kimi asks or infers from repo structure)
3. git add + commit
4. git push + gh pr create
5. Post PR link in thread
→ Memory extraction → committed to PR branch
→ User: "dale merge" → summary + merge + cleanup
```

This is lightweight — no worktree needed, no Claude, just a simple
git operation. The PR exists for review and the 1:1:1 rule (thread =
branch = PR) is maintained.

#### Implementation

```go
// Uses the Artist interface (section 24.8).
// The daemon doesn't know if it's gpt-image-1, DALL-E 3, or anything else.

// internal/daemon/phase1_images.go

// GenerateImage generates an image during the product manager phase.
// The product manager (Kimi) provides the enriched prompt.
func (d *Daemon) GenerateImage(thread *Thread, prompt string,
    reference []byte) (*models.ImageResult, error) {

    if reference != nil {
        return d.artist.Edit(ctx, models.ImageEditRequest{
            Prompt: prompt, InputImage: reference,
        })
    }
    return d.artist.Generate(ctx, models.ImageGenRequest{Prompt: prompt})
}

// PushAsset creates a branch, commits the image, and opens a PR.
// Used when the product manager resolves the thread without the coder.
func (d *Daemon) PushAsset(thread *Thread, image *models.ImageResult,
    repoPath string) (prURL string, err error)
```

#### Cost

| Operation | Cost |
|---|---|
| Image generation (gpt-image-1) | ~$0.01-0.04 |
| Image edit (gpt-image-1) | ~$0.01-0.04 |
| Kimi orchestration per message | ~$0.001 |
| Full image thread (3 iterations) | ~$0.10-0.15 |
| Image thread + Claude implementation | ~$0.40-2.10 |

#### What Claude Can and Cannot Ask

| Claude asks | OK? | Why |
|---|---|---|
| "should I use bcrypt or argon2?" | Yes | implementation choice, either works |
| "the tests use testify but this module uses stdlib, which?" | Yes | codebase ambiguity |
| "I found a race condition in the existing auth — fix it too?" | Yes | discovered issue, needs scope approval |
| "what fields should the user model have?" | **No** | requirements — Kimi should have defined this |
| "what does 'fix the login bug' mean?" | **No** | this was resolved in Phase 1 |
| "should I add email verification?" | **No** | scope was explicitly set in the plan |

If Claude hits a question that should have been resolved by Kimi, it
means Kimi's plan wasn't detailed enough. Over time, Kimi's system
prompt gets refined to produce more complete plans (auto-memory helps
with this — see section 16).

### 24.4 Post-flight: After PR Creation + Thread Close

Post-flight has two stages:

**Stage 1: After PR creation** — memory extraction begins immediately:
```
PR created (detected in Claude's response)
    ↓
    ├─ Kimi: add thread URL to PR body
    ├─ Kimi: detect if Claude left TODO/FIXME in code
    │   → warn in thread: "Claude left 2 TODOs — want a new thread to resolve them?"
    │
    └─ Kimi: analyze full thread → propose memory updates (section 16)
        → post in thread → wait for user approval
        → commit approved memory.md changes to PR branch → push
```

**Stage 2: User closes thread** — when the PR is approved and user says
"done"/"merge"/"dale", CodeButler finishes the thread:
```
User: "merge" / "done" / "dale"
    ↓
    ├─ Kimi: generate PR summary → update PR description via `gh pr edit`
    ├─ Merge PR: `gh pr merge --squash`
    ├─ Delete remote branch: `git push origin --delete <branch>`
    └─ Clean up worktree: `git worktree remove`
```

The user controls when the thread ends. The PR can stay open for review,
additional commits, or follow-up conversation. Only when the user
explicitly says to close does CodeButler merge and clean up.

**While Claude is still working** (before PR creation), the only
post-processing is practical:
```
Claude response arrives
    ↓
    └─ Kimi: summarize for Slack (if response > 4000 chars)
```

### 24.5 Thread = Branch = PR: Conflict Coordination

Each thread potentially becomes a branch and then a PR. With N concurrent
threads, you have N branches being modified simultaneously. Kimi acts as
a **merge coordinator** — detecting conflicts before they happen and
orchestrating the merge order.

#### The Problem

```
Thread A: "refactor auth module"     → branch: refactor/auth
Thread B: "fix login timeout"        → branch: fix/login-timeout
Thread C: "add 2FA to login"         → branch: feat/2fa

All three touch auth/login.go. If they all open PRs, at least two
will have merge conflicts. Without coordination, developers discover
this at PR review time — too late.
```

#### Lifecycle Tracking

The daemon tracks each thread's strict lifecycle: **1 thread = 1 branch = 1 PR**.
A thread lives until its PR is merged. That's the only exit.

```
Thread created (Slack thread)
    → Kimi classifies as code_task
    → Worktree + branch created
    → Claude starts working
    → Claude modifies files (tracked per response)
    → Claude opens PR
    → Memory extracted → committed to PR branch
    → User: "dale merge"
    → Thread CLOSED: PR merged, branch deleted, worktree removed
```

```go
// internal/conflicts/tracker.go

type ThreadState string
const (
    StateCreated  ThreadState = "created"   // Just started, classifying
    StateWorking  ThreadState = "working"   // Claude is active
    StatePR       ThreadState = "pr"        // PR opened, awaiting merge
    StateMerged   ThreadState = "merged"    // PR merged → thread CLOSED
)

type ThreadScope struct {
    ThreadTS  string
    Branch    string        // branch name (e.g., "fix/login-timeout")
    PRNumber  int           // 0 if no PR yet
    State     ThreadState
    Files     []string      // files modified by Claude in this thread
    StartedAt time.Time
}

type Tracker struct {
    mu      sync.Mutex
    threads map[string]*ThreadScope  // threadTS → scope
}
```

#### Conflict Detection Levels

Three levels of conflict, from obvious to subtle:

```
Level 1 — SAME FILE
    Thread A modifies auth/login.go
    Thread B modifies auth/login.go
    → "⚠️ Both threads modify auth/login.go"

Level 2 — SAME PACKAGE/DIRECTORY
    Thread A modifies auth/login.go
    Thread B modifies auth/session.go
    → "⚠️ Both threads modify files in auth/"

Level 3 — SEMANTIC OVERLAP (Kimi analyzes)
    Thread A modifies auth/login.go (changes bcrypt rounds)
    Thread B modifies config/security.go (adds password policy)
    → Kimi: "Both threads affect authentication behavior.
       Thread A changes password hashing, Thread B changes password rules.
       These might need coordinated testing."
```

#### When Conflicts Are Checked

```
                          ┌─ check conflicts
                          ↓
New thread starts → Kimi pre-flight:
    1. Classify message
    2. Predict which files will be touched (from message content)
    3. Check against active threads
    4. If overlap detected:
       → Warn in thread BEFORE Claude starts
       → Suggest: wait, proceed with caution, or coordinate

After each Claude response:
    1. Extract modified files from Claude's output (git diff or response text)
    2. Update thread scope
    3. Check for NEW conflicts with other active threads
    4. If new overlap detected:
       → Warn in both threads
```

#### Merge Order Suggestions

When multiple threads have open PRs touching the same files, Kimi suggests
a merge order to minimize conflicts:

```
Kimi (posted in channel, not in thread):

    📋 *Merge Order Recommendation*

    3 PRs touch overlapping files in auth/:

    1. PR #42 "fix login timeout" (Thread A)
       → Smallest change (1 file, +3/-2 lines)
       → Merge first to minimize rebase work

    2. PR #44 "add 2FA" (Thread C)
       → Medium change (3 files, +120/-15 lines)
       → Will need minor rebase after #42

    3. PR #43 "refactor auth" (Thread B)
       → Largest change (8 files, +300/-250 lines)
       → Merge last, rebase after #42 and #44
```

#### Post-Merge Notifications

When a PR merges, Kimi notifies other active threads that touch
overlapping files:

```
PR #42 (Thread A) merged
    ↓
Kimi checks: which other threads touch the same files?
    ↓
Thread B (auth/login.go overlap) →
    "ℹ️ PR #42 just merged and modified auth/login.go,
     which this thread also modifies. Consider rebasing
     your branch before continuing."

Thread C (auth/ directory overlap) →
    "ℹ️ PR #42 merged changes to auth/. Your branch
     might need a rebase."
```

#### PR Merge Detection

Two options for detecting when a PR is merged:

**Option A: GitHub webhook** (real-time, requires public endpoint or ngrok)
```go
// internal/github/webhook.go
func HandlePRMerged(event PullRequestEvent) {
    tracker.MarkMerged(event.PRNumber)
    notifyOverlappingThreads(event.PRNumber)
}
```

**Option B: Polling** (simpler, no public endpoint needed)
```go
// Poll every 60s for merged PRs
func (d *Daemon) prWatchdog() {
    for _, scope := range tracker.GetPRScopes() {
        merged, _ := gh.IsMerged(scope.PRNumber)
        if merged {
            tracker.MarkMerged(scope.ThreadTS)
            d.notifyOverlappingThreads(scope)
        }
    }
}
```

#### Conflict Detection Prompt (Kimi)

```
Given these active threads and their modified files, identify conflicts:

Active threads:
---
Thread A (1732456789.123456): branch "refactor/auth"
  Files: auth/login.go, auth/session.go, auth/middleware.go

Thread B (1732460000.654321): branch "fix/login-timeout"
  Files: auth/login.go, auth/config.go

Thread C (1732470000.111111): branch "feat/2fa"
  Files: auth/login.go, auth/totp.go, models/user.go
---

New thread message: "optimize the password hashing in auth"

Respond with JSON:
{
  "predicted_files": ["auth/login.go", "auth/hash.go"],
  "conflicts": [
    {
      "with_thread": "1732456789.123456",
      "level": "same_file",
      "files": ["auth/login.go"],
      "recommendation": "Thread A is refactoring auth/login.go extensively. Wait for Thread A to finish or coordinate changes."
    }
  ],
  "merge_order": ["B", "C", "A"],
  "merge_reason": "B is smallest, C adds new files, A is a large refactor that should go last"
}
```

#### Implementation

- **File**: `internal/conflicts/tracker.go`
- **Functions**:
  - `Track(threadTS, branch string)` — start tracking a thread
  - `UpdateFiles(threadTS string, files []string)` — update modified files
  - `SetPR(threadTS string, prNumber int)` — mark PR opened
  - `MarkMerged(threadTS string)` — mark PR merged
  - `DetectConflicts(threadTS string, predictedFiles []string) []Conflict`
  - `SuggestMergeOrder() []MergeStep`
  - `GetOverlapping(threadTS string) []ThreadScope`
- **File**: `internal/conflicts/notify.go`
  - `NotifyConflict(slackClient, threadTS string, conflict Conflict)`
  - `NotifyRebase(slackClient, threadTS string, mergedPR int)`
  - `PostMergeOrder(slackClient, channelID string, steps []MergeStep)`

### 24.6 Cost Dashboard & Thread Usage Report

Track and display cost per model, per thread, per day:

```
Thread 1732456789.123456: "fix login bug"
    Kimi:   3 calls  ·  $0.003
    Claude: 2 calls  ·  $0.84
    Total:            ·  $0.843

Thread 1732460000.654321: "create app favicon"
    Kimi:   4 calls  ·  $0.004
    Images: 2 gens   ·  $0.06
    Claude: 0 calls  ·  $0.00
    Total:            ·  $0.064

Daily: Claude $12.40 · Kimi $0.15 · Images $0.18 · Whisper $0.02 · Total $12.75
```

Exposed in the web dashboard (`/api/costs`) and optionally posted
to Slack weekly.

#### Thread Usage Report (posted at close)

When a thread closes (merge/done), the PM posts a usage summary in the
thread before cleanup. This shows exactly where tokens went and helps
the user learn to interact more efficiently with the PM.

```
📊 *Thread Summary: "fix login bug"*

*PM (Kimi)*
  Calls: 4 (3 conversation + 1 memory extraction)
  Tools used: Grep ×2, ReadFile ×3, GitLog ×1
  Tokens: 2,340 in → 1,180 out
  Cost: $0.004

*Coder (Claude)*
  Calls: 1 (5 turns)
  Tokens: 18,400 in → 6,200 out
  Cost: $0.62

*Total: $0.624*

💡 *Tips for next time:*
  • You gave a vague request ("fix the login bug") which required 2
    clarification rounds. Try: "fix the session expiry bug in auth/session.go"
  • The PM explored 6 files to find the issue. If you know the file,
    mention it — saves ~3 tool calls.
```

The tips are generated by the PM itself, analyzing the thread conversation
to identify where the user could have been more specific. This is a learning
loop: the user gets better at prompting, the PM needs fewer tool calls,
threads get cheaper.

#### What Gets Tracked (per thread)

```go
// internal/models/interfaces.go (additions to existing types)

type ThreadUsage struct {
    ThreadTS   string
    Summary    string        // one-line thread description

    PM         ModelUsage    // all PM calls in this thread
    Coder      ModelUsage    // all Coder calls
    Artist     ModelUsage    // all Artist calls (image gen)
    Whisper    ModelUsage    // voice transcription

    ToolCalls  []ToolUsage   // breakdown of PM tool calls
}

type ModelUsage struct {
    Calls        int           // number of API calls
    TokensIn     int           // prompt tokens (sum across all calls)
    TokensOut    int           // completion tokens
    Cost         float64       // estimated cost (USD)
    Duration     time.Duration // wall clock time
}

type ToolUsage struct {
    Name   string // "ReadFile", "Grep", etc.
    Count  int    // how many times called
}
```

#### How Tokens Are Tracked

The OpenAI API returns `usage.prompt_tokens` and `usage.completion_tokens`
in every response. The shared client accumulates these per-thread:

```go
// internal/provider/openai/client.go

type ChatResponse struct {
    // ... existing fields ...
    Usage *Usage `json:"usage"`
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

For Claude (Coder), the `claude -p --output-format json` response already
includes `num_turns` and cost data in the result JSON. The agent parser
extracts this.

The daemon maintains a `ThreadUsage` struct per active thread, updated
after every API call. At thread close, the PM reads this data and
generates the summary + tips.

#### The Tips: How the PM Generates Them

The PM analyzes the thread conversation to generate actionable tips.
This is a separate `Chat` call (not `ChatWithTools` — no tools needed):

```
System: "Analyze this thread conversation and generate 1-3 actionable
tips for the user to interact more efficiently next time. Focus on:
- Was the initial request vague? Suggest how to be more specific.
- Did the PM need many tool calls to find context? Suggest mentioning
  file names or areas of the codebase.
- Did the user change requirements after Claude started? Suggest
  defining scope better upfront.
- Was the task simple enough that the user could have been more direct?
Be concrete and reference the actual conversation. Keep each tip
to 1-2 sentences."

User: {full thread conversation + usage data}
```

#### Conversation Examples

**Efficient thread (short report, no tips):**
```
📊 *Thread Summary: "add CORS header to /api/ endpoints"*

*PM (Kimi)*
  Calls: 2 (1 conversation + 1 memory)
  Tools used: Grep ×1, ReadFile ×1
  Tokens: 890 in → 420 out · $0.001

*Coder (Claude)*
  Calls: 1 (3 turns)
  Tokens: 12,100 in → 4,800 out · $0.38

*Total: $0.381*

✅ Efficient thread — clear request with file context.
```

**Question-only thread (no Claude):**
```
📊 *Thread Summary: "how do we handle auth tokens?"*

*PM (Kimi)*
  Calls: 2 (1 conversation + 1 memory)
  Tools used: Grep ×1, ReadFile ×2
  Tokens: 1,200 in → 680 out · $0.002

*Coder (Claude)*
  Not used — PM answered directly.

*Total: $0.002*
```

**Expensive thread (lots of tips):**
```
📊 *Thread Summary: "refactor the whole auth module"*

*PM (Kimi)*
  Calls: 6 (5 conversation + 1 memory)
  Tools used: ListFiles ×1, Grep ×4, ReadFile ×8, GitLog ×2
  Tokens: 8,400 in → 3,200 out · $0.012

*Coder (Claude)*
  Calls: 3 (12 turns total)
  Tokens: 68,000 in → 24,000 out · $2.84

*Total: $2.852*

💡 *Tips for next time:*
  • Your initial request was broad ("refactor the whole auth module").
    The PM needed 15 tool calls to understand scope. Try breaking it
    into smaller tasks: "extract session logic from auth/login.go
    into its own file".
  • You changed requirements twice after Claude started ("actually
    also change the middleware", "wait, keep the old API"). Each
    change cost ~$0.80 in Claude turns. Finalize scope with the PM
    before approving.
  • The PM read 8 files to understand the auth module. Consider
    asking about specific files first: "what does auth/middleware.go
    do?" — the PM answers these for free.
```

### 24.7 The Full Pipeline

Every thread follows this pipeline. Kimi owns the conversation until
the user approves. Claude only appears after approval. Some threads
never leave Phase 1 — Kimi can resolve questions, generate images,
and even push assets to the repo without Claude.

```
┌──────────────────────────────────────────────────────┐
│                NEW SLACK THREAD                       │
│            User: "fix the login bug"                  │
└──────────────────────┬───────────────────────────────┘
                       ↓
    ╔══════════════════════════════════════╗
    ║        PHASE 1: KIMI (cheap)         ║
    ║     ~$0.001-0.005 per message        ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  Kimi: scan repo, understand request ║
    ║      ↓                               ║
    ║  Kimi: ask questions / propose plan  ║──→ User replies
    ║      ↓                               ║     (loop until
    ║  Kimi: refine plan                   ║←── plan is right)
    ║      ↓                               ║
    ║  (optional) generate/edit images     ║
    ║      ↓                               ║
    ║  Kimi: "Here's the plan. Yes?"       ║
    ║                                      ║
    ╚════╤═════════════════╤═══════════════╝
         │                 │
         │              ┌──┴──────────────────────────┐
         │              │  Thread resolved by Kimi:    │
         │              │  question answered, or       │
         │              │  images delivered, or        │
         │              │  assets pushed via PR         │
         │              └──┬──────────────────────────┘
         │                 ↓
         │        ┌────────────────┐
         │        │  user: "done"  │───→ Phase 3 (memory + merge + cleanup)
         │        └────────────────┘
         ↓
┌────────────────┐
│  User: "yes"   │  (approves code task)
└────────┬───────┘
         ↓
    ┌─── create worktree + branch
    ↓
    ╔══════════════════════════════════════╗
    ║       PHASE 2: CLAUDE (expensive)    ║
    ║          ~$0.30-2.00 per run         ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  Claude: execute approved plan       ║
    ║  Claude: edit files, run tests       ║
    ║  (images from Phase 1 available      ║
    ║   in worktree if generated)          ║
    ║  Claude: commit, push, open PR       ║
    ║      ↓                               ║
    ║  User replies → Claude --resume      ║
    ║                                      ║
    ╚══════════════════╤═══════════════════╝
                       ↓
              ┌────────────────┐
              │  PR created    │
              └────────┬───────┘
                       ↓
    ╔══════════════════════════════════════╗
    ║   PHASE 3: MEMORY + CLOSE (Kimi)     ║
    ║          ~$0.003                      ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  3a. Memory extraction:              ║
    ║  Kimi: analyze full thread           ║
    ║    - what Claude asked (learnings)   ║
    ║    - project decisions made          ║
    ║    - planning improvements           ║
    ║      ↓                               ║
    ║  Kimi: post proposed updates         ║
    ║    "📝 Here's what I'd remember..."  ║
    ║      ↓                               ║
    ║  User: approves / edits / adds       ║
    ║      ↓                               ║
    ║  Commit memory.md to PR branch       ║
    ║  git push                            ║
    ║                                      ║
    ║  ── PR open for review ──            ║
    ║                                      ║
    ║  3b. User closes thread:             ║
    ║  User: "merge" / "done" / "dale"     ║
    ║      ↓                               ║
    ║  Kimi: generate summary → PR desc    ║
    ║  Kimi: post thread usage report      ║
    ║  gh pr merge --squash                ║
    ║  Delete branch + worktree            ║
    ║                                      ║
    ╚══════════════════╤═══════════════════╝
                       ↓
              ┌────────────────┐
              │ Thread CLOSED  │
              │ memory on main │
              └────────────────┘
```

**Threads that skip Phase 2:**
- **Question** → Kimi answers directly. User says "done". Memory extraction → committed directly to main. ~$0.002.
- **Image only** → Kimi generates, user says "looks good". Thread done. ~$0.01.
- **Image + push** → Kimi generates, pushes to repo via PR. Memory committed to PR branch. ~$0.01.
- **Image + Claude** → Kimi generates, then transitions to Phase 2 with images ready.

### 24.8 Model Interfaces

Three interfaces, three roles, all swappable. The daemon depends on
interfaces, not concrete providers. You can swap any of them without
touching the daemon code.

```go
// internal/models/interfaces.go

// ProductManager handles conversation, planning, memory, and triage.
// Today: Kimi. Tomorrow: GPT-4o-mini, DeepSeek, Gemini, local LLM.
type ProductManager interface {
    // Chat sends a message and returns the response text.
    // Use for simple operations: memory extraction, summarization, routing.
    Chat(ctx context.Context, system string, messages []Message) (string, error)

    // ChatJSON sends a message and parses the response as JSON.
    ChatJSON(ctx context.Context, system string, messages []Message, out interface{}) error

    // ChatWithTools runs the tool-calling loop: the PM can autonomously
    // call read-only tools (ReadFile, Grep, GitLog, etc.) to explore the
    // codebase until it produces a final text response.
    // Use for: Phase 1 conversation (Kimi exploring and planning).
    // See section 24.1 for tool definitions and safety guarantees.
    ChatWithTools(ctx context.Context, system string, messages []Message, tools []Tool) (string, error)

    // Name returns the provider name for logging.
    Name() string
}

// Tool defines a function-calling tool available to the ProductManager.
// Tools are defined in section 24.1 (ReadFile, Grep, ListFiles, etc.)
type Tool struct {
    Name        string                 // e.g. "ReadFile", "Grep"
    Description string                 // shown to the LLM
    Parameters  map[string]interface{} // JSON Schema for arguments
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
    ID        string // provider-assigned call ID
    Name      string // tool name (must match a Tool.Name)
    Arguments string // raw JSON arguments
}

// ToolResult is the output of executing a tool call.
type ToolResult struct {
    CallID  string // matches ToolCall.ID
    Content string // text output (file contents, grep matches, etc.)
    IsError bool   // true if the tool call failed
}

// Artist handles image generation and editing.
// Today: OpenAI gpt-image-1. Tomorrow: DALL-E 3, Midjourney API,
// Stability AI, Flux, local Stable Diffusion, etc.
type Artist interface {
    // Generate creates a new image from a text prompt.
    Generate(ctx context.Context, req ImageGenRequest) (*ImageResult, error)

    // Edit modifies an existing image based on a prompt.
    Edit(ctx context.Context, req ImageEditRequest) (*ImageResult, error)

    // Name returns the provider name for logging.
    Name() string
}

// Coder writes code, runs tests, creates PRs.
// Today: Claude CLI (claude -p). Tomorrow: any coding agent CLI.
type Coder interface {
    // Run executes a coding task in the given working directory.
    Run(ctx context.Context, req CoderRequest) (*CoderResult, error)

    // Resume continues a previous session.
    Resume(ctx context.Context, sessionID string, message string) (*CoderResult, error)

    // Name returns the provider name for logging.
    Name() string
}
```

#### Request/Result Types

```go
// ProductManager messages
type Message struct {
    Role    string // "user", "assistant", "system"
    Content string
    Image   []byte // optional: attached image
}

// Artist types
type ImageGenRequest struct {
    Prompt string
    Size   string // "1024x1024", "512x512", etc.
}

type ImageEditRequest struct {
    Prompt     string
    InputImage []byte // the image to edit
    Size       string
}

type ImageResult struct {
    Data      []byte // PNG image data
    LocalPath string // saved to .codebutler/images/<hash>.png
}

// Coder types
type CoderRequest struct {
    Prompt     string // the enriched task prompt
    WorkDir    string // worktree path
    MaxTurns   int
    Timeout    time.Duration
    Permission string // "bypassPermissions", etc.
}

type CoderResult struct {
    Response  string
    SessionID string
    Turns     int
    Duration  time.Duration
    Cost      float64 // estimated cost
}
```

#### Shared API Client Layer

The key insight: if you use OpenAI for product manager (GPT-4o-mini), artist
(gpt-image-1), and even coder (Codex/Responses API), they all talk to the
same API with the same auth. Don't create three HTTP clients — create one.

```go
// internal/provider/openai/client.go
//
// Shared OpenAI API client. Handles HTTP, auth, rate limiting, retries.
// Used by any role that talks to OpenAI-compatible APIs.

type Client struct {
    httpClient  *http.Client
    baseURL     string  // "https://api.openai.com/v1" or compatible
    apiKey      string
    rateLimiter *rate.Limiter  // shared across all roles
}

// NewClient creates a shared client for an OpenAI-compatible provider.
func NewClient(baseURL, apiKey string) *Client {
    return &Client{
        httpClient:  &http.Client{Timeout: 120 * time.Second},
        baseURL:     baseURL,
        apiKey:      apiKey,
        rateLimiter: rate.NewLimiter(rate.Every(100*time.Millisecond), 10),
    }
}

// Low-level methods shared by all roles:

func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
func (c *Client) ChatCompletionJSON(ctx context.Context, req ChatRequest, out interface{}) error
func (c *Client) ImageGenerate(ctx context.Context, req ImageAPIRequest) (*ImageAPIResponse, error)
func (c *Client) ImageEdit(ctx context.Context, req ImageEditAPIRequest) (*ImageAPIResponse, error)

// Convenience constructors for known providers:

func NewOpenAI(apiKey string) *Client {
    return NewClient("https://api.openai.com/v1", apiKey)
}

func NewKimi(apiKey string) *Client {
    return NewClient("https://api.moonshot.cn/v1", apiKey)
}

func NewDeepSeek(apiKey string) *Client {
    return NewClient("https://api.deepseek.com/v1", apiKey)
}

func NewOllama(baseURL string) *Client {
    return NewClient(baseURL, "") // no API key for local
}
```

#### Role Adapters (Thin Wrappers)

Each role adapter is a thin wrapper around the shared client. It
implements the role interface and adds role-specific logic (prompt
formatting, response parsing) but **zero HTTP/auth code**.

```go
// internal/provider/openai/product_manager.go
//
// Adapts the shared Client to the ProductManager interface.
// ~30 lines of code — just translates between types.

type ProductManagerAdapter struct {
    client  *Client   // shared HTTP client
    model   string    // "gpt-4o-mini", "moonshot-v1-8k", etc.
    repoDir string    // repo root for tool execution
}

func NewProductManager(client *Client, model, repoDir string) models.ProductManager {
    return &ProductManagerAdapter{client: client, model: model, repoDir: repoDir}
}

func (a *ProductManagerAdapter) Chat(ctx context.Context, system string,
    messages []models.Message) (string, error) {
    // Convert models.Message → ChatRequest (with a.model)
    // Call a.client.ChatCompletion()
    // Return response text
}

func (a *ProductManagerAdapter) ChatJSON(ctx context.Context, system string,
    messages []models.Message, out interface{}) error {
    // Same, but parse JSON response
    return a.client.ChatCompletionJSON(ctx, req, out)
}

func (a *ProductManagerAdapter) ChatWithTools(ctx context.Context, system string,
    messages []models.Message, tools []models.Tool) (string, error) {
    // Create executor sandboxed to a.repoDir
    // Run tools.RunLoop() with a.client as ChatFunc
    // See section 24.1 for the loop implementation
    executor := tools.NewExecutor(a.repoDir)
    return tools.RunLoop(ctx, a.chatFn, executor, system, messages, tools, 15)
}

func (a *ProductManagerAdapter) Name() string { return "openai:" + a.model }
```

```go
// internal/provider/openai/artist.go
//
// Adapts the shared Client to the Artist interface.

type ArtistAdapter struct {
    client *Client
    model  string  // "gpt-image-1", "dall-e-3"
}

func NewArtist(client *Client, model string) models.Artist {
    return &ArtistAdapter{client: client, model: model}
}

func (a *ArtistAdapter) Generate(ctx context.Context,
    req models.ImageGenRequest) (*models.ImageResult, error) {
    // Convert → ImageAPIRequest, call a.client.ImageGenerate()
}

func (a *ArtistAdapter) Edit(ctx context.Context,
    req models.ImageEditRequest) (*models.ImageResult, error) {
    // Convert → ImageEditAPIRequest, call a.client.ImageEdit()
}

func (a *ArtistAdapter) Name() string { return "openai:" + a.model }
```

```go
// internal/provider/openai/coder.go
//
// Adapts the shared Client for an OpenAI-based coding agent.
// (Future: Codex, Responses API with code execution, etc.)

type CoderAdapter struct {
    client *Client
    model  string
}

func NewCoder(client *Client, model string) models.Coder {
    return &CoderAdapter{client: client, model: model}
}
// ...
```

```go
// internal/provider/claude/coder.go
//
// Claude CLI coder — not OpenAI-based, uses exec.Command.
// Doesn't share the OpenAI client at all.

type CLICoder struct {
    permissionMode string
    maxTurns       int
    timeout        time.Duration
}

func NewCLICoder(cfg CoderConfig) models.Coder {
    return &CLICoder{...}
}
```

#### The Point: One API Key, One Connection, Three Roles

When you use OpenAI for everything, the wiring looks like this:

```go
// internal/daemon/daemon.go — initialization

func NewDaemon(cfg Config) *Daemon {
    // If productManager and artist both use OpenAI,
    // they SHARE the same underlying client.
    openaiClient := openai.NewOpenAI(cfg.OpenAI.APIKey)

    return &Daemon{
        productManager: openai.NewProductManager(openaiClient, "gpt-4o-mini", cfg.RepoDir),
        artist:       openai.NewArtist(openaiClient, "gpt-image-1"),
        coder:        claude.NewCLICoder(cfg.Coder),  // different provider
        // ...
    }
}
```

If you use Kimi as product manager + OpenAI for images:

```go
    kimiClient   := openai.NewKimi(cfg.Kimi.APIKey)
    openaiClient := openai.NewOpenAI(cfg.OpenAI.APIKey)

    return &Daemon{
        productManager: openai.NewProductManager(kimiClient, "moonshot-v1-8k", cfg.RepoDir),
        artist:       openai.NewArtist(openaiClient, "gpt-image-1"),
        coder:        claude.NewCLICoder(cfg.Coder),
    }
```

If you use OpenAI for all three:

```go
    client := openai.NewOpenAI(cfg.OpenAI.APIKey)  // ONE client

    return &Daemon{
        productManager: openai.NewProductManager(client, "gpt-4o-mini", cfg.RepoDir),
        artist:       openai.NewArtist(client, "gpt-image-1"),
        coder:        openai.NewCoder(client, "codex-mini"),  // future
    }
```

**One `*http.Client`, one API key, one rate limiter. Three roles.**

#### Non-OpenAI Providers

For providers that aren't OpenAI-compatible, create a separate package
with its own client:

```
internal/provider/
  openai/          ← shared client + 3 adapters
    client.go      ← HTTP, auth, rate limiting (THE shared code)
    product_manager.go ← thin adapter → models.ProductManager
    artist.go      ← thin adapter → models.Artist
    coder.go       ← thin adapter → models.Coder (future)
  claude/          ← exec-based, no HTTP client
    coder.go       ← CLI wrapper → models.Coder
  stability/       ← different API, own client
    client.go      ← Stability AI HTTP client
    artist.go      ← adapter → models.Artist
  ollama/          ← local, own client
    client.go      ← local HTTP client
    product_manager.go ← adapter → models.ProductManager
```

Each provider package owns its client. The `openai` package is the most
reusable because many providers (Kimi, DeepSeek, Groq, Together AI)
are OpenAI-compatible and share the same client code.

#### How the Daemon Uses Them

```go
// The daemon never knows which provider is behind each role.
// It only talks to interfaces.

func (d *Daemon) runKimi(thread *Thread, msg Message) {
    // Phase 1: PM explores the repo autonomously via tools
    tools := tools.PMTools(d.repoDir)
    resp, _ := d.productManager.ChatWithTools(ctx, systemPrompt, thread.KimiHistory, tools)
    // ...
}

func (d *Daemon) generateImage(thread *Thread, prompt string) {
    result, _ := d.artist.Generate(ctx, models.ImageGenRequest{Prompt: prompt})
    // ...
}

func (d *Daemon) startClaude(thread *Thread) {
    result, _ := d.coder.Run(ctx, models.CoderRequest{Prompt: plan, WorkDir: worktree})
    // ...
}
```

### 24.9 Config

```json
// ~/.codebutler/config.json (global)
{
  "productManager": {
    "provider": "kimi",
    "apiKey": "...",
    "model": "moonshot-v1-8k",
    "conflictDetection": true
  },
  "artist": {
    "provider": "openai",
    "model": "gpt-image-1",
    "apiKey": "..."
  },
  "coder": {
    "provider": "claude-cli",
    "maxTurns": 10,
    "timeout": 30,
    "permissionMode": "bypassPermissions"
  }
}
```

When two roles share the same provider and API key, the daemon detects
this and creates a single shared client automatically:

```json
// All OpenAI — one API key, one shared client
{
  "productManager": { "provider": "openai", "model": "gpt-4o-mini", "apiKey": "sk-..." },
  "artist":       { "provider": "openai", "model": "gpt-image-1", "apiKey": "sk-..." },
  "coder":        { "provider": "openai", "model": "codex-mini",  "apiKey": "sk-..." }
}
```

```go
// Provider factory detects shared keys and reuses clients
func BuildProviders(cfg Config) (models.ProductManager, models.Artist, models.Coder) {
    clients := map[string]*openai.Client{} // key = baseURL+apiKey

    getOrCreate := func(baseURL, apiKey string) *openai.Client {
        key := baseURL + "|" + apiKey
        if c, ok := clients[key]; ok {
            return c  // reuse existing client
        }
        c := openai.NewClient(baseURL, apiKey)
        clients[key] = c
        return c
    }
    // ...
}
```

The product manager is **required** in v2. Kimi-first is the core interaction
model, not an optional optimization. Without it, messages would go directly
to Claude — which defeats the architecture.

The artist is **optional**. If not configured, image generation is disabled
and Kimi tells the user: "Image generation is not configured."

The coder defaults to Claude CLI. It's the only battle-tested option today,
but the interface exists so it can be swapped if needed.

If the product manager API is down, the circuit breaker (section 25) kicks in
and routes directly to Claude as a temporary fallback.

### 24.10 What This Means for CodeButler's Identity

CodeButler remains **Claude Code with extras**. The core loop is unchanged:

```
message → claude -p → response
```

The orchestration layer is invisible to the user. They still talk to
"the bot" in Slack. They don't know (or care) that Kimi triaged their
message, enriched the prompt, and extracted memory afterward.

The coder is the only model that touches code. The product manager runs
**read-only tools** (ReadFile, Grep, GitLog — see section 24.1) to explore
the codebase, but never writes, edits, or deletes anything. The artist
generates images but never modifies code. Only the coder writes code.

---

## 25. Error Recovery & Resilience

The daemon runs 24/7. Things will fail: Slack disconnects, Claude hangs,
Kimi times out, the machine reboots. Every failure mode needs a recovery
path that doesn't lose user messages.

### Failure Modes & Recovery

| Failure | Detection | Recovery | User Impact |
|---|---|---|---|
| Slack disconnect | Socket Mode auto-reconnect + state callback | Auto-reconnect (built into slack-go SDK) | Brief pause, messages queued by Slack |
| Claude process hangs | `context.WithTimeout` (from config `timeout` min) | Kill process, reply in thread: "timed out, try again" | One thread affected |
| Claude process crashes | Non-zero exit code from `exec.Command` | Reply in thread with error, session preserved for retry | User can say "try again" |
| Kimi/product manager unreachable | HTTP timeout (10s) | Try fallback model → if also fails, send directly to Claude (v1 behavior) | Slightly more expensive, but works |
| SQLite locked | Busy timeout on connection (`_busy_timeout=5000`) | Retry with backoff, max 3 attempts | Brief delay |
| Out of disk | Write failure on store.db | Log error, continue processing in-memory | New messages not persisted until resolved |
| Machine reboot | systemd/launchd restarts daemon | Daemon starts, reconnects Slack, pending messages reprocessed | Brief downtime |

### Message Durability

Messages are persisted to SQLite **before** processing. If the daemon
crashes mid-Claude-call, the message is already in the DB. On restart,
unacked messages are reprocessed:

```go
// On startup
pending, _ := store.GetPending()
for _, msg := range pending {
    go d.processThread(msg.ThreadTS, msg)
}
```

### Claude Session Recovery

If Claude crashes mid-thread, the `session_id` is still stored. The next
message in that thread will `--resume` from where Claude left off. The
user sees: "Something went wrong. Send another message to continue."

### Graceful Shutdown

```go
func (d *Daemon) Run() error {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // ... start Slack, event loop ...

    <-ctx.Done()

    // 1. Stop accepting new messages
    // 2. Wait for active Claude processes (with timeout)
    // 3. Flush pending memory updates
    // 4. Close SQLite
    // 5. Disconnect Slack
    d.shutdown(30 * time.Second)
    return nil
}
```

### Model Fallback for ProductManager and Artist

Each role can have a **fallback model** configured. If the primary fails,
try the fallback before giving up. This is especially useful because PM
and Artist use cheap models — having two options costs almost nothing.

```json
{
  "productManager": {
    "provider": "openai", "model": "gpt-4o-mini", "apiKey": "sk-...",
    "fallback": { "provider": "openai", "model": "gpt-4o", "apiKey": "sk-..." }
  },
  "artist": {
    "provider": "openai", "model": "gpt-image-1", "apiKey": "sk-...",
    "fallback": { "provider": "stability", "model": "sd3", "apiKey": "sk-..." }
  }
}
```

The fallback chain:
1. **Try primary** — e.g., Kimi (cheapest)
2. **Try fallback** — e.g., GPT-4o-mini (still cheap)
3. **Skip to Claude** — if both fail, route directly to Claude (v1 behavior)

### Circuit Breaker

If the primary fails 3 times in a row, switch to fallback for 5 minutes.
If the fallback also fails, skip the product manager entirely and route
directly to Claude. This prevents cascading slowdowns:

```go
type CircuitBreaker struct {
    failures    int
    lastFailure time.Time
    threshold   int           // 3
    cooldown    time.Duration // 5 minutes
}

func (cb *CircuitBreaker) Allow() bool {
    if cb.failures < cb.threshold { return true }
    if time.Since(cb.lastFailure) > cb.cooldown {
        cb.failures = 0 // reset
        return true
    }
    return false // try fallback or skip product manager
}
```

---

## 26. Access Control & Rate Limiting

### Who Can Use the Bot?

By default, **any member of the Slack channel** can interact with
CodeButler. The channel IS the access control boundary — no separate
auth layer needed. Every participant is logged in the PR description
(see section 23) so there's always an audit trail of who said what.

Optional restrictions in config:

```json
// <repo>/.codebutler/config.json
{
  "access": {
    "allowedUsers": [],           // empty = everyone in channel
    "maxConcurrentThreads": 5,    // per channel
    "maxClaude CallsPerHour": 20, // cost protection
    "maxClaudeCallsPerUser": 10   // per user per hour
  }
}
```

### Rate Limiting Layers

```
Layer 1 — Slack rate limits (platform-enforced)
    1 message/second for chat.postMessage
    20 files/minute for files.uploadV2
    → Queue with backoff, built into slack-go SDK

Layer 2 — Claude concurrency (resource protection)
    Max N concurrent Claude processes (configurable, depends on machine)
    Each Claude CLI is a full process — CPU, memory, potentially builds
    A laptop might handle 2-3, a beefy server 5-10
    → New threads wait in queue if limit reached
    → Reply: "⏳ Queue position: 3. Processing will start shortly."

Layer 3 — Per-user rate limiting (cost protection)
    Max M Claude calls per user per hour (default 10)
    → Reply: "You've reached the hourly limit. Try again in 23 minutes."

Layer 4 — Cost ceiling (budget protection)
    Max daily spend (estimated from token counts)
    → When exceeded: "Daily budget reached. Bot will resume tomorrow."
    → Or: notify admin in DM, continue processing
```

### Why This Matters

Without limits, one team member can accidentally burn $500/day by
spamming the bot with complex tasks. The rate limiting is primarily
**cost protection**, not access control.

### Implementation

```go
// internal/ratelimit/limiter.go

type Limiter struct {
    mu              sync.Mutex
    activeThreads   int
    maxThreads      int
    userCalls       map[string][]time.Time  // userID → timestamps
    maxCallsPerUser int
    maxCallsPerHour int
    dailySpend      float64
    maxDailySpend   float64
}

func (l *Limiter) AllowThread() bool
func (l *Limiter) AllowUser(userID string) (bool, time.Duration)  // allowed, retryAfter
func (l *Limiter) AllowBudget() bool
func (l *Limiter) RecordCall(userID string, estimatedCost float64)
```

---

## 27. Thread Lifecycle & Resource Cleanup

### The Rule: 1 Thread = 1 Branch = 1 PR

This is non-negotiable. Every thread follows the same lifecycle:

```
created → working → pr_opened → merged → closed
```

There is exactly **one way** a thread ends: **the user says to close it**
("merge", "done", "dale"). CodeButler then writes the summary, commits
memory, merges the PR, and cleans up. No timeouts, no automatic close,
no "stale" state.

```
Thread "fix login bug"
    → branch: codebutler/fix-login
    → worktree: .codebutler/branches/fix-login/
    → PR #42 created → memory committed to branch
    → User: "dale merge"
    → Kimi: summary → PR description updated
    → PR #42 merged (squash)
    → thread CLOSED ✓
    → branch deleted, worktree removed, resources freed
```

### Thread States

```
created → working → pr_opened → merged (closed)
                        ↑
                     working (user asks for more changes after PR review)
```

Only 4 states. No "idle", no "stale", no "archived":

| State | Meaning | Thread accepts messages? |
|---|---|---|
| `created` | Thread just started, Kimi classifying | Yes |
| `working` | Claude is coding (or waiting for user input) | Yes |
| `pr_opened` | PR exists, awaiting review/merge | Yes (triggers new Claude run for changes) |
| `merged` | PR merged, thread is done | No — bot replies: "This thread is closed. Start a new thread." |

### What Happens After PR Creation (Memory)

```go
func (d *Daemon) onPRCreated(threadTS string, prNumber int) {
    scope := d.tracker.Get(threadTS)

    // 1. Add thread URL to PR body
    go d.addThreadLinkToPR(threadTS, prNumber)

    // 2. Extract memory (proposes in thread, waits for approval, commits to PR branch)
    go d.extractMemory(threadTS, scope.Branch) // commit memory.md + push
}
```

### What Happens When User Closes Thread

The user explicitly triggers the close ("merge", "done", "dale"):

```go
func (d *Daemon) onUserClose(threadTS string) {
    scope := d.tracker.Get(threadTS)

    // 1. Generate PR summary → update PR description
    d.updatePRDescription(threadTS, scope.PRNumber)

    // 2. Post thread usage report (tokens, costs, tips)
    //    See section 24.6 for format details.
    d.postThreadUsageReport(threadTS, scope)

    // 3. Merge the PR
    d.github.MergePR(scope.PRNumber) // gh pr merge --squash

    // 4. Notify in thread
    d.slack.SendMessage(scope.ChannelID,
        fmt.Sprintf("PR #%d merged. Thread closed.", scope.PRNumber),
        threadTS)

    // 5. Cleanup: delete remote branch + remove worktree
    d.github.DeleteBranch(scope.Branch)
    worktree.Remove(d.repoDir, scope.Branch)

    // 6. Remove from active tracking
    d.tracker.Close(threadTS)

    // 7. Notify overlapping threads to rebase
    d.notifyOverlappingThreads(scope)
}
```

### Why No Timeouts?

A thread without a PR is a thread that hasn't finished its job.
The user might come back tomorrow, next week, or in a month. The
worktree and branch cost almost nothing to keep around (disk only).
There's no reason to force-close it.

If the user wants to abandon a thread, they close the PR on GitHub
(or never open one). The daemon can detect closed-without-merge PRs
and clean up:

```go
// PR closed without merge = abandoned
func (d *Daemon) onPRClosed(threadTS string, prNumber int) {
    d.slack.SendMessage(scope.ChannelID,
        fmt.Sprintf("PR #%d closed without merge. Thread closed. "+
            "Worktree and branch preserved — reopen the PR to continue.", prNumber),
        threadTS)
    d.tracker.Close(threadTS)
    // Note: worktree NOT removed — user might reopen
}
```

### Messages After Close

If someone replies in a closed thread, the bot responds:

```
"This thread is closed (PR #42 merged). Start a new thread for new work."
```

No Claude call, no cost. Just a static message.

### Resource Cleanup

| Resource | Cleaned up when | How |
|---|---|---|
| Worktree (`.codebutler/branches/X/`) | User closes thread | `git worktree remove` |
| Local branch | User closes thread | `git branch -d` |
| Remote branch | User closes thread | `git push origin --delete` |
| Conflict tracking | User closes thread | `tracker.Close()` |
| Claude session in DB | Never (cheap, allows audit) | Stays in SQLite |
| Thread in Slack | Never (it's Slack history) | Stays visible |

### Orphan Cleanup

For threads where a PR was never opened (user started a task but
abandoned it), a CLI command can clean up:

```
codebutler --cleanup-orphans
```

Lists worktrees with no PR and no activity in 30+ days.
User confirms before deletion. The branch is NOT deleted — only the
worktree. The user can always re-create the worktree from the branch.

### Cross-Thread References

A thread is isolated: 1 branch, 1 PR. But sometimes a new task needs
context from a previous thread or PR. The user can **link** another
thread to give Claude context without breaking the 1:1:1 rule.

#### How It Works

The user pastes a Slack thread link or PR URL in their message:

```
User (in new thread): "Add rate limiting to the auth endpoints.
  Context: https://myworkspace.slack.com/archives/C0123/p1732456789123456"

User (in new thread): "The login fix from PR #42 broke the remember-me
  feature. Fix it. See: https://github.com/org/repo/pull/42"
```

The daemon detects the reference and fetches context:

```
Slack thread link → fetch thread messages via conversations.replies
PR URL            → fetch PR description + diff via gh pr view
PR URL            → fetch PR description + diff via gh pr view
```

This context is injected into Claude's prompt as **read-only background**:

```
CONTEXT FROM RELATED THREAD (read-only, for reference):
---
Thread "fix login bug" (PR #42, merged 2026-02-10):
  - Fixed timezone comparison in auth/session.go
  - Set cookie MaxAge to 30 days for remember-me
  - Decision: kept session cookies (MaxAge=0) for non-remember-me
---

YOUR TASK (work in THIS thread's branch only):
"The login fix from PR #42 broke the remember-me feature. Fix it."
```

#### The Rule Stays

The reference is **read-only context**. Claude still works only in
this thread's branch, this thread's worktree, this thread's PR.
It cannot modify the referenced thread's branch or PR.

```
Thread A (closed): PR #42 "fix login"     ← referenced for context
Thread B (new):    PR #45 "fix remember-me" ← all work happens here
```

#### Detection

```go
// internal/references/detect.go

var (
    slackThreadPattern = regexp.MustCompile(`https://\S+\.slack\.com/archives/(\w+)/p(\d+)`)
    githubPRPattern    = regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)
)

type Reference struct {
    Type     string  // "thread" or "pr"
    ThreadTS string  // for thread references
    PRNumber int     // for PR references
    Context  string  // fetched content (messages or PR diff)
}

func ExtractReferences(message string) []Reference
func FetchContext(ref Reference, slackClient, repoDir string) (string, error)
```

#### Why This Is Better Than Resuming Old Threads

The user could reply in the old closed thread, but that would violate
the 1:1:1 rule (that PR is already merged). Instead, they start a
**new thread** and link the old one. Clean separation:

| Approach | Result |
|---|---|
| Reply in closed thread | Bot says "Thread closed. Start a new one." |
| New thread + link old one | New branch, new PR, with context from old thread |

---

## 28. Testing Strategy

### Unit Tests (no external services)

| Package | What to Test | How |
|---|---|---|
| `internal/slack/snippets.go` | Code block extraction, size-based routing | Markdown input → expected snippets output |
| `internal/router/router.go` | Message classification | Mock LLM client, verify routing decisions |
| `internal/tools/executor.go` | Tool execution (ReadFile, Grep, etc.) | Create temp repo, verify sandboxing, output limits |
| `internal/tools/loop.go` | Tool-calling loop | Mock ChatFunc, verify iteration, truncation, max-iter |
| `internal/conflicts/tracker.go` | File overlap detection, merge ordering | In-memory tracker with test data |
| `internal/github/github.go` | PR detection, merge polling | Regex tests + mock `gh` output |
| `internal/ratelimit/limiter.go` | Rate limiting logic | Time-based tests with controlled clock |
| `internal/provider/openai/*` | All OpenAI adapters | HTTP test server + shared mock client |
| `internal/models/interfaces.go` | Role interfaces | Mock implementations (MockProductManager, MockArtist, MockCoder) |

### Integration Tests (require tokens)

```bash
# Set test tokens (dedicated test workspace)
export CODEBUTLER_TEST_BOT_TOKEN=xoxb-test-...
export CODEBUTLER_TEST_APP_TOKEN=xapp-test-...

go test ./internal/slack/ -integration
```

Tests:
- Connect to Slack via Socket Mode
- Send and receive messages in a test channel
- Upload a file snippet
- Add reactions

### End-to-End Test (manual, described)

```
1. Create a test Slack workspace
2. Install CodeButler app
3. Run: codebutler --setup (pick test channel)
4. Send: "what files are in this repo?"
   → Expect: Kimi answers directly (question route)
5. Send: "add a comment to main.go"
   → Expect: Kimi enriches → Claude executes → response in thread
6. Reply in thread: "also add to the other file"
   → Expect: --resume with same session
7. Send two messages in different threads simultaneously
   → Expect: both processed concurrently
```

### Mock Providers for Tests

Two levels of testing:

**1. Interface mocks** — for testing daemon logic without any API calls:

```go
// internal/models/mock.go (build tag: testing)

// MockProductManager implements models.ProductManager
type MockProductManager struct {
    Responses     map[string]string  // keyword → response (for Chat)
    ToolResponses map[string]string  // keyword → response (for ChatWithTools)
}

func (m *MockProductManager) Chat(ctx context.Context, system string, msgs []Message) (string, error) {
    for key, resp := range m.Responses {
        if strings.Contains(msgs[len(msgs)-1].Content, key) {
            return resp, nil
        }
    }
    return `{"route": "code_task"}`, nil
}

func (m *MockProductManager) ChatJSON(ctx context.Context, system string, msgs []Message, out interface{}) error {
    text, err := m.Chat(ctx, system, msgs)
    if err != nil { return err }
    return json.Unmarshal([]byte(text), out)
}

func (m *MockProductManager) ChatWithTools(ctx context.Context, system string, msgs []Message, tools []Tool) (string, error) {
    // Mock skips tool execution — returns canned response directly.
    // For tool-loop testing, use the httptest-based shared client mock instead.
    for key, resp := range m.ToolResponses {
        if strings.Contains(msgs[len(msgs)-1].Content, key) {
            return resp, nil
        }
    }
    return m.Chat(ctx, system, msgs)
}

func (m *MockProductManager) Name() string { return "mock-product-manager" }

// MockArtist implements models.Artist
type MockArtist struct{ ImageData []byte }

func (m *MockArtist) Generate(ctx context.Context, req ImageGenRequest) (*ImageResult, error) {
    return &ImageResult{Data: m.ImageData, LocalPath: "/tmp/mock.png"}, nil
}
func (m *MockArtist) Name() string { return "mock-artist" }

// MockCoder implements models.Coder
type MockCoder struct{ Response string }

func (m *MockCoder) Run(ctx context.Context, req CoderRequest) (*CoderResult, error) {
    return &CoderResult{Response: m.Response, SessionID: "mock-session"}, nil
}
func (m *MockCoder) Name() string { return "mock-coder" }
```

**2. Shared client mock** — for testing the OpenAI adapters themselves:

```go
// internal/provider/openai/client_test.go

// httptest.NewServer that mimics OpenAI API responses.
// One test server, test all three adapters against it.

func TestSharedClient(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case strings.HasSuffix(r.URL.Path, "/chat/completions"):
            // return canned chat response
        case strings.HasSuffix(r.URL.Path, "/images/generations"):
            // return canned image response
        }
    }))

    client := openai.NewClient(server.URL, "test-key")

    // Same client, test all roles:
    pm := openai.NewProductManager(client, "gpt-4o-mini")
    art  := openai.NewArtist(client, "gpt-image-1")

    resp, _ := pm.Chat(ctx, "system", []Message{{Role: "user", Content: "hello"}})
    assert.Contains(t, resp, "expected")

    img, _ := art.Generate(ctx, ImageGenRequest{Prompt: "a cat"})
    assert.NotNil(t, img.Data)
}
```

---

## 29. Migration Path: v1 → v2

### Can Both Coexist?

Yes. v1 (WhatsApp) and v2 (Slack) are completely independent:
- Different messaging backend
- Different config keys (`whatsapp` vs `slack`)
- Same SQLite schema (with renamed columns in v2)
- Same Claude agent wrapper

A repo can run v1 while another runs v2. They share nothing.

### Migration Steps for an Existing Repo

```
1. Install Slack app in workspace (section 5)
2. Run: codebutler --setup
   → Detects existing config, asks: "Migrate to Slack? (y/n)"
   → Prompts for Slack tokens
   → Picks/creates channel
   → Saves new config (preserves Claude settings)
3. Old WhatsApp config backed up to .codebutler/config.whatsapp.bak
4. Old messages and sessions remain in store.db
   → Sessions are per-thread now, old ones ignored
   → Messages retain history for reference
5. Run: codebutler
   → Starts with Slack backend
```

### What Happens to Old Sessions?

Old `sessions` rows have `chat_jid` as primary key. New rows use
`thread_ts`. Since the key format is completely different
(`...@g.us` vs `1732456789.123456`), they don't conflict. Old rows
are simply never queried — they're dead data that can be cleaned up
with a migration script or left in place (harmless).

### Rollback

Delete `.codebutler/config.json`, restore from `.codebutler/config.whatsapp.bak`.
Run `codebutler` — it will use WhatsApp again.

---

## 30. Worktree Isolation — True Parallel Execution

### The Problem We Hadn't Solved

Sections 11 and 12 describe N concurrent threads running Claude
simultaneously. But if all N Claude processes run in the **same directory**,
they'll see each other's uncommitted changes, conflict on `git checkout`,
and corrupt each other's work. Concurrency at the thread level means
nothing if the filesystem is shared.

### The Solution: One Worktree Per Thread

The daemon runs in the root repo directory. Each thread gets its own
**git worktree** inside `.codebutler/branches/<branchName>/`. Claude
runs inside that worktree — it sees only its own branch, its own changes.

```
myrepo/                              ← daemon runs here (Slack, SQLite, orchestration)
  .codebutler/
    config.json
    store.db
    branches/
      fix-login/                     ← Thread A: Claude works here
        auth/login.go  (modified)
        ...
      add-2fa/                       ← Thread B: Claude works here
        auth/totp.go   (new)
        ...
      refactor-api/                  ← Thread C: Kimi is planning, no Claude yet
        ...
  src/
  go.mod
  CLAUDE.md
```

### Why Git Worktrees?

`git worktree` creates a lightweight checkout that shares the same `.git`
directory as the main repo. No full clone, no duplicate objects.

```bash
# Create a worktree for a new thread
git worktree add .codebutler/branches/fix-login -b fix/login

# Result: .codebutler/branches/fix-login/ is a full working tree
# on branch fix/login, branched from current HEAD
# Shares .git with the root repo — fast, lightweight

# When user closes thread (after merge)
git worktree remove .codebutler/branches/fix-login
git branch -d fix/login
git push origin --delete fix/login
```

**Comparison:**

| Approach | Create time | Disk usage | Shared .git | Isolated |
|---|---|---|---|---|
| Same directory | 0 | 0 | — | No |
| `git clone` (full) | Slow | 2x repo size | No | Yes |
| `git worktree` | ~instant | Only changed files | Yes | Yes |

Worktrees are the clear winner: instant creation, minimal disk, full isolation.

### Thread Lifecycle (Updated)

```
1. User sends "fix the login bug" in Slack
       ↓
2. Kimi classifies → code_task
       ↓
3. Daemon creates worktree + branch:
       git worktree add .codebutler/branches/fix-login -b codebutler/fix-login
       (this is THE branch for this thread — one and only one, forever)
       ↓
4. Kimi pre-flight runs (can read files from worktree or main repo)
       ↓
5. Claude spawns IN the worktree directory:
       cd .codebutler/branches/fix-login && claude -p "..."
       ↓
6. Claude works: edits files, runs tests, commits, pushes, opens PR
       (this is THE PR for this thread — one and only one)
       ↓
7. User replies in thread → Claude resumes IN SAME worktree:
       cd .codebutler/branches/fix-login && claude -p --resume <id> "..."
       ↓
8. PR created → memory extraction:
       - Kimi analyzes thread → proposes learnings → user approves
       - memory.md committed to PR branch + pushed
       ↓
9. User: "dale merge" → THREAD CLOSED:
       - Kimi generates summary → updates PR description via gh pr edit
       - gh pr merge --squash
       - git push origin --delete codebutler/fix-login
       - git worktree remove .codebutler/branches/fix-login
       - Remove from conflict tracker
       - Thread no longer accepts messages
```

### Concurrency Model (Revised)

This changes the concurrency model from section 11. Now it actually works:

```
Thread A: "fix login bug"
    → worktree: .codebutler/branches/fix-login/
    → Claude running in fix-login/ (modifying auth/login.go)

Thread B: "add 2FA" (arrives 10 seconds later)
    → worktree: .codebutler/branches/add-2fa/
    → Claude running in add-2fa/ (creating auth/totp.go)

Thread C: "refactor API" (arrives 1 minute later)
    → worktree: .codebutler/branches/refactor-api/
    → Kimi planning (no Claude yet — user hasn't approved plan)

All three run simultaneously. No filesystem conflicts.
Each Claude sees only its own branch.
```

### What the Daemon Sees vs What Claude Sees

```
Daemon (root repo):
    - Manages Slack connection
    - Manages SQLite (store.db, sessions)
    - Runs orchestration (Kimi classify, enrich, plan)
    - Creates/removes worktrees
    - Tracks thread lifecycle + conflicts
    - Does NOT modify source code

Claude (inside worktree):
    - Sees a normal repo checkout on its own branch
    - Reads CLAUDE.md (from its branch — may include changes from main)
    - Edits files, runs tests, commits, pushes
    - Opens PRs via gh
    - Has no idea it's inside .codebutler/branches/
    - Has no idea other threads exist
```

### Branch Naming

The daemon generates branch names from the thread context:

```go
func branchName(threadTS, firstMessage string) string {
    // Kimi generates a short slug from the message
    // e.g., "fix the login bug" → "fix-login"
    // e.g., "add 2FA to the auth module" → "add-2fa-auth"
    slug := kimiClient.GenerateSlug(firstMessage)
    return fmt.Sprintf("codebutler/%s", slug)
}
```

Convention: `codebutler/<slug>` — makes it clear which branches
are bot-managed. Example branches:
- `codebutler/fix-login`
- `codebutler/add-2fa-auth`
- `codebutler/refactor-api-endpoints`

### Kimi Planning Phase (No Worktree Yet)

For complex tasks, Kimi plans before Claude executes (section 24.3).
During planning, Kimi reads files from the **main repo** (no worktree
needed — Kimi is read-only). The worktree is created only when Claude
is about to execute:

```
User: "add user registration"
    ↓
Kimi reads main repo → generates plan → posts in thread
    (no worktree created yet)
    ↓
User: "yes, go ahead"
    ↓
Daemon: git worktree add .codebutler/branches/add-registration -b codebutler/add-registration
    ↓
Claude executes plan inside worktree
```

This avoids creating worktrees for tasks that never get approved.

### Worktree Base Branch

Worktrees are created from the current `main` (or default branch):

```go
func (d *Daemon) createWorktree(branchName string) (string, error) {
    dir := filepath.Join(d.repoDir, ".codebutler", "branches", branchName)

    // Fetch latest main first
    exec.Command("git", "fetch", "origin", "main").Run()

    // Create worktree from origin/main
    cmd := exec.Command("git", "worktree", "add", dir, "-b", branchName, "origin/main")
    cmd.Dir = d.repoDir
    return dir, cmd.Run()
}
```

Always branching from latest `origin/main` minimizes future merge conflicts.

### Disk Usage & Limits

Git worktrees are cheap — they only store the working tree files, not
the `.git` objects. A repo with 100MB of code creates ~100MB worktrees.
With 5 concurrent threads: ~500MB.

Config limit to prevent disk abuse:

```json
{
  "access": {
    "maxConcurrentThreads": 5,
    "maxWorktreeDiskMB": 2000
  }
}
```

The daemon checks before creating a worktree:

```go
func (d *Daemon) canCreateWorktree() bool {
    size := dirSize(filepath.Join(d.repoDir, ".codebutler", "branches"))
    return size < d.config.Access.MaxWorktreeDiskMB * 1024 * 1024
}
```

### What Updates in Previous Sections

This section changes assumptions in several earlier sections:

| Section | Change |
|---|---|
| **11. Message Flow** | `agent.Run()` now receives worktree path as working directory |
| **12. Threads = Sessions** | Each thread now also = one worktree |
| **19. Claude Sandboxing** | `You are working in: {worktree_path}` (not root repo) |
| **24.1 Pre-flight** | Kimi reads from main repo or worktree (depending on phase) |
| **24.5 Conflicts** | File overlap detection still applies — conflicts happen at merge, not at filesystem level |
| **27. Cleanup** | `git worktree remove` added to cleanup cycle |

### Gitignore

Add to `.gitignore`:

```
.codebutler/
```

The entire `.codebutler/` directory (config, store, branches, images)
is gitignored. Worktrees inside it are never committed to the main repo.

### Implementation

```go
// internal/worktree/worktree.go

// Create creates a new git worktree for a thread
func Create(repoDir, branchName, baseBranch string) (worktreeDir string, err error)

// Remove deletes a worktree and its local branch
func Remove(repoDir, branchName string) error

// List returns all active worktrees
func List(repoDir string) ([]WorktreeInfo, error)

// Exists checks if a worktree already exists for a branch
func Exists(repoDir, branchName string) bool

type WorktreeInfo struct {
    Branch    string
    Directory string
    HEAD      string    // current commit
    CreatedAt time.Time
}
```

### The Full Picture

```
myrepo/                              ← daemon: Slack + SQLite + orchestration
  .codebutler/
    config.json                      ← per-repo config
    store.db                         ← messages + sessions
    branches/
      fix-login/                     ← Thread A worktree (Claude active)
      add-2fa/                       ← Thread B worktree (Claude active)
      refactor-api/                  ← Thread C worktree (Kimi planning)
    images/                          ← generated images
  src/                               ← main repo source (daemon reads, never modifies)
  CLAUDE.md                          ← shared knowledge
  .gitignore                          ← includes .codebutler/
  go.mod
```

Each thread is fully isolated: its own Slack thread, its own Claude session,
its own git branch, its own filesystem. The only shared state is SQLite
(thread-safe) and the Slack connection (multiplexed). True parallel execution.

---

## 31. Worktree Initialization — Build Environments Per Branch

### The Problem

A git worktree gives you isolated source files. But most projects need
more than source files to build and test: dependency caches, build
artifacts, environment configs. If two worktrees share these, they collide.

**iOS/Xcode is the worst case:**

```
Worktree A: xcodebuild test → writes to ~/Library/Developer/Xcode/DerivedData/MyApp-abc123/
Worktree B: xcodebuild test → writes to ~/Library/Developer/Xcode/DerivedData/MyApp-abc123/
                                                                              ↑ COLLISION
```

Both worktrees build the same scheme. Xcode hashes the project path
to generate the DerivedData folder name. If both worktrees have similar
project paths, they overwrite each other's build artifacts mid-compilation.

### The Solution: Per-Worktree Build Isolation

Each worktree gets its own build artifacts directory. The daemon configures
this **before** spawning Claude via environment variables and build flags.

#### iOS/Xcode

```bash
# Each worktree uses its own DerivedData
xcodebuild test \
    -derivedDataPath .derivedData \
    -scheme MyApp \
    -destination 'platform=iOS Simulator,name=iPhone 16'
```

With `-derivedDataPath .derivedData`, Xcode writes build artifacts
inside the worktree itself (`.codebutler/branches/fix-login/.derivedData/`).
No collisions. Each worktree is fully self-contained.

**CocoaPods**: `Pods/` is part of the working tree. If `Pods/` is committed
(common in iOS), it's already in the worktree via checkout. If `Pods/`
is gitignored, the daemon runs `pod install` after creating the worktree.

**SPM (Swift Package Manager)**: Package cache is global
(`~/Library/Developer/Xcode/`), but SPM handles concurrent resolution
safely — it's read-only once resolved. The `Package.resolved` file is
per-worktree (committed to git), so each branch can have different
dependency versions.

#### Per-Platform Init Scripts

Different project types need different initialization. The daemon
detects the project type and runs the appropriate setup:

```go
// internal/worktree/init.go

type ProjectType string

const (
    ProjectXcode   ProjectType = "xcode"     // .xcodeproj or .xcworkspace
    ProjectNode    ProjectType = "node"      // package.json
    ProjectGo      ProjectType = "go"        // go.mod
    ProjectPython  ProjectType = "python"    // requirements.txt or pyproject.toml
    ProjectRust    ProjectType = "rust"      // Cargo.toml
    ProjectGeneric ProjectType = "generic"   // no special setup
)

func DetectProject(worktreeDir string) ProjectType

func InitWorktree(worktreeDir string, projectType ProjectType) error
```

#### Init Steps Per Platform

| Platform | Detection | Init Steps | Build Isolation |
|---|---|---|---|
| **iOS/Xcode** | `.xcodeproj` or `.xcworkspace` | `pod install` (if Podfile + no Pods/) | `-derivedDataPath .derivedData` |
| **Node.js** | `package.json` | `npm ci` or `yarn install --frozen-lockfile` | `node_modules/` is per-worktree |
| **Go** | `go.mod` | Nothing — module cache is global + safe | `GOBIN` per worktree (optional) |
| **Python** | `requirements.txt` / `pyproject.toml` | `python -m venv .venv && pip install -r requirements.txt` | `.venv/` per worktree |
| **Rust** | `Cargo.toml` | Nothing — global cache safe | `CARGO_TARGET_DIR=.target` |
| **Generic** | None of the above | Nothing | Nothing |

#### Sandboxing: Injected Build Flags

The daemon injects build isolation into Claude's environment. Claude
doesn't need to know about this — the flags are set in the process
environment before `claude -p` spawns:

```go
func (d *Daemon) spawnClaude(worktreeDir string, prompt string, sessionID string) {
    cmd := exec.Command("claude", "-p", prompt, "--output-format", "json")
    cmd.Dir = worktreeDir

    // Inject per-worktree build isolation
    env := os.Environ()
    switch detectProject(worktreeDir) {
    case ProjectXcode:
        // Tell Claude's sandbox prefix to use local DerivedData
        env = append(env, "XCODEBUILD_DERIVED_DATA=.derivedData")
    case ProjectRust:
        env = append(env, "CARGO_TARGET_DIR=.target")
    }
    cmd.Env = env

    // ...
}
```

For Xcode specifically, the sandbox prompt (section 19) gets an extra rule:

```
- When running xcodebuild, ALWAYS use: -derivedDataPath .derivedData
  This keeps build artifacts inside this directory.
```

This is prompt-level enforcement. Claude reads it and obeys.

### Xcode Simulators: Shared but Safe

iOS simulators are system-level (`~/Library/Developer/CoreSimulator/`).
Multiple `xcodebuild test` runs can use the same simulator concurrently
in Xcode 15+ (parallel testing). But to be safe:

**Option A: Same simulator, sequential tests**
```json
{
  "build": {
    "maxConcurrentBuilds": 1
  }
}
```

Only one worktree runs `xcodebuild test` at a time. Others queue.
Simple, safe, but slower.

**Option B: Different simulators per worktree**
```bash
# Worktree A
xcodebuild test -destination 'platform=iOS Simulator,id=AAAA-BBBB'

# Worktree B
xcodebuild test -destination 'platform=iOS Simulator,id=CCCC-DDDD'
```

The daemon pre-creates cloned simulators for each worktree:

```bash
xcrun simctl clone <base-device-id> "CodeButler-fix-login"
```

Cloned simulators are cheap (~50MB) and fully isolated.

**Option C: Let Xcode handle it**

Xcode 15+ supports concurrent test runs on the same simulator via
`-parallel-testing-enabled YES`. The test runner handles scheduling.
This works for unit tests but can be unreliable for UI tests.

**Recommendation**: Start with Option A (sequential builds, `maxConcurrentBuilds: 1`).
It's the simplest and avoids all simulator/DerivedData issues. Optimize later
if build concurrency becomes a bottleneck.

### Resource Awareness

iOS builds are heavy: ~2GB RAM, high CPU for minutes. The daemon should
know that "this project is Xcode" and limit concurrency accordingly:

```go
type ResourceProfile struct {
    MaxConcurrentClaude int  // how many Claude processes can run
    MaxConcurrentBuilds int  // how many builds can run (subset of Claude)
    EstimatedRAMPerBuild int // MB, for queue decisions
}

var profiles = map[ProjectType]ResourceProfile{
    ProjectXcode:   {MaxConcurrentClaude: 3, MaxConcurrentBuilds: 1, EstimatedRAMPerBuild: 2048},
    ProjectNode:    {MaxConcurrentClaude: 5, MaxConcurrentBuilds: 3, EstimatedRAMPerBuild: 512},
    ProjectGo:      {MaxConcurrentClaude: 5, MaxConcurrentBuilds: 5, EstimatedRAMPerBuild: 256},
    ProjectPython:  {MaxConcurrentClaude: 5, MaxConcurrentBuilds: 5, EstimatedRAMPerBuild: 128},
    ProjectRust:    {MaxConcurrentClaude: 3, MaxConcurrentBuilds: 2, EstimatedRAMPerBuild: 1024},
    ProjectGeneric: {MaxConcurrentClaude: 5, MaxConcurrentBuilds: 5, EstimatedRAMPerBuild: 128},
}
```

**Key distinction**: `MaxConcurrentClaude` limits how many threads run
Claude at all (editing code, reading files — lightweight). `MaxConcurrentBuilds`
limits how many are actually compiling/testing (heavyweight). Claude can
edit code in 3 worktrees simultaneously, but only 1 can run `xcodebuild test`
at a time.

### Init Time Budget

Worktree init can be slow (`pod install` = 30s, `npm ci` = 20s, Xcode build =
minutes). The daemon should:

1. **Init in background** while Kimi does pre-flight enrichment
2. **Report progress** in the Slack thread: "Setting up environment..."
3. **Cache aggressively**: if Podfile.lock hasn't changed since the last
   worktree, symlink or copy the existing `Pods/` directory

```go
func (d *Daemon) prepareWorktree(threadTS, branchName string) (string, error) {
    // 1. Create worktree
    dir, err := worktree.Create(d.repoDir, branchName, "origin/main")
    if err != nil { return "", err }

    // 2. Detect project type
    pt := worktree.DetectProject(dir)

    // 3. Init (can be slow)
    d.slack.SendMessage(d.channelID, "Setting up build environment...", threadTS)
    if err := worktree.InitWorktree(dir, pt); err != nil {
        return "", fmt.Errorf("worktree init failed: %w", err)
    }

    return dir, nil
}
```

### Config

```json
// <repo>/.codebutler/config.json
{
  "build": {
    "projectType": "auto",          // auto-detect, or force: "xcode", "node", etc.
    "maxConcurrentBuilds": 1,       // for Xcode projects
    "derivedDataInWorktree": true,  // -derivedDataPath .derivedData
    "initCommand": "",              // custom: "make setup" (overrides auto-detect)
    "preBuildCommand": ""           // runs before each Claude spawn: "bundle exec pod install"
  }
}
```

`initCommand` lets advanced users define custom setup for exotic projects.
`preBuildCommand` runs before each Claude invocation (useful for projects
where deps change frequently between branches).

### What This Means for Section 30

Section 30 described worktrees as "instant creation". With init scripts,
creation is instant but **readiness** depends on the project:

| Project | Worktree ready in |
|---|---|
| Go | ~1s (nothing to init) |
| Python | ~5s (venv + pip) |
| Node.js | ~15s (npm ci) |
| iOS (Pods committed) | ~1s (already in checkout) |
| iOS (Pods gitignored) | ~30s (pod install) |
| iOS (first build) | ~2-5min (Xcode indexing + compile) |

The daemon overlaps init with Kimi's pre-flight enrichment to hide latency.
By the time the enriched prompt is ready, the worktree is usually
initialized too.

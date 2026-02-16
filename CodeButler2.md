# CodeButler 2

**Status**: Planning (implementation not started)

---

## 1. What is CodeButler

CodeButler is **Claude Code accessible from Slack**. Multi-model, multi-role,
with persistent memory that improves over time.

The core idea: you describe what you want in a Slack thread. A cheap model
(the PM) plans the work, explores the codebase, and proposes a plan. You
approve. Claude Code executes — with its full agent loop, tool use, file
editing, test running, and PR creation. No terminal needed. You can be
on your phone.

```
You (Slack)  →  PM plans  →  you approve  →  Claude Code executes  →  PR ready
                 $0.003         "dale"              $0.30-2.00
```

### 1.1 The Two Loops

CodeButler has two agent loops, one inside the other:

**Outer loop — CodeButler orchestration:**
Decides WHAT to build and WHEN. The PM talks to the user, explores
the repo with read-only tools, proposes a plan, gets approval, spawns
Claude Code, routes Coder questions back to the PM, extracts memory,
and closes the thread.

**Inner loop — Claude Code's native agent:**
Decides HOW to build it. When `claude -p` runs, Claude Code executes
its own internal agent loop: reading files, writing code, running tests,
iterating on errors, committing, and pushing — all autonomously inside
a git worktree. CodeButler never interferes with this loop except via
`--resume` (to inject PM answers or user messages).

```
┌─── OUTER LOOP (CodeButler) ─────────────────────────────┐
│                                                          │
│  User message → PM explores → PM proposes plan → User   │
│  approves → create worktree + branch                     │
│       │                                                  │
│       ▼                                                  │
│  ┌─── INNER LOOP (Claude Code) ──────────────────────┐  │
│  │                                                    │  │
│  │  claude -p "<plan>" --output-format json            │  │
│  │       │                                            │  │
│  │       ▼                                            │  │
│  │  Read files → write code → run tests → fix errors  │  │
│  │  → commit → push → open PR                         │  │
│  │  (all autonomous, Claude Code's own agent loop)    │  │
│  │                                                    │  │
│  └────────────────────────────────────────────────────┘  │
│       │                                                  │
│       ▼                                                  │
│  PR created → memory extraction → user closes → merge    │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**maxTurns** and **timeout** (from config) control the inner loop.
CodeButler controls the outer loop.

### 1.2 The Three Roles

| Role | What it does | Model | Writes code? | Cost |
|------|-------------|-------|-------------|------|
| **PM** (Product Manager) | Plans, explores, routes, extracts memory | Kimi / GPT-4o-mini / Claude (swappable) | Never | ~$0.001/msg |
| **Artist** | Generates and edits images | OpenAI gpt-image-1 | Never | ~$0.02/img |
| **Coder** | Writes code, runs tests, creates PRs | Claude Code (`claude -p`) | Always | ~$0.30-2.00/task |

The separation of powers is absolute:
- **Only the Coder writes code.** The PM has read-only tools. The Artist generates images.
- **Only the PM orchestrates.** The Coder doesn't know about other threads, memory, or Slack.
- **Only the user approves.** No code runs without explicit "yes"/"dale"/"go".

### 1.3 What Makes It an Agent

CodeButler is an agent, not a chatbot. The difference:

**A chatbot** receives a message, calls an LLM, returns text. Stateless.

**CodeButler** receives a message and then:
1. Classifies the intent (question? code task? image request?)
2. Selects a workflow from memory (bugfix? feature? refactor?)
3. Explores the codebase autonomously with tools (ReadFile, Grep, GitLog...)
4. Detects conflicts with other active threads
5. Proposes a plan with file:line references
6. Waits for user approval
7. Creates an isolated git worktree
8. Spawns Claude Code with the plan + context + MCP servers
9. Routes Coder questions to the PM (resolved internally or escalated)
10. Detects PR creation
11. Extracts learnings → proposes memory updates → user approves
12. Merges PR, cleans up worktree, closes thread

Each step involves decisions, tool calls, and state management. The PM
makes 5-15 tool calls per thread just during planning. Claude Code makes
dozens more during execution. The memory system means every thread makes
the next one better.

### 1.4 MCP — Model Context Protocol

Claude Code supports MCP servers natively. This means the Coder can connect
to external tools — databases, APIs, documentation servers, Figma, Linear,
Jira, Sentry, custom internal tools — via MCP.

**This is what makes CodeButler fundamentally different from "a Slack bot
that calls an LLM API."** With MCP, the Coder doesn't just edit files on
disk — it interacts with the full development ecosystem.

#### MCP for the Coder (Claude Code)

Claude Code discovers MCP servers from:
1. **Repo-level**: `.claude/mcp.json` (committed, shared with team)
2. **User-level**: `~/.claude/mcp.json` (personal, not committed)

When CodeButler spawns `claude -p` inside a worktree, Claude Code
automatically loads MCP config from the worktree's `.claude/mcp.json`.
CodeButler doesn't need to do anything — Claude Code handles it natively.

Examples of what MCP enables for the Coder:
- Query a PostgreSQL database to understand the schema before writing migrations
- Read Linear/Jira tickets for requirements context
- Check Sentry for error traces when fixing bugs
- Read Figma designs when implementing UI
- Query internal documentation servers for API specs

#### MCP for the PM (future)

The PM currently has a fixed set of read-only tools (ReadFile, Grep, etc.).
In the future, the PM could also connect to MCP servers to access external
knowledge during the planning phase:

- Read a Linear ticket as the starting point for a task
- Check monitoring dashboards to understand an error
- Access documentation servers for API specs

This is not in v1 of CodeButler2 but the tool-calling loop is already
provider-agnostic and can accommodate MCP tool definitions.

#### MCP Server Lifecycle

If MCP servers need to be running (e.g., a local Postgres MCP server), the
daemon should verify they're accessible before spawning Claude. For remote
MCP servers (SaaS tools), no lifecycle management is needed — Claude Code
connects via the configured URL.

```json
// .claude/mcp.json (committed to repo)
{
  "mcpServers": {
    "postgres": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-postgres", "postgresql://..."]
    },
    "linear": {
      "command": "npx",
      "args": ["@linear/mcp-server"],
      "env": { "LINEAR_API_KEY": "${LINEAR_API_KEY}" }
    }
  }
}
```

The daemon passes environment variables to the Claude Code process so MCP
servers that need API keys can access them:

```go
cmd := exec.Command("claude", "-p", prompt, "--output-format", "json")
cmd.Dir = worktreeDir
cmd.Env = append(os.Environ(), d.mcpEnvVars()...)
```

### 1.5 Why CodeButler Exists — Differentiators

#### vs. Claude Code in the terminal

You already have Claude Code. Why wrap it in Slack?

| | Claude Code (terminal) | CodeButler |
|---|---|---|
| **Interface** | Terminal, one user | Slack — team, persistent, mobile |
| **Planning** | You decide what to do | PM plans, explores, proposes. You approve. |
| **Cost** | Every interaction is Claude | PM planning is 100x cheaper (~$0.001 vs ~$0.10) |
| **Memory** | CLAUDE.md (you write it) | Automated: PM extracts learnings after every thread |
| **Concurrency** | One session | N parallel threads with isolated worktrees |
| **Audit** | Terminal history (ephemeral) | Slack threads + PRs + memory (permanent) |
| **Conflict detection** | None | Cross-thread file overlap detection |
| **MCP** | Supported | Supported (same — it IS Claude Code) |

**The core insight**: CodeButler doesn't replace Claude Code. It makes every
Claude Code invocation more efficient. A vague task in terminal costs $1.00
and may go wrong. Through CodeButler: $0.003 for planning + $0.30 for a
well-defined, pre-researched task.

#### vs. Cursor / Windsurf / AI IDE tools

| | Cursor/Windsurf | CodeButler |
|---|---|---|
| **Interface** | IDE extension (you sit at your desk) | Slack (you can be anywhere) |
| **Interaction** | Interactive — you guide every step | Autonomous — PM plans, you approve, Coder executes |
| **Agent** | Custom model-agnostic agent | Claude Code's native agent (maintained by Anthropic) |
| **Team** | Single user | Multi-user, shared channel |
| **Memory** | Session-only | Persistent, evolving, role-specific |
| **PR workflow** | Manual | Automated: branch → PR → memory → merge → cleanup |

**The core insight**: CodeButler is fire-and-forget. Describe what you want,
approve the plan, get a PR. No IDE needed.

#### vs. Devin / OpenHands / autonomous agents

| | Devin/OpenHands | CodeButler |
|---|---|---|
| **Agent** | Custom agent, custom sandbox | Claude Code (Anthropic maintains it) |
| **Sandbox** | Cloud VM / Docker | Git worktrees (lightweight, local, no cloud) |
| **Planning** | Internal (opaque) | Visible PM phase, user approves before code runs |
| **Control** | Fire-and-forget (hope it works) | PM-mediated, inter-role dialogue, user corrections |
| **Cost transparency** | Opaque or per-task | Per-role breakdown, per-thread, cost caps |
| **Learning** | None | Memory system improves with every thread |
| **Local execution** | Cloud-based | Runs on your machine, your repos, your tools |

**The core insight**: CodeButler gives you control without requiring attention.
You see the plan before it runs. You see what the PM decided behind the scenes.
You can correct it. And the system learns from every correction.

#### vs. Simple Slack bots (LLM API wrappers)

Simple Slack bots generate text. CodeButler ships code.

A Slack bot says: "Here's how you'd fix that bug: [code block]."
CodeButler says: "I fixed it. PR #42 ready for review."

The difference is the Coder. It's not an API call that generates text —
it's Claude Code running inside a git worktree with full filesystem access,
shell execution, test running, and PR creation capabilities. Plus MCP
servers for database queries, issue trackers, monitoring, and more.

---

## 2. Concept Mapping (WhatsApp → Slack)

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

## 3. Architecture

```
Slack <-> slack-go SDK <-> Go daemon <-> spawns claude -p <-> repo context + MCP
                               |                                    ↕
                           SQLite DB                          git worktrees
                      (messages + sessions)                  (1 per thread)
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
| `internal/journal/journal.go` | Thread journal: incremental MD narrative committed to branch |
| `internal/models/interfaces.go` | ProductManager, Artist, Coder interfaces + shared types |
| `internal/provider/openai/client.go` | Shared OpenAI HTTP client (auth, rate limiting, retries) |
| `internal/provider/openai/product_manager.go` | Thin adapter: shared client → ProductManager interface |
| `internal/provider/openai/artist.go` | Thin adapter: shared client → Artist interface |
| `internal/provider/openai/coder.go` | Thin adapter: shared client → Coder interface (future) |
| `internal/provider/claude/coder.go` | Claude CLI Coder (exec.Command, not HTTP) |
| `internal/provider/claude/product_manager.go` | Claude CLI as PM (read-only mode, same binary, PM system prompt) |
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
PM/Artist/Coder response
    ↓
Prepend role prefix ("*PM:*", "*Artist:*", "*Coder:*")
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

### Bot Prefix → Role Prefix

WhatsApp needed `[BOT]` to filter own messages. Slack identifies bots
natively (`bot_id`), so we no longer need a filtering prefix.

Instead, every outgoing message gets a **role prefix** so users always
know which part of the system is talking and what phase they're in:

```
PM: "I see auth/login.go. What's the symptom?"
PM: "Plan: fix session.go:42. Say *yes*."
Coder: "Working on it..."
Coder: "Done. PR #42 opened."
Artist: [uploads image] "Here it is. Adjust?"
PM: "📝 Here's what I'd remember: ..."
PM: "📊 Thread Summary: ..."
```

The prefix is **bold** in Slack (`*PM:*`, `*Coder:*`, `*Artist:*`).

```go
// internal/daemon/message.go

type Role string

const (
    RolePM     Role = "PM"
    RoleArtist Role = "Artist"
    RoleCoder  Role = "Coder"
)

// FormatMessage prepends the role prefix to outgoing messages.
func FormatMessage(role Role, text string) string {
    return fmt.Sprintf("*%s:* %s", role, text)
}
```

**Why this matters:**
- Users learn the system fast — they see which "person" is talking
- Phase transitions are visible: "PM asked questions → now Coder is working"
- When the Artist generates an image, it's clear it's a different actor
- Memory extraction messages come from PM — users know it's organizing, not coding
- If Coder asks a question, users understand it's an implementation question, not a planning one

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
- [x] **memory.md with approval**: after PR creation, Claude (always) analyzes thread + diff → proposes memory updates in thread, user approves/edits, committed to PR branch
- [x] **PM self-improvement**: memory extraction analyzes what Coder asked → adds "Planning Notes" so PM handles it next time
- [x] **memory.md via git flow**: memory.md committed to repo, follows PR flow, merges to main with the PR
- [x] **PR as journal**: thread summary goes in PR description (via `gh pr edit`)
- [x] **Thread journal**: detailed narrative MD file committed to branch (`.codebutler/journals/thread-<ts>.md`), lands on main with merge
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

### Files — One Memory Per Role

Each role has its own memory file. They learn different things and have
different personalities. The Coder already has `CLAUDE.md` — we don't
create a separate memory for it.

```
<repo>/memory-pm.md       # ProductManager: workflows, project knowledge, planning notes
<repo>/memory-artist.md   # Artist: style preferences, color palettes, icon conventions
<repo>/CLAUDE.md          # Coder: coding conventions, architecture, test patterns (already exists)
```

**Why separate files:**
- The PM learns workflows, clarification patterns, and project structure.
  It doesn't care about art styles or coding conventions.
- The Artist learns color palettes, icon sizes, visual motifs, preferred
  prompts. It doesn't care about JWT vs cookies.
- The Coder (Claude) uses `CLAUDE.md`, which already exists in most repos.
  CodeButler doesn't manage `CLAUDE.md` — the user and Claude do.

**What each role sees:**
- PM system prompt gets: `memory-pm.md` (workflows + knowledge + planning)
- Artist system prompt gets: `memory-artist.md` (visual style + preferences)
- Coder prompt gets: `CLAUDE.md` (whatever the repo already has)

**Memory extraction is role-aware.** After a thread, the PM analyzes the
conversation and proposes updates to the right file:
- Planning/project learnings → `memory-pm.md`
- Visual/style learnings → `memory-artist.md`
- Coding convention learnings → suggested as a note to the user (they
  decide whether to update `CLAUDE.md` themselves)

All memory files are committed to the repo — not gitignored. They live
at the repo root alongside `CLAUDE.md`.

### The Git Flow

Memory files follow the same PR flow as code:

1. **After PR creation**: PM extracts learnings → proposes in thread → user approves
2. **PM commits** the approved changes to the right memory file(s) on the PR branch and pushes
3. **Memory updates are part of the PR** — visible in the diff, reviewable
4. **On merge**: memory lands on main → available to all future threads

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

Memory extraction **always uses Claude in the PM role** regardless of
the thread's active PM model. Claude uses the same PM interface — same
read-only tools, same memory file routing (memory-pm.md, memory-artist.md),
same JSON output format. The only difference is a smarter brain doing
the analysis.

This is a deliberate design choice:

- Claude is the best model for identifying subtle patterns and learnings
- The diff is complex code — Claude understands it better than any other model
- The cost is one extra Claude call (~$0.02-0.05) — worth it for quality learnings
- This is the most valuable output of the entire thread: the learnings compound
- Claude acts as PM here, NOT as Coder — it cannot edit files, only analyze

When triggered, the daemon:

1. Reads current `memory.md` from the PR branch
2. Reads the full thread conversation (whatever phases occurred)
3. **Gets the git diff** (`git diff main...<branch>`) to see exactly what changed
4. **Calls Claude** (not the active PM) to analyze conversation + diff → propose memory updates
5. **Posts the proposed changes in the Slack thread** for user review
6. User approves, edits, or adds more learnings
7. Daemon commits approved changes to `memory.md` on the PR branch and pushes

Why the diff matters: the conversation shows what was *discussed*, but the
diff shows what was *actually done*. Claude can spot:
- Patterns the Coder established that should become conventions
- Decisions that were made implicitly (not discussed) but visible in code
- Files/packages that were created — future context for the PM
- Test patterns, error handling choices, naming conventions
- Dependencies added (`go.mod` changes) worth remembering

### The Feedback Loop

Claude doesn't just extract general learnings — it analyzes the full
conversation AND the diff to find patterns worth remembering. It
specifically looks at:

1. **What the Coder asked** — if the PM could have pre-resolved it in
   Phase 1, that becomes a learning for the PM.
2. **What the diff shows** — patterns, conventions, and decisions that
   were made in code but never discussed explicitly.

```
Thread conversation:
  [PM] Plan: 1. Create user model  2. Add POST /register  3. Tests
  [User] yes
  [Coder] auth/login.go uses JWT but auth/session.go uses cookies.
           Which pattern for registration?
  [User] JWT, always JWT
  [Coder] Done. PR opened.
  → PR merged

Diff shows:
  + auth/register.go (new file, uses JWT)
  + auth/register_test.go (test with mock JWT)
  ~ go.mod: added github.com/golang-jwt/jwt/v5

Claude's analysis (from conversation + diff):
  1. Coder asked about JWT vs cookies — PM should have pre-resolved
     by reading auth/login.go and auth/session.go.
  2. Diff shows jwt/v5 is now a dependency — worth remembering.
  3. Diff shows a test pattern: mock JWT with fixed claims.
     This pattern should be noted for future auth tests.

Claude proposes in thread:
  📝 *Proposed memory updates:*
  1. ➕ PM: `Auth pattern: always use JWT, never cookies (auth/login.go is the reference)`
  2. ➕ PM: `When planning auth tasks, check auth/login.go for the JWT pattern`
  3. ➕ PM: `JWT dependency: github.com/golang-jwt/jwt/v5`
  4. 💡 CLAUDE.md: `Auth tests: mock JWT with fixed claims (see auth/register_test.go)`

  Reply *yes* to save all, or tell me what to change.
```

### What the User Sees in the Thread

```
PM: 📝 PR #42 merged! Here's what I'd like to remember:

  *PM memory (memory-pm.md):*
  1. ➕ Auth pattern: always use JWT, never cookies
  2. ➕ When planning auth tasks, always reference auth/login.go
  3. 🤝 Working with Coder: always mention JWT pattern in auth plans

  *CLAUDE.md suggestion:*
  4. 💡 Consider adding: "Auth uses JWT everywhere, never cookies"

  Reply *yes* to save all, or tell me what to change.
```

**For image-only threads** (no Coder, no PR):
```
PM: 📝 Thread done! Here's what I'd like to remember:

  *Artist memory (memory-artist.md):*
  1. ➕ Logo style: minimalist, blue #2563EB, no text
  2. ➕ User prefers "lost astronaut" motif over robots

  *PM memory (memory-pm.md):*
  3. 🤝 Working with Artist: default to blue #2563EB for all image prompts

  Reply *yes* to save all, or tell me what to change.
```

The user can:
- **"yes"** → save all proposed changes
- **"remove 3"** → save 1 and 2, skip 3
- **"add: we use bcrypt for passwords, never md5"** → add a custom learning
- **"change 1 to: Auth uses JWT everywhere except WebSocket handlers"** → edit before saving
- **"no"** → discard all, save nothing

### Memory Extraction Prompt (Always Claude)

Memory extraction always uses Claude, regardless of the thread's active
PM model. Claude receives the conversation AND the git diff.

```
You analyze completed conversations AND code diffs to extract learnings.
You receive:
1. The full thread (PM planning, image generation, user interactions,
   Coder implementation)
2. The git diff (main..branch) showing exactly what code changed

Your job has FOUR parts:

PART 1 — General learnings:
Extract useful decisions, conventions, and gotchas worth remembering.
For image threads: style preferences, color choices, motifs, formats.

PART 2 — Self-improvement:
- If Coder ran: look at what Coder asked. If it could have been
  answered during planning, propose a planning note for PM.
- If images were generated: note style preferences, prompt adjustments
  the user made, preferred formats/sizes. Add to Artist memory.
- If a workflow was followed: did it work well? Missing steps?
  Propose workflow refinements.

PART 3 — Inter-role learning:
- Did PM give the Coder enough context? If Coder asked implementation
  questions, PM should learn to include that info next time.
- Did PM give the Artist the right style info? If user corrected
  colors/sizes, update both PM and Artist memory.
- Did the Artist output in the right format for Coder? If there was
  a mismatch (wrong size, wrong path), both roles should learn.
- Route inter-role learnings to the "Working with Other Roles" section
  of the right memory file.

PART 4 — Diff analysis:
Analyze the git diff (main..branch) for patterns NOT discussed in
conversation but visible in the code changes:
- New files/packages created → PM should know about them
- Dependencies added (go.mod, package.json) → worth remembering
- Test patterns established → suggest for CLAUDE.md
- Error handling patterns → suggest for CLAUDE.md
- Naming conventions → suggest for CLAUDE.md
- Architecture patterns (new interfaces, new abstractions) → PM memory
- Config changes (new env vars, new flags) → PM memory
Only extract learnings from the diff that are GENUINELY USEFUL for
future threads. Don't note trivial changes.

Respond with a JSON array of operations. Each op targets a specific
memory file — route learnings to the right role:

PM memory (memory-pm.md):
- {"op": "append", "file": "pm", "line": "- ...", "category": "project"}
    — project decision, convention, or pattern
- {"op": "append", "file": "pm", "line": "- ...", "category": "planning"}
    — something PM should check/resolve during Phase 1 next time
- {"op": "append", "file": "pm", "line": "...", "category": "workflow"}
    — new workflow step or workflow refinement (include ## workflow-name)
- {"op": "append", "file": "pm", "line": "- ...", "category": "roles", "role": "Artist"}
    — how PM should work with Artist (goes to "Working with Other Roles / Artist")
- {"op": "append", "file": "pm", "line": "- ...", "category": "roles", "role": "Coder"}
    — how PM should work with Coder (goes to "Working with Other Roles / Coder")
- {"op": "replace", "file": "pm", "old": "exact existing line", "new": "merged line"}
    — update existing PM knowledge with new info

Artist memory (memory-artist.md):
- {"op": "append", "file": "artist", "line": "- ...", "category": "style"}
    — visual style preference, color, icon convention
- {"op": "append", "file": "artist", "line": "- ...", "category": "roles", "role": "PM"}
    — how Artist should work with PM (goes to "Working with Other Roles / PM")
- {"op": "append", "file": "artist", "line": "- ...", "category": "roles", "role": "Coder"}
    — how Artist should work with Coder (goes to "Working with Other Roles / Coder")
- {"op": "replace", "file": "artist", "old": "exact existing line", "new": "merged line"}
    — update existing Artist knowledge

Coder suggestion (not committed — shown as tip):
- {"op": "suggest-claude", "line": "- ..."}
    — something the user should consider adding to CLAUDE.md

No changes:
- {"op": "none"}

Rules:
- Route each learning to the RIGHT file. Project facts → PM. Colors → Artist.
- Workflow refinements go to PM (category "workflow"). Include the workflow name.
- "planning" learnings tell PM what to pre-check next time
- Visual/style learnings ALWAYS go to Artist, never PM
- Inter-role learnings go to category "roles" with the target role specified.
  If Coder asked a question PM should have answered → PM learns about Coder.
  If Artist used wrong size → both PM and Artist learn about each other.
- Coding conventions go as "suggest-claude" — PM doesn't write CLAUDE.md
- Keep each line concise (1 line max)
- Only record genuinely useful knowledge — not trivia
- Use "replace" to merge with existing entries when possible

Current PM memory:
---
{contents of memory-pm.md}
---

Current Artist memory:
---
{contents of memory-artist.md}
---

Thread conversation:
---
{full thread: PM planning + user messages + Claude implementation + image generation}
---
```

### Expected Response

```json
[
  {"op": "append", "file": "pm", "line": "- Auth: always JWT, never cookies. Reference: auth/login.go", "category": "project"},
  {"op": "append", "file": "pm", "line": "- Planning: when task touches auth, pre-read auth/login.go and auth/session.go to identify JWT vs cookie pattern", "category": "planning"},
  {"op": "replace", "file": "pm", "old": "- Registration endpoint exists at /register", "new": "- Registration at POST /register, uses JWT (same as login)"},
  {"op": "append", "file": "artist", "line": "- Login page uses blue gradient background, not flat", "category": "style"},
  {"op": "suggest-claude", "line": "- Auth uses JWT everywhere, never cookies (see auth/login.go)"}
]
```

### memory-pm.md Format

```markdown
# Workflows

## bugfix
Trigger: user reports a bug, error, or something broken
Steps:
1. Ask: what's the symptom? (error message, wrong behavior, where)
2. Grep + ReadFile to find the relevant code
3. GitLog to check recent changes in that area
4. Identify root cause, explain it to the user
5. Propose fix plan (which lines change, what test to add)
6. Wait for approval → Claude executes

## feature
Trigger: user wants to add new functionality
Steps:
1. Ask: what exactly should it do? Who uses it? Any UI?
2. ListFiles + Grep to find where it fits in the codebase
3. ReadFile existing similar features for patterns
4. Propose plan: new files, modified files, tests, migrations
5. If complex, break into sub-tasks and propose order
6. Wait for approval → Claude executes

## implement-milestone
Trigger: user wants to implement a defined milestone/epic
Steps:
1. Ask: which milestone? Read issue/doc if referenced
2. Break milestone into ordered tasks (each becomes a thread or sequential Claude run)
3. For each task: identify files, dependencies, test strategy
4. Propose the full implementation order with estimates
5. Execute tasks one by one, reporting progress after each
6. Final: integration test plan across all tasks

## question
Trigger: user asks about the codebase, architecture, or how something works
Steps:
1. ReadFile + Grep to find the answer
2. Explain clearly with file:line references
3. If the answer reveals a gap in memory, propose a memory update
4. No Claude needed — PM answers directly

# Project Knowledge
- Auth: always JWT, never cookies. Reference: auth/login.go
- Registration at POST /register, uses JWT (same as login)
- Tests use testify, not stdlib testing
- Deploy: make build → docker push → kubectl apply

# Planning Notes
- When task touches auth, pre-read auth/login.go and auth/session.go
- When task involves models, check existing models/ for GORM conventions
- Always mention the test framework (testify) in plans so Claude doesn't ask
```

### memory-artist.md Format

```markdown
# Style
- Primary color: #2563EB (blue)
- Design language: flat, minimalist, rounded corners
- Error pages: "lost astronaut" motif (not robots)
- Background: transparent for icons, white for illustrations

# Icons
- Size: 24px grid
- Stroke: 2px, outlined style
- Reference set: static/icons/
- Match existing style before creating new icons

# Image Generation Defaults
- Always use project primary color (#2563EB) unless told otherwise
- Flat design, no gradients, no 3D effects
- For app assets: SVG-friendly flat shapes
- For error pages: friendly illustrations, cartoon style

# Learned Preferences
- User prefers "lost astronaut" over robots for error imagery
- Icons should be outlined, never filled
- Logos: no text, abstract motifs only
```

### CLAUDE.md (Coder — already exists)

The Coder uses the project's existing `CLAUDE.md`. CodeButler does **not**
manage this file. The user and Claude maintain it as they normally would.
CodeButler only reads it to inject into Claude's prompt.

If a thread reveals a coding convention that should be in `CLAUDE.md`, the
PM notes it in the thread usage report (section 24.6) as a suggestion:
```
💡 Claude asked about the test framework during implementation.
   Consider adding to CLAUDE.md: "Tests use testify, not stdlib testing"
```

The PM never writes to `CLAUDE.md` — that's the user's domain.

### Three Roles, Three Memories

```
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  ProductManager   │    │     Artist        │    │     Coder         │
│                   │    │                   │    │                   │
│  Reads:           │    │  Reads:           │    │  Reads:           │
│  memory-pm.md     │    │  memory-artist.md │    │  CLAUDE.md        │
│                   │    │                   │    │                   │
│  Learns:          │    │  Learns:          │    │  Learns:          │
│  - workflows      │    │  - color palettes │    │  (user manages    │
│  - project facts  │    │  - icon styles    │    │   CLAUDE.md)      │
│  - planning tips  │    │  - visual motifs  │    │                   │
│  - clarification  │    │  - prompt tweaks  │    │                   │
│    patterns       │    │  - size prefs     │    │                   │
└──────────────────┘    └──────────────────┘    └──────────────────┘
```

Four sections in `memory-pm.md`:

1. **Workflows** — defines the playbooks the PM follows. Each workflow has
   a trigger (how to recognize it) and steps (what the PM does). These are
   the starting defaults. They evolve per project through the memory feedback
   loop (see below).

2. **Project Knowledge** — what the codebase does, architectural decisions,
   conventions. No visual/style info (that goes to Artist).

3. **Planning Notes** — what the PM should check during Phase 1. Self-improvement
   notes that make future planning more accurate.

4. **Working with Other Roles** — how to interact effectively with the
   Artist and Coder. What context each needs, what mistakes to avoid.

Three sections in `memory-artist.md`:

1. **Style** — colors, design language, general visual identity.

2. **Icons/Assets** — specific conventions for icons, illustrations, etc.

3. **Working with Other Roles** — what context the Artist gets from PM,
   how to format images for the Coder to use, what the user prefers.

#### Inter-Role Learning

Each role learns not just about the project, but about how to work
better with the other roles. This is the "Working with Other Roles"
section in each memory file.

**PM learns about the Artist:**
```markdown
# Working with Other Roles

## Artist
- Always read memory-artist.md before generating prompts
- Include the primary color (#2563EB) in every image prompt
- For icons: specify exact pixel size (Artist defaults to 1024x1024 otherwise)
- User prefers to see the prompt before generation — always show it first
- When generating multiple variants, ask user to pick before moving on
```

**PM learns about the Coder:**
```markdown
## Coder
- Always include file:line references in the plan — Coder wastes turns exploring without them
- Mention the test framework (testify) explicitly — Coder asked about it in thread #12
- For auth tasks: include the JWT pattern in the plan (Coder asked in thread #8)
- Keep plans under 6 steps — Coder handles detailed steps better than vague big ones
- If the task touches >5 files, break into sequential Claude runs
```

**Artist learns about the PM and Coder:**
```markdown
# Working with Other Roles

## PM
- PM sends prompts enriched with project context — trust the color/style suggestions
- If PM doesn't mention a style, read memory-artist.md defaults first
- Always output PNG — Coder needs PNG for web assets

## Coder
- Save images to the paths PM specifies (e.g. static/images/)
- Use web-friendly sizes: icons 24-48px, illustrations max 1024px
- If the image will be used in code, provide the exact filename
```

**How this evolves:**

```
Thread #5: PM sends Coder a plan without mentioning the test framework.
  → Coder asks "which test framework?"
  → Memory extraction proposes:
    memory-pm.md # Working with Other Roles / Coder:
    "- Always mention test framework (testify) in plans"
  → User approves → PM never forgets again

Thread #8: PM generates an icon prompt without pixel size.
  → Artist generates 1024x1024 instead of 24x24.
  → User: "too big"
  → Memory extraction proposes:
    memory-pm.md # Working with Other Roles / Artist:
    "- Always specify exact pixel size in icon prompts"
    memory-artist.md # Working with Other Roles / PM:
    "- If PM doesn't specify size, default to 24px for icons (project standard)"
  → Both roles learn from the same mistake

Thread #15: Coder creates a PR with images in the wrong directory.
  → PM proposes:
    memory-pm.md # Working with Other Roles / Coder:
    "- Specify target directory for images explicitly (e.g. static/images/)"
    memory-artist.md # Working with Other Roles / Coder:
    "- Save all generated images to static/images/ unless told otherwise"
```

Over time, the roles form a well-coordinated team. The PM knows exactly
what context the Coder needs. The Artist knows the project's visual
standards. The Coder gets precise plans with all the information it
needs to execute without asking questions.

#### Workflows Are Living Documents

Workflows are **not hardcoded**. They live in `memory-pm.md` and follow the
same approval + git flow as all other memory. This means:

- **Default workflows** are seeded on first run (bugfix, feature,
  implement-milestone, question). The PM proposes them, user approves.
- **Users can add custom workflows**. A game studio might add `asset-pipeline`.
  A data team might add `etl-job`. A mobile team might add `release-build`.
- **Workflows improve over time**. After each thread, the memory extraction
  can propose workflow refinements: "add step 2.5: check migrations/" or
  "the bugfix workflow should also check error logs".
- **Each project develops its own playbook**. After 20 threads, the
  workflows reflect that specific team's patterns, not generic defaults.

```
Week 1 (defaults):
  ## bugfix
  Steps: 1. Ask symptom  2. Grep  3. GitLog  4. Propose fix

Week 4 (after 15 threads):
  ## bugfix
  Steps: 1. Ask symptom  2. Check Sentry for error context
         3. Grep + ReadFile  4. GitLog for recent deploys
         5. Check if related to known flaky areas (see Planning Notes)
         6. Propose fix with rollback plan if touching payments/

Week 8 (team has learned):
  ## bugfix
  Steps: 1. Ask symptom  2. Check Sentry + Datadog  3. Grep
         4. If payments-related: always add integration test
         5. If auth-related: pre-read auth/login.go (JWT pattern)
         6. Propose fix, mention deploy window if critical
```

#### Custom Workflow Example

A user can define a new workflow at any time:

```
User: "I want to define a new workflow for database migrations"
PM: "Got it. Let me draft a workflow based on how your project handles migrations."
PM: (ReadFile config/database.go, Grep "migrate", ListFiles migrations/)
PM: "Here's what I'd add to Workflows:

  ## db-migration
  Trigger: user needs a schema change, new table, or column modification
  Steps:
  1. Ask: what changes? New table, alter column, add index?
  2. ReadFile the latest migration in migrations/ for naming pattern
  3. ReadFile models/ to understand current schema
  4. Propose migration SQL + model update + test
  5. If destructive (drop column, rename): flag it and ask for rollback plan
  6. Wait for approval → Claude executes

  Save this workflow?"
User: "yes, but add a step to check if there's a pending migration first"
PM: "Updated — added step 1.5: check for unapplied migrations. Saving."
→ Committed to memory-pm.md
```

Next time:
```
User: "I need to add a created_at column to the users table"
PM: "This looks like a *db-migration* workflow. Let me follow the steps..."
PM: (checks pending migrations, reads latest migration, reads User model)
PM: "Here's the plan: ..."
```

### Implementation

- **File**: `internal/memory/memory.go`
- **Functions**:
  - `LoadPM(repoDir) string` — read memory-pm.md (or "" if doesn't exist)
  - `LoadArtist(repoDir) string` — read memory-artist.md (or "" if doesn't exist)
  - `Apply(content string, ops []Op) string` — apply approved operations to a memory file
  - `Save(path, content string)` — write a memory file
  - `FormatProposal(ops []Op) string` — format ops as Slack message for user review
    (groups by target file: "PM memory:", "Artist memory:", "CLAUDE.md suggestion:")
  - `ParseUserResponse(text string, ops []Op) []Op` — process user edits/approvals
  - `SeedDefaults(repoDir)` — create memory-pm.md and memory-artist.md with
    default workflows and empty sections on first run
- **ProductManager**: uses `models.ProductManager` interface
  - Any provider that implements `Chat()` and `ChatJSON()`
  - Requires productManager config in global config
- **Daemon integration**: after PR creation, launch goroutine:
  1. Call PM for memory analysis (reads both memory files + thread)
  2. Post grouped proposal in thread (PM ops, Artist ops, Claude suggestions)
  3. Wait for user response (with timeout — if no response in 24h, discard)
  4. Commit approved changes to the right file(s) on the PR branch
  5. `git push` to update the PR
  6. For "suggest-claude" ops: just show in thread, never commit

### The Virtuous Cycle

```
Thread N (code task):
  PM plans → Claude implements → Claude asks "JWT or cookies?"
  → User: "JWT" → PR created
  → PM proposes: memory-pm.md: "always JWT" + "pre-check auth pattern"
  →              suggest for CLAUDE.md: "Auth uses JWT everywhere"
  → User: "yes"
  → memory-pm.md committed to PR branch + pushed
  → PR merged → memory-pm.md lands on main ✓

Thread N+1 (touches auth, branched from main after merge):
  PM reads memory-pm.md → knows JWT pattern → includes it in plan
  → Claude never asks → faster, cheaper, better

Thread N+2 (image task):
  User: "create an icon for settings"
  → Artist reads memory-artist.md → knows blue #2563EB, outlined, 24px
  → PM shows prompt with correct style (from Artist memory)
  → User: "perfect, go"
  → PM generates, user says "push it"
  → PR created → PM proposes: memory-artist.md: "settings icon is a gear"
  → PR merged → Artist learning on main ✓

Thread N+3 (code + image):
  User: "add a loading spinner to the dashboard"
  → Artist reads memory-artist.md → generates spinner with correct style
  → PM plans: "save spinner + update dashboard template"
  → Claude implements → no style questions needed

Thread N+4 (bugfix — workflow has evolved):
  User: "login is broken"
  → PM reads bugfix workflow from memory-pm.md
  → Workflow now says "check Sentry" (added after thread N)
  → PM follows refined steps → faster diagnosis
  → After fix: PM proposes workflow refinement:
    "bugfix step 3: also check auth/login.go JWT pattern"
  → User approves → workflow keeps improving

Thread N+5:
  Same bugfix workflow is now 2 steps more specific
  → PM catches auth issues on its own → fewer clarification rounds
  → System keeps improving, each role independently
```

Over time, each role's memory gets more accurate:
- **PM** workflows become project-specific playbooks
- **Artist** style memory becomes a complete brand guide
- **Coder** (via CLAUDE.md suggestions) accumulates coding conventions

The user drives this process — nothing gets remembered without their
approval. And because memory follows git flow, it's versioned,
reviewable, and merge-safe.

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
    → Claude extracts learnings (thread + diff) → user approves → committed to memory.md on branch
    → PR merged to main
    → memory.md now on main ✓

Thread B: "add password reset" (started after merge)
    → Branches from main → memory.md already has Thread A's learnings
    → PM reads memory → knows "auth uses bcrypt, sessions expire after 24h"
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
    Thread ends → Claude extracts learnings (thread + diff) → commits to memory.md on PR branch
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
| `memory.md` | Yes (injected) | Claude (extraction) + PM (reads) | Learnings from conversations + diffs, planning notes |
| Thread journal | No | Humans (user, reviewers) | Detailed narrative of what happened in the thread |
| PR description | No | Humans (reviewers) | Short summary (lives on GitHub) |

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

### Thread Journal — Detailed Narrative per Thread

The PR description is a short summary for reviewers. But sometimes
you want to read **exactly what happened** — every interaction, every
decision, every exchange between roles. That's the thread journal.

A thread journal is a markdown file committed to the PR branch that
tells the full story of the thread, oriented to the user (not to models).

#### What It Looks Like

```markdown
# Thread Journal: "fix the login bug"

**Started**: 2026-02-14 14:30 · **Closed**: 2026-02-14 15:12
**Branch**: codebutler/fix-login · **PR**: [#42](https://github.com/user/repo/pull/42)
**Slack thread**: [View conversation](https://myworkspace.slack.com/archives/C0123ABCDEF/p1732456789123456)
**PM model**: Kimi (switched to Claude at 14:45, back to Kimi at 14:52)

---

## Timeline

### 14:30 — User request
> fix the login bug, sometimes the session expires immediately after login

### 14:30 — PM (Kimi) analyzes
PM read `auth/session.go`, `auth/login.go`, and `auth/session_test.go`.
Found: session expiry uses `time.Now()` (local time) but cookies use UTC.
**Plan proposed:**
1. Fix timezone comparison in `auth/session.go`
2. Add test for timezone edge case
3. Check remember-me cookie behavior

### 14:31 — User approved plan
> yes

### 14:31 — Coder (Claude) starts working
Files modified: `auth/session.go`, `auth/login.go`, `auth/session_test.go`

### 14:35 — Coder asks PM a question
> "auth/login.go uses JWT but auth/session.go uses cookies. Which pattern
>  for registration?"
PM (Kimi) answered from memory: "JWT, always JWT"
*(PM could have included this in the plan — learning extracted)*

### 14:38 — Coder asks user (escalated by PM)
> "The remember-me checkbox doesn't exist in the UI yet. Should I add it,
>  or just fix the session expiry for now?"

### 14:39 — User responds
> just fix the session, remember-me is a separate ticket

### 14:42 — Coder done, PR #42 opened
Changes: 3 files, +45 -12 lines
- `auth/session.go`: Fixed UTC vs local time comparison
- `auth/login.go`: Set cookie timezone explicitly
- `auth/session_test.go`: Added timezone edge case test

### 14:42 — User requests deeper analysis
> /pm claude
PM switched to Claude (Pro).
> Can you also check if there are similar timezone bugs elsewhere?

### 14:45 — PM (Claude) investigates
Claude read 8 files across `auth/`, `middleware/`, and `handlers/`.
Found: `middleware/ratelimit.go` has a similar `time.Now()` vs UTC issue.
**Recommended**: fix in a follow-up thread (different scope).

### 14:48 — User switches back
> /pm kimi
> dale merge

### 14:48 — Memory extraction (Claude)
Proposed learnings:
1. ➕ PM: "All time comparisons must use UTC — check for time.Now() vs time.UTC()"
2. ➕ PM: "middleware/ratelimit.go has a timezone bug — needs separate fix"
3. 💡 CLAUDE.md: "Always use time.Now().UTC() for time comparisons"
User approved all 3.

### 14:49 — Thread closed
PR #42 merged (squash). Branch deleted. Worktree removed.

---

## Cost
| Role | Model | Calls | Cost |
|------|-------|-------|------|
| PM | Kimi | 4 | $0.004 |
| PM | Claude (Pro) | 2 | $0.06 |
| Coder | Claude | 1 (5 turns) | $0.85 |
| Memory extraction | Claude | 1 | $0.03 |
| **Total** | | | **$0.944** |
```

#### How It's Generated

The journal is **built incrementally** as the thread progresses —
each significant event appends to the file. It's NOT generated at the
end by summarizing the conversation (that would lose detail).

```go
// internal/journal/journal.go

type Journal struct {
    threadTS   string
    branch     string
    title      string // thread title (from PM classification)
    slackURL   string // link to Slack thread
    prURL      string // link to GitHub PR (set when PR is created)
    entries    []Entry
}

type Entry struct {
    Time    time.Time
    Type    string // "user", "pm", "coder", "exchange", "switch", "memory", "close"
    Content string // markdown-formatted
}

// Append adds an entry and commits the journal to the branch
func (j *Journal) Append(entry Entry) {
    j.entries = append(j.entries, entry)
    j.writeAndCommit()
}

// writeAndCommit renders the full journal.md and commits to the branch
func (j *Journal) writeAndCommit() {
    md := j.render() // full markdown from all entries
    // Write to .codebutler/journal.md on the branch
    // git add + commit with message "journal: <latest entry type>"
}
```

Events that get journal entries:

| Event | Journal entry |
|-------|--------------|
| User message | Quote of the message |
| PM plan proposed | Plan text + which files PM read |
| User approves/rejects plan | User's response |
| PM model switch (`/pm`) | Which model, when |
| Coder starts working | Files being modified |
| Coder→PM question (internal) | The question + PM's answer + source (memory/grep/file) |
| Coder→User question (escalated) | The question |
| User answers escalated question | User's response |
| Coder done / PR opened | Files changed, lines added/removed |
| Memory extraction | Proposed learnings + user's response |
| Thread closed | PR merged, cleanup done |

#### Where It Lives

```
.codebutler/journals/
  thread-1732456789.123456.md   ← one file per thread
```

The journal is committed to the PR branch alongside the code changes.
When the PR is merged, the journal lands on main permanently.

For threads that don't produce a PR (questions, image-only), the journal
is committed directly to main on thread close.

#### Journal vs PR Description vs Slack Thread

| | PR Description | Thread Journal | Slack Thread |
|---|---|---|---|
| **Audience** | Code reviewers | User (you, future you) | Everyone in channel |
| **Detail level** | Summary (30s read) | Full narrative (5min read) | Raw conversation |
| **Includes internal exchanges** | No | Yes (PM↔Coder questions) | No (those are invisible) |
| **Includes cost breakdown** | No | Yes | Only in summary message |
| **Includes PM model switches** | No | Yes | Only the switch messages |
| **Includes which files PM read** | No | Yes | No |
| **Searchable in repo** | No (GitHub only) | Yes (`grep -r "timezone" .codebutler/journals/`) | No (Slack search) |
| **Links to others** | Has Slack thread link | Has Slack thread link + PR link | Has PR link (bot message) |
| **Lives** | GitHub PR | Git repo | Slack |

All three are cross-linked: from the journal you can jump to the PR or
the Slack thread. From the PR description you can jump to the Slack thread.
From Slack you can see the PR link. Full traceability.

The key insight: the thread journal shows **what happened behind the
scenes** — the PM reading files, the internal exchanges between PM and
Coder, the decisions the PM made autonomously. This is invisible in Slack
(the user never sees internal exchanges) and absent from the PR description
(which is a summary). The journal is the detailed "director's commentary"
of the thread.

#### Gitignored or Not?

The journals live in `.codebutler/journals/` which is **NOT gitignored**.
They are part of the repo. This is intentional:

- They're a permanent record of how each change was developed
- `git log` shows commits, journals show the _thought process_
- You can grep journals to find "when did we decide to use bcrypt?"
- They're small (~2-5KB each) and text-only — negligible repo overhead
- Future threads can reference past journals for context

If the repo owner doesn't want them, they can add
`.codebutler/journals/` to `.gitignore`.

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

**`ReadMemory`** — Access role-specific memory

```json
{
  "name": "ReadMemory",
  "description": "Read a role's memory file. PM memory has workflows, project knowledge, and planning notes. Artist memory has style preferences and visual conventions.",
  "parameters": {
    "type": "object",
    "properties": {
      "role":    { "type": "string", "description": "'pm' (memory-pm.md) or 'artist' (memory-artist.md). Default: 'pm'." },
      "section": { "type": "string", "description": "Only return this section (e.g. 'Workflows', 'Style', 'Planning Notes'). Omit for full file." }
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
Thread ts_002 [Phase: PM] branch: (not yet created)
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

#### What the PM Does in Phase 1

1. **Selects a workflow**: reads `memory-pm.md`, matches the user's request
   to a workflow trigger. If unclear, asks: "Is this a bugfix, a new
   feature, or something else?" If the user describes a new type of work,
   the PM proposes a new custom workflow (see "Custom Workflow Example" in
   section 16). The selected workflow drives the rest of Phase 1.
2. **Follows the workflow steps**: each workflow defines the exploration and
   clarification steps. The PM follows them in order, using tools as needed.
3. **Scans the codebase**: grep, read files, check git log (cheap operations)
4. **Answers questions directly**: "what does X do?" → PM reads the file and explains (no Claude needed)
5. **Proposes a plan for code tasks**: lists files to change, approach, estimated complexity
6. **Generates images**: if the task involves creating visual assets (icons, logos, mockups), PM triggers the Artist via OpenAI gpt-image-1 — then asks what to do next (see below)
7. **Detects conflicts**: checks if other threads touch the same files
8. **Waits for approval**: user must say "yes" before Claude starts

#### Conversation Examples

**Simple question (PM handles entirely, Claude never runs):**
```
User: "how do we deploy?"
PM: (matches → question workflow)
PM: "Based on README.md and the Makefile, deployment is:
  1. make build
  2. docker push
  3. kubectl apply -f k8s/
  The CI pipeline in .github/workflows/deploy.yml does this automatically on merge to main."
```

**Bugfix (PM follows bugfix workflow):**
```
User: "login is broken"
PM: (matches → bugfix workflow, follows steps)
PM: "Following the *bugfix* workflow.
  I see several login-related files:
  - auth/login.go (POST /login endpoint)
  - auth/session.go (session management)
  - auth/middleware.go (auth middleware)
  What's happening? Error message? Timeout? Wrong redirect?"
User: "it redirects to a 404 after login"
PM: "Found it. auth/login.go:58 redirects to /dashboard but routes.go
  defines it as /app/dashboard. The redirect path is wrong.
  Plan: change redirect in auth/login.go:58 from /dashboard to /app/dashboard.
  Say *yes* to fix it."
User: "yes"
→ Claude executes
```

**Feature (PM follows feature workflow):**
```
User: "add user registration with email verification"
PM: (matches → feature workflow, follows steps)
PM: "That's a multi-file feature. Here's my plan:

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
PM: "Updated plan:
  1. Create `models/user.go` — User struct + DB migration
  2. Create `auth/register.go` — POST /register with bcrypt
  3. Update `routes.go` — add POST /register
  4. Add tests
  Say *yes*."
User: "dale"
→ Coder executes the refined plan
→ Coder: "Done. PR #42 opened with registration endpoint."
```

**Image generation — just show it (thread ends, no Coder):**
```
User: "create a logo for the app, minimalist, blue tones"
PM: (reads README, checks existing assets/ folder, notes the app name)
PM: "I'd send this prompt to the Artist:
  'Minimalist logo for CodeButler, a developer tool. Clean lines,
   blue tones (#2563EB), no text, abstract butler/code motif,
   SVG-friendly flat design'
  Want me to adjust anything?"
User: "go"
Artist: [generates via gpt-image-1, uploads image]
Artist: "Here it is. What do you want to do?
   ✅ Done  ·  🔄 Adjust  ·  📁 Push to repo  ·  🔧 Use with Coder"
User: "looks good"
→ User: "done" → memory extraction
  → memory-artist.md: "logo style: minimalist, blue #2563EB"
```

**Image generation — iterate then push to repo:**
```
User: "need a favicon for the web dashboard"
PM: (reads internal/daemon/web.go, sees HTML template, checks static/)
PM: "I'd generate with this prompt:
  'Favicon 32x32px, flat design, blue (#2563EB) on transparent
   background, matches existing icon style in static/icons/'
  Adjust?"
User: "looks good"
Artist: [generates via gpt-image-1, uploads image]
Artist: "What do you think?"
User: "make it rounder"
Artist: "Updated prompt: '...rounded corners, circular shape...' Generating."
Artist: [uploads new image] "Better?"
User: "yes, push it"
PM: creates branch, adds favicon to static/, commits, opens PR
→ PR created → memory extraction
  → memory-artist.md: "favicon is round, blue"
→ User: "dale merge" → summary + merge + cleanup
```

**Image generation — then hand off to Coder for implementation:**
```
User: "create an error illustration for the 404 page"
PM: (reads routes, finds 404 handler, checks templates)
PM: "I'd generate: 'Friendly 404 error illustration, cartoon style,
  confused robot, matches the blue/white theme in templates/base.html'
  Want to adjust the prompt?"
User: "make it a lost astronaut instead of a robot"
PM: "Updated. Generating..."
Artist: [generates via gpt-image-1, uploads image]
Artist: "Here it is. What next?"
User: "use it in the 404 page, replace the current text-only error"
PM: "Plan:
  1. Save illustration to static/images/404.png
  2. Update templates/error.html to show the image
  3. Update the CSS for centered layout
  Say *yes* to start Coder."
User: "yes"
Coder: "Working on the 404 page..."
→ Phase 2: Coder gets the plan + the generated image in worktree
```

**Image with user reference — PM asks for inspiration:**
```
User: "need icons for the settings page"
PM: (checks static/icons/, sees existing icon style)
PM: "I see existing icons in static/icons/ use a flat, 24px style.
  Do you have a reference image for the style you want? Or should I
  match the existing set?"
User: [sends screenshot of Material Design icons]
PM: "Got it. I'll tell the Artist: 'Settings gear icon, Material Design
  style, 24x24px, matching the attached reference, outlined, 2px
  stroke, #333 on transparent'
  Adjust?"
User: "perfect"
Artist: [generates using edit API with reference, uploads result]
Artist: "What next?"
```

**Image editing — user sends a photo directly:**
```
User: [attaches screenshot] "remove the watermark and use this as the hero image"
PM: "I'd tell the Artist: 'Remove watermark, clean up
  artifacts, maintain original colors and composition'
  Adjust?"
User: "also make it wider, 1920x1080"
Artist: "Updated: '...remove watermark, extend to 1920x1080,
  fill edges naturally...' Generating."
Artist: [uploads edited image] "What next?"
User: "push it and have Claude add it to the landing page"
PM: "Plan:
  1. Save to static/images/hero.png (branch + PR)
  2. Coder updates templates/landing.html to use the new hero
  Say *yes*."
User: "dale"
→ PM pushes image first, then Coder implements the template change
```

#### PM System Prompt

```
You are the ProductManager (PM) — an AI development assistant.
Your role is to UNDERSTAND, DEFINE, and PLAN — never to write code.
Your messages are prefixed with "PM:" so users know who is talking.

You work alongside two other roles:
- Artist: generates and edits images. You coordinate with Artist.
- Coder: writes code, runs tests, creates PRs. You hand off plans to Coder.

EXPLORATION TOOLS (read-only, use freely):
- ReadFile(path, offset?, limit?) — read file contents with line numbers
- Grep(pattern, glob?, path?) — search for regex patterns across files
- ListFiles(pattern, path?) — find files by glob pattern
- GitLog(n?, path?) — see recent commits
- GitDiff(ref?, path?) — see uncommitted or ref-based changes
- ReadMemory(role?, section?) — read memory (pm or artist)
- ListThreads(status?) — see active threads and their files
- GHStatus(type, number) — check GitHub PR/issue status

ACTION TOOLS (use when the user requests):
- GenerateImage(prompt, size?) — trigger the Artist to create an image
- EditImage(prompt, image, size?) — trigger the Artist to edit an image
- UploadImage(path) — send image to Slack
- CreateBranch(name) — create git branch
- CommitAndPush(files, message) — commit files and push
- OpenPR(title, body) — open a pull request

WORKFLOWS:
On every new thread, your FIRST step is:
1. ReadMemory(role="pm", section="Workflows") to load your workflows
2. Match the user's request to a workflow trigger
3. If unclear, ask: "Is this a bugfix, a new feature, or something else?"
4. If the user describes a new type of work that doesn't match any
   workflow, propose a new custom workflow for their approval
5. Follow the matched workflow steps in order

Each workflow defines the exploration and clarification steps. Follow
them — they've been refined by this team over time.

WORKING WITH OTHER ROLES:
Before starting, also read:
- ReadMemory(role="pm", section="Working with Other Roles")
This tells you what context each role needs from you. Follow it.

When handing off to Coder:
- Include file:line references in the plan
- Mention test framework, coding conventions, patterns
- Check "Working with Other Roles / Coder" for project-specific notes

When triggering Artist:
- Read memory-artist.md first for style defaults
- Include colors, sizes, and style in the prompt
- Check "Working with Other Roles / Artist" for project-specific notes

RULES:
- Use exploration tools proactively. Don't guess — look.
- When a user asks about code, READ the actual files before answering.
- When planning a code task, GREP and READ to find existing patterns.
- When detecting conflicts, LIST active threads and compare file lists.
- Never make claims about code you haven't read with ReadFile.
- Never propose changes to files you haven't read.

Your job is to fully define the task before handing it to the Coder.
The Coder should NEVER need to ask "what do you mean?" or "what should
this do?". By the time the Coder starts, the task must be unambiguous.

Your plan must be detailed enough that the Coder can implement it
without asking requirements questions. Implementation questions
("bcrypt or argon2?") are fine — scope questions ("what fields should
User have?") mean your plan wasn't complete enough.

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
PM: scan repo for context (using tools)
  - check existing assets (static/, images/, public/)
  - read any design tokens / CSS variables
  - ReadMemory(role="artist") for style defaults
  - note project name, branding, existing style
    ↓
PM: build context-aware prompt, SHOW IT to the user
  "I'd tell the Artist:
   'Minimalist 32x32 favicon, flat design, blue (#2563EB) on
    transparent background, matching the existing icon set in
    static/icons/ which uses rounded corners and 2px stroke'
   Want me to adjust anything before generating?"
    ↓
User: "looks good" / "make it green instead" / adjusts prompt
    ↓
Artist: generate/edit with approved prompt (gpt-image-1)
    ↓
Artist: upload to Slack thread + ask user what to do next
    ↓
User chooses:
  ├─ "looks good"/"done" → memory extraction → thread closed
  ├─ "make it rounder"   → Artist adjusts prompt, shows it, re-generates
  ├─ "push it"           → PM creates branch + PR with asset
  ├─ "use it in the app" → PM builds plan → user approves → Coder starts
  └─ (sends a photo)     → Artist uses it as input for edit/inspiration
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

In all cases, PM shows the prompt before generating, and Artist prefixes
its messages with "Artist:" so the user sees the role transition.

#### Thread Outcomes After Image Generation

The key insight: image generation can **resolve the thread entirely**
or be a **stepping stone to Coder**. PM asks, the user decides.

| User says | What happens | Phase 2? | PR? |
|---|---|---|---|
| "looks good" / "done" | Thread ends | No | No |
| "make it X" / "try again" | Artist re-generates, loop | No (yet) | No |
| "push it to the repo" | PM: branch + commit + PR | No | Yes |
| "push it and implement" | PM: push asset, then plan for Coder | Yes | Yes |
| "use it in the app" | PM: plan includes the image | Yes | Yes |
| [sends new image] | Artist: use as reference, re-generate | No (yet) | No |

#### PM Pushing Assets (Without Coder)

When the user says "push it", PM handles it directly:

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

#### What Coder Can and Cannot Ask

| Coder asks | Who answers | Why |
|---|---|---|
| "should I use bcrypt or argon2?" | **PM** (checks memory + codebase) | Implementation choice — PM can research it |
| "the tests use testify but this module uses stdlib, which?" | **PM** (greps codebase) | Codebase ambiguity — PM reads the files |
| "I found a race condition in the existing auth — fix it too?" | **PM** (decides scope) | Discovered issue — PM decides fix or skip |
| "which HTTP router does this project use?" | **PM** (reads go.mod, imports) | Convention — PM reads the answer |
| "what fields should the user model have?" | **Escalate to user** | Requirements gap — PM can't decide this |
| "what does 'fix the login bug' mean?" | **Should never happen** | This was resolved in Phase 1 |
| "should I add email verification?" | **Escalate to user** | Scope change — user decides |

If a question is classified as "answerable", the PM resolves it using
its tools and memory. The user never sees the exchange. If it's a scope
or requirements question, the PM escalates to the user with context:
`PM: "Coder asks: [question]. What should we do?"`

Over time, the PM learns to include answers to common questions directly
in the plan (via "Working with Other Roles / Coder" memory). Questions
that repeat get pre-answered.

### 24.4 Post-flight: After PR Creation + Thread Close

Post-flight has two stages:

**Stage 1: After PR creation** — memory extraction begins immediately:
```
PR created (detected in Claude's response)
    ↓
    ├─ PM (active model): add thread URL to PR body
    ├─ PM (active model): detect if Coder left TODO/FIXME in code
    │   → warn in thread: "Coder left 2 TODOs — want a new thread to resolve them?"
    │
    ├─ Journal: append "PR opened" entry → commit to PR branch
    │
    └─ Claude (always): memory extraction
        → git diff main..branch → get the full diff
        → analyze full thread + diff → propose memory updates (section 16)
        → post in thread → wait for user approval
        → commit approved memory.md changes to PR branch → push
```

Memory extraction always uses Claude regardless of the thread's active
PM model. This is the one phase where we always want the best brain —
learnings compound over time and a missed pattern now costs future threads.

**Stage 2: User closes thread** — when the PR is approved and user says
"done"/"merge"/"dale", CodeButler finishes the thread:
```
User: "merge" / "done" / "dale"
    ↓
    ├─ PM (active model): generate PR summary → update PR description via `gh pr edit`
    ├─ Journal: finalize (close entry + cost table) → commit to branch
    ├─ Merge PR: `gh pr merge --squash` (journal lands on main with merge)
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
    └─ PM (active model): summarize for Slack (if response > 4000 chars)
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
    → PM: selects workflow, explores, proposes plan
    → User approves → worktree + branch created
    → Coder: starts working
    → Coder: modifies files (tracked per response)
    → Coder: opens PR
    → PM: memory extraction → committed to PR branch
    → User: "dale merge"
    → PM: thread summary → PR merged, branch deleted, worktree removed
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

*PM*
  Calls: 4 (3 conversation + 1 memory extraction)
  Tools used: Grep ×2, ReadFile ×3, GitLog ×1
  Tokens: 2,340 in → 1,180 out
  Cost: $0.004

*Coder*
  Calls: 1 (5 turns)
  Tokens: 18,400 in → 6,200 out
  Cost: $0.62

*Total: $0.626*

🔍 *Behind the scenes (what PM resolved for Coder):*
  1. Coder: "the tests use testify but this module uses stdlib, which?"
     → PM: "testify — 8 files use it, only this module was inconsistent.
       See internal/auth/auth_test.go:1 for the pattern."
     ✅ Added to PM memory for future plans.

💡 *Tips for next time:*
  • You gave a vague request ("fix the login bug") which required 2
    clarification rounds. Try: "fix the session expiry bug in auth/session.go"
  • The PM explored 6 files to find the issue. If you know the file,
    mention it — saves ~3 tool calls.

  If any PM answer above was wrong, reply with the correction —
  PM will update its memory.
```

The thread usage report has three sections:

1. **Stats** — tokens, costs, tool calls per role. Quick overview.
2. **Behind the scenes** — every inter-role exchange, showing the exact
   question and PM's answer. This is the transparency layer: the user
   sees what the PM decided on their behalf.
3. **Tips** — PM-generated suggestions for more efficient prompting.

The "behind the scenes" section serves three purposes:

**The user learns how the system works.** After a few threads, users
understand that the PM handles implementation questions autonomously.
They see what kind of questions get resolved internally vs escalated.
They start trusting the system — or correcting it when it's wrong.

**The user can correct wrong answers.** If the PM told the Coder
"use testify" but the user actually wanted stdlib for this module,
they reply with the correction. The PM updates its memory immediately.
This is critical — autonomous resolution only works if the user has
a way to course-correct.

**The user learns what to include upfront.** If every thread shows
"PM answered: which test framework?" the user learns to mention it
in the initial request. The tips reinforce this, but seeing the
actual exchange is more convincing.

```
User corrects a wrong PM answer:

PM: 📊 Thread Summary...
  🔍 Behind the scenes:
    1. Coder: "bcrypt or argon2 for password hashing?"
       → PM: "bcrypt — based on auth/login.go:34"

User: "actually we're migrating to argon2, the login.go code is legacy"

PM: "Got it. I'll update my memory:
  ❌ Old: 'Auth uses bcrypt for passwords (auth/login.go:34)'
  ✅ New: 'Auth is migrating from bcrypt to argon2. New code uses argon2.
     auth/login.go bcrypt is legacy, will be updated.'
  Saved to memory-pm.md."
```

This correction flow means the system is **self-healing**. Wrong
autonomous answers get caught at the summary, not at production time.

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

    // Inter-role dialogue stats
    InterRole  InterRoleUsage
}

type InterRoleUsage struct {
    Exchanges       []InterRoleExchange // full log of every exchange
    ResolvedCount   int                 // questions PM resolved internally
    EscalatedCount  int                 // questions escalated to user
}

// InterRoleExchange records one inter-role Q&A for the thread summary.
type InterRoleExchange struct {
    From     string // "Coder" or "Artist"
    Question string // what they asked
    Answer   string // what PM answered
    Source   string // how PM found the answer ("memory-pm.md", "ReadFile auth/login.go", etc.)
    Escalated bool  // true if this went to the user instead
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

#### How the PM Generates the Summary

The PM generates the thread summary in one `Chat` call, producing the
"behind the scenes" section and tips together:

```
System: "Generate a thread summary report with two sections.

SECTION 1 — Behind the scenes:
For each inter-role exchange (PM answering Coder or Artist questions),
show:
- What the other role asked (exact question)
- What PM answered and how it found the answer (memory, ReadFile, Grep)
- Whether PM's answer was added to memory or was already there
Format each exchange as a numbered list item.
If there were no inter-role exchanges, skip this section.

SECTION 2 — Tips (1-3 max):
Analyze the thread and suggest how the user could be more efficient:
- Was the initial request vague? Suggest how to be more specific.
- Did the PM need many tool calls to find context? Suggest mentioning
  file names or areas of the codebase.
- Did the user change requirements after Coder started? Suggest
  defining scope better upfront.
- Were there inter-role questions that the user could have preempted
  by being more specific in the initial request?
Be concrete and reference the actual conversation. Keep each tip
to 1-2 sentences.

End with: 'If any PM answer above was wrong, reply with the correction.'"

User: {full thread conversation + inter-role exchange log + usage data}
```

#### Conversation Examples

**Efficient thread (short report, no behind-the-scenes):**
```
📊 *Thread Summary: "add CORS header to /api/ endpoints"*

*PM*
  Calls: 2 (1 conversation + 1 memory)
  Tools used: Grep ×1, ReadFile ×1
  Tokens: 890 in → 420 out · $0.001

*Coder*
  Calls: 1 (3 turns)
  Tokens: 12,100 in → 4,800 out · $0.38

*Total: $0.381*

✅ Clean thread — clear request, Coder had no questions.
```

**Question-only thread (no Coder):**
```
📊 *Thread Summary: "how do we handle auth tokens?"*

*PM*
  Calls: 2 (1 conversation + 1 memory)
  Tools used: Grep ×1, ReadFile ×2
  Tokens: 1,200 in → 680 out · $0.002

*Coder*
  Not used — PM answered directly.

*Total: $0.002*
```

**Image thread (PM ↔ Artist exchange):**
```
📊 *Thread Summary: "create a settings icon"*

*PM*
  Calls: 3 (2 conversation + 1 memory)
  Tools used: ListFiles ×1, ReadFile ×1, ReadMemory ×1
  Tokens: 1,800 in → 900 out · $0.003

*Artist*
  Calls: 1 (1 generation)
  Cost: $0.02

*Total: $0.023*

🔍 *Behind the scenes (PM ↔ Artist):*
  1. Artist: "Size and background?"
     → PM: "24x24px, transparent. From memory-artist.md defaults."
  2. Artist: "Should I match existing icons in static/icons/?"
     → PM: "Yes — I read static/icons/gear.svg. Use 2px outlined stroke."

  Both learned — this info is now in Artist memory for next time.
```

**Expensive thread (lots of behind-the-scenes):**
```
📊 *Thread Summary: "refactor the whole auth module"*

*PM*
  Calls: 6 (5 conversation + 1 memory)
  Tools used: ListFiles ×1, Grep ×4, ReadFile ×8, GitLog ×2
  Tokens: 8,400 in → 3,200 out · $0.012

*Coder*
  Calls: 3 (12 turns total)
  Tokens: 68,000 in → 24,000 out · $2.84

*Total: $2.858*

🔍 *Behind the scenes (PM ↔ Coder):*
  1. Coder: "the tests use testify but this module uses stdlib, which?"
     → PM: "testify — 8/9 test files use it. This module was the outlier."
     ✅ Added to PM memory.
  2. Coder: "auth/login.go uses JWT but auth/session.go uses cookies. Which?"
     → PM: "JWT — memory-pm.md says 'always JWT, never cookies'."
     ✅ Already in memory, will include in future plans.
  3. Coder: "should errors return 401 or 403 for expired tokens?"
     → PM: "401 — I read middleware.go:28, existing pattern returns 401."
     ✅ Added to PM memory.

💡 *Tips for next time:*
  • Your initial request was broad ("refactor the whole auth module").
    The PM needed 15 tool calls to understand scope. Try breaking it
    into smaller tasks: "extract session logic from auth/login.go
    into its own file".
  • You changed requirements twice after Coder started. Each change
    cost ~$0.80 in Coder turns. Finalize scope with the PM before
    approving.

  If any PM answer above was wrong, reply with the correction.
```

### 24.7 The Full Pipeline

Every thread follows this pipeline. The PM owns the conversation until
the user approves. The Coder only appears after approval. During Phase 2,
the PM stays "on call" to answer Coder questions internally (see 24.7b).
Some threads never leave Phase 1 — the PM can resolve questions, generate
images via the Artist, and even push assets to the repo without the Coder.

```
┌──────────────────────────────────────────────────────┐
│                NEW SLACK THREAD                       │
│            User: "fix the login bug"                  │
└──────────────────────┬───────────────────────────────┘
                       ↓
    ╔══════════════════════════════════════╗
    ║     PHASE 1: PM + Artist (cheap)    ║
    ║     ~$0.001-0.005 per message       ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  PM: select workflow from memory     ║
    ║  PM: scan repo with tools            ║
    ║      ↓                               ║
    ║  PM: ask questions / propose plan   ║──→ User replies
    ║      ↓                               ║     (loop until
    ║  PM: refine plan                    ║←── plan is right)
    ║      ↓                               ║
    ║  (optional) Artist: gen/edit images  ║
    ║      ↓                               ║
    ║  PM: "Here's the plan. Yes?"        ║
    ║                                      ║
    ╚════╤═════════════════╤═══════════════╝
         │                 │
         │              ┌──┴──────────────────────────┐
         │              │  Thread resolved by PM:      │
         │              │  question answered, or       │
         │              │  images delivered by Artist,  │
         │              │  or assets pushed via PR      │
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
    ║      PHASE 2: CODER (expensive)     ║
    ║          ~$0.30-2.00 per run         ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  Coder: execute approved plan       ║
    ║  Coder: edit files, run tests       ║
    ║  (images from Phase 1 available      ║
    ║   in worktree if generated)          ║
    ║      ↓                               ║
    ║  Coder question? ──→ PM answers     ║
    ║    (internal, user doesn't see)      ║
    ║    OR scope question → user          ║
    ║      ↓                               ║
    ║  Coder: commit, push, open PR       ║
    ║      ↓                               ║
    ║  User replies → Coder --resume      ║
    ║                                      ║
    ╚══════════════════╤═══════════════════╝
                       ↓
              ┌────────────────┐
              │  PR created    │
              └────────┬───────┘
                       ↓
    ╔══════════════════════════════════════╗
    ║    PHASE 3: MEMORY + CLOSE (PM)      ║
    ║          ~$0.003                      ║
    ╠══════════════════════════════════════╣
    ║                                      ║
    ║  3a. Memory extraction:              ║
    ║  PM: analyze full thread             ║
    ║    - what Coder asked (learnings)    ║
    ║    - project decisions made          ║
    ║    - inter-role learnings            ║
    ║    - workflow refinements            ║
    ║      ↓                               ║
    ║  PM: post proposed updates           ║
    ║    "📝 Here's what I'd remember..."  ║
    ║    (grouped: PM memory, Artist       ║
    ║     memory, CLAUDE.md suggestions)   ║
    ║      ↓                               ║
    ║  User: approves / edits / adds       ║
    ║      ↓                               ║
    ║  Commit to right memory file(s)      ║
    ║  git push                            ║
    ║                                      ║
    ║  ── PR open for review ──            ║
    ║                                      ║
    ║  3b. User closes thread:             ║
    ║  User: "merge" / "done" / "dale"     ║
    ║      ↓                               ║
    ║  PM: generate summary → PR desc      ║
    ║  PM: post thread usage report        ║
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
- **Question** → PM answers directly. User says "done". Memory extraction → committed directly to main. ~$0.002.
- **Image only** → Artist generates, user says "looks good". Thread done. ~$0.01.
- **Image + push** → PM pushes asset via PR. Memory committed to PR branch. ~$0.01.
- **Image + Coder** → Artist generates, then transitions to Phase 2 with images ready.

### 24.7b Inter-Role Dialogue — Roles Talk to Each Other

Phase transitions are not fire-and-forget handoffs. When PM sends specs
to the Artist or a plan to the Coder, the receiving role can **ask back**.
This creates a dialogue between roles that happens behind the scenes —
invisible to the user unless a question gets escalated.

#### The Concept

```
OLD (unidirectional):
    PM ──plan──→ Coder ──works──→ result
         (Coder confused about test framework? Too bad, ask the user.)

NEW (bidirectional):
    PM ──plan──→ Coder
              ←── "which test framework?"
    PM ──(reads memory, checks codebase)──→ "testify, see internal/auth/auth_test.go:1"
              ←── "got it, proceeding"
    Coder ──works──→ result
         (User never saw the exchange. Both roles learned.)
```

#### Why This Matters

Today, when Claude (Coder) hits a question the PM should have answered,
it asks the **user**. This is expensive in two ways:
1. The user has to context-switch to answer an implementation question
2. The Coder pauses while waiting for a human response

With inter-role dialogue, the PM acts as a **live oracle** for the Coder.
The PM has tools (ReadFile, Grep, ReadMemory) — it can answer most
implementation questions itself, faster than a human.

```
Without inter-role dialogue:
    Coder: "the tests use testify but this module uses stdlib, which?"
    → Message appears in Slack → User reads → User answers → Coder resumes
    → 5 minutes of human waiting

With inter-role dialogue:
    Coder: "the tests use testify but this module uses stdlib, which?"
    → PM: ReadMemory → "testify" + Grep("testify") → 8 files use it
    → PM: "testify. The project standard. See memory-pm.md and 8 files."
    → Coder resumes immediately
    → 2 seconds, no human involved
```

#### PM ↔ Artist Dialogue

When the PM triggers image generation, the Artist can ask clarifying
questions before generating. These questions are answered by the PM
using its tools and memory — not by the user.

```
PM: "Generate a settings icon, flat design, for the web dashboard"
    ↓
Artist: "What size? What color? Should I match existing icons?"
    ↓
PM: (ReadMemory(role="artist") → "24px, #2563EB, outlined 2px stroke")
PM: (Grep("icon") → finds static/icons/, reads a few)
PM: "24x24px, blue #2563EB, 2px outlined stroke, match static/icons/*.svg"
    ↓
Artist: generates with full context
```

**What gets escalated to the user:**
- Style questions that aren't in Artist memory ("cartoon or flat?")
- Ambiguous requests where PM genuinely can't decide
- First-time tasks where no memory exists yet

**What gets resolved internally:**
- Size, color, format (if in Artist memory)
- Style matching (PM reads existing assets)
- File naming and paths (PM checks project structure)

```
Thread with inter-role dialogue (user sees):

PM: "I'd tell the Artist:
  'Settings gear icon, 24x24px, blue #2563EB, outlined 2px stroke,
   matching static/icons/ style, on transparent background'
  Want to adjust?"
User: "go"
Artist: [uploads image] "Here it is."

What happened internally (user doesn't see):
  PM → Artist: "Generate settings icon for web dashboard"
  Artist → PM: "Size? Color? Existing style reference?"
  PM: ReadMemory(role="artist") → got defaults
  PM: ListFiles("*.svg", "static/icons/") → found reference set
  PM: ReadFile("static/icons/gear.svg") → understood style
  PM → Artist: "24x24px, #2563EB, 2px outlined, match gear.svg style"
  PM → User: shows the resolved prompt for approval
```

Over time, this dialogue gets **shorter**. After 10 image threads:

```
PM → Artist: "Settings gear icon, 24x24px"
Artist: (reads own memory → knows all defaults)
Artist: generates immediately, no questions
```

The inter-role dialogue trained both roles:
- PM learned to include size in the first message
- Artist learned the project's default color, stroke, and style

#### PM ↔ Coder Dialogue

When the Coder hits an implementation question during Phase 2, instead
of immediately asking the user, it asks the PM first. The PM tries to
answer using tools and memory. Only if the PM can't answer does it
escalate to the user.

**Question taxonomy:**

| Question type | Who answers | Example |
|---|---|---|
| Implementation detail | PM (tools + memory) | "which test framework?" |
| Codebase ambiguity | PM (reads files) | "testify or stdlib?" |
| Convention/pattern | PM (reads memory) | "JWT or cookies for auth?" |
| Discovered issue | PM decides: fix or skip | "found a race condition, fix too?" |
| Scope change | **Escalate to user** | "should I add email verification?" |
| Requirements gap | **Escalate to user** | "what fields should User have?" |

**How it works technically:**

The Coder is Claude CLI (`claude -p`). It can't directly "talk" to the PM.
Instead, the daemon intercepts Coder questions and routes them:

```
Coder output contains a question
    ↓
Daemon detects question (PM classifies: answerable vs escalate)
    ↓
If answerable:
    PM: uses tools to find the answer
    PM → Coder: injects answer as follow-up message (--resume)
    Coder: continues working
    ↓
If scope/requirements:
    PM → User: "Coder asks: [question]. What should we do?"
    User → PM: answers
    PM → Coder: injects answer (--resume)
```

**Conversation example — resolved internally:**
```
Thread (user sees):
  PM: "Plan: add user registration with bcrypt. Say *yes*."
  User: "dale"
  Coder: "Working on it..."
  Coder: "Done. PR #42 opened."

What happened internally:
  Coder → Daemon: "auth/login.go uses JWT but auth/session.go uses cookies.
                    Which pattern for registration?"
  Daemon → PM: classify this question
  PM: ReadMemory → "Auth: always JWT" → answerable!
  PM → Daemon: "JWT. Memory says: always JWT, never cookies."
  Daemon → Coder: --resume "Use JWT. The project standard is JWT for
                   all auth (see memory-pm.md). Never cookies."
  Coder: continues, finishes, opens PR
  User never saw the exchange.
```

**Conversation example — escalated to user:**
```
Thread (user sees):
  PM: "Plan: add user registration with bcrypt. Say *yes*."
  User: "dale"
  Coder: "Working on it..."
  PM: "Coder asks: Should registration require email verification,
       or just username + password? This wasn't in the plan."
  User: "just username and password, no email"
  Coder: "Got it, continuing..."
  Coder: "Done. PR #42 opened."

What happened:
  Coder → Daemon: "Should registration require email verification?"
  Daemon → PM: classify
  PM: this is a scope question → can't answer → escalate
  PM → User: "Coder asks: ..."
  User → PM: "just username and password"
  PM → Coder: --resume "Username + password only. No email verification."
```

#### Artist ↔ PM Dialogue (After Generation)

After the Artist generates an image, the PM can also ask the Artist
questions before presenting to the user:

```
Artist: generates image
PM: "Is this transparent background? The user needs it for a web icon."
Artist: "Yes, PNG with alpha channel, 24x24px."
PM: (satisfied) → shows to user
```

This is less common but useful when PM needs to verify the output
matches requirements before showing it to the user.

#### Cost of Inter-Role Dialogue

Each inter-role exchange is 1-2 cheap LLM calls ($0.001-0.003).
This is dramatically cheaper than:
- Coder wasting an expensive turn asking the user ($0.10-0.30)
- User context-switching to answer (priceless time)
- Artist generating the wrong thing and needing a redo ($0.02-0.04)

```
Typical thread cost breakdown:

Without inter-role dialogue:
  PM planning:     $0.004  (3 messages)
  Coder working:   $0.84   (includes 2 user questions = 2 wasted turns)
  Total:           $0.844

With inter-role dialogue:
  PM planning:     $0.004  (3 messages)
  PM answering Coder: $0.002  (1 internal exchange)
  Coder working:   $0.52   (no wasted turns, focused execution)
  Total:           $0.526  (38% cheaper, faster, no user interruption)
```

#### The Learning Loop

Every inter-role exchange generates a learning opportunity. After the
thread closes, memory extraction analyzes these internal dialogues:

```
Internal exchange during thread:
  Coder → PM: "which test framework?"
  PM → Coder: "testify"

Memory extraction proposes:
  memory-pm.md # Working with Other Roles / Coder:
  "- Always mention test framework (testify) in plans for code tasks"

Next thread:
  PM: includes "Tests: use testify (see internal/auth/auth_test.go)"
  in the plan → Coder never asks → exchange eliminated
```

```
Internal exchange during thread:
  PM → Artist: "Generate settings icon"
  Artist → PM: "What size?"
  PM → Artist: "24x24px"

Memory extraction proposes:
  memory-pm.md # Working with Other Roles / Artist:
  "- Always specify pixel size in icon prompts (project default: 24px)"
  memory-artist.md # Working with Other Roles / PM:
  "- If PM doesn't specify size for icons, default to 24px"

Next thread:
  PM: includes size in prompt → Artist doesn't ask → exchange eliminated
  AND even if PM forgets, Artist defaults to 24px → still correct
```

Over time, the internal dialogues get shorter and eventually disappear
as both roles learn exactly what the other needs. The system converges
toward zero-overhead handoffs.

```
Week 1: PM → Coder exchange on every thread (5 questions per thread)
Week 3: 2 questions per thread (common patterns learned)
Week 6: 0-1 questions per thread (roles know each other's needs)
Week 8: Rare — only novel situations trigger inter-role dialogue
```

#### Implementation

```go
// internal/daemon/interrole.go

// QuestionType classifies whether a question can be answered internally.
type QuestionType string
const (
    Answerable QuestionType = "answerable"  // PM can resolve with tools + memory
    Escalate   QuestionType = "escalate"    // scope/requirements → ask user
)

// ClassifyQuestion asks the PM to classify a Coder question.
func (d *Daemon) ClassifyQuestion(ctx context.Context, question string,
    thread *Thread) (QuestionType, error) {
    // PM Chat call with question + thread context
    // Returns "answerable" or "escalate"
}

// AnswerForCoder asks the PM to answer an implementation question.
func (d *Daemon) AnswerForCoder(ctx context.Context, question string,
    thread *Thread) (string, error) {
    // PM ChatWithTools call — PM uses ReadFile, Grep, ReadMemory
    // to research and answer the question
}

// ResolveArtistQuestion asks the PM to answer an Artist question
// before image generation.
func (d *Daemon) ResolveArtistQuestion(ctx context.Context, question string,
    thread *Thread) (string, error) {
    // PM ChatWithTools — reads artist memory, checks existing assets
}

// InjectAnswer resumes the Coder with the PM's answer.
func (d *Daemon) InjectAnswer(ctx context.Context, answer string,
    sessionID string) (*models.CoderResult, error) {
    // Coder.Resume(sessionID, answer)
}
```

#### User Correction at Thread Close

The "behind the scenes" section in the thread summary is not just
informational — it's a **correction opportunity**. If the PM gave
wrong info to the Coder, the user catches it here and corrects it.

```
PM answer was wrong:
  Thread summary shows:
    Coder: "bcrypt or argon2?"
    → PM: "bcrypt — based on auth/login.go:34"

  User: "that's wrong, we're migrating to argon2"
    ↓
  PM: "Got it. Updating memory:
    ❌ Old: 'Auth uses bcrypt (auth/login.go:34)'
    ✅ New: 'Auth migrating from bcrypt to argon2. New code uses argon2.'
    Saved."
```

This makes the autonomous resolution safe. The PM can make mistakes,
but they're always surfaced to the user and correctable. Without the
"behind the scenes" section, wrong PM answers would be invisible —
the user would only see the final result and not know why the Coder
made a bad choice.

**The correction flow:**
1. Thread closes → PM posts summary with "behind the scenes"
2. User reads the exchanges
3. If everything looks right → user says "merge" / ignores
4. If something is wrong → user replies with the correction
5. PM updates the relevant memory file immediately
6. Then proceeds with merge

The daemon listens for user replies after the summary is posted.
If the user replies before saying "merge", the PM processes the
correction. This is a lightweight guardrail — most of the time
the user just glances at the behind-the-scenes and moves on.

#### What This Changes in Previous Sections

- **24.3 Approval Gate, "What Coder Can and Cannot Ask"**: Updated. Coder
  still asks these questions, but PM answers them internally first. Only
  scope/requirements questions reach the user.
- **24.7 Pipeline diagram**: Phase 2 now has a back-channel to Phase 1.
  The PM is "on call" during Coder execution.
- **24.6 Thread Usage Report**: Shows "behind the scenes" with every
  inter-role exchange, plus correction opportunity.
- **Section 16 Memory**: Inter-role exchanges are a primary source of
  "Working with Other Roles" learnings.

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
// See 24.9 for the full PM pool initialization.
// The daemon holds a pool of PM models, not a single one.

func NewDaemon(cfg Config) *Daemon {
    d := &Daemon{
        pmModels:  buildPMPool(cfg),      // map[string]models.ProductManager
        pmDefault: cfg.ProductManager.Default,
        artist:    buildArtist(cfg),       // single artist
        coder:     claude.NewCLICoder(cfg.Coder),
    }
    return d
}

// Getting the PM for a thread:
func (d *Daemon) pmForThread(thread *Thread) models.ProductManager {
    return d.pmModels[thread.ActivePM]
}
```

**Typical config: Kimi default + Claude pro + OpenAI for images:**

```go
    pmModels = {
        "kimi":      openai.NewProductManager(kimiClient, "moonshot-v1-8k", repoDir),
        "claude":    claude.NewCLIProductManager(repoDir, "claude-sonnet-4-5-20250929"),
        "gpt4o-mini": openai.NewProductManager(openaiClient, "gpt-4o-mini", repoDir),
    }
    artist = openai.NewArtist(openaiClient, "gpt-image-1")
    coder  = claude.NewCLICoder(coderCfg)
```

When providers share an API key, the daemon still reuses the same
`*http.Client` across all adapters that share the key.

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
    product_manager.go ← CLI wrapper → models.ProductManager (Claude as PM)
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

func (d *Daemon) runPM(thread *Thread, msg Message) {
    // Phase 1: PM explores the repo autonomously via tools.
    // Uses whichever PM model is active for this thread.
    pm := d.pmForThread(thread)
    pmTools := tools.PMTools(d.repoDir)
    resp, _ := pm.ChatWithTools(ctx, systemPrompt, thread.PMHistory, pmTools)
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

### 24.9 Config — PM Model Pool + Hot Swap

The PM role is not tied to one model. The config defines a **pool of
PM models** with a default. Users can switch between them mid-thread
via a Slack command.

```json
// ~/.codebutler/config.json (global)
{
  "productManager": {
    "default": "kimi",
    "memoryExtraction": "claude",
    "conflictDetection": true,
    "models": {
      "kimi": {
        "provider": "kimi",
        "apiKey": "...",
        "model": "moonshot-v1-8k",
        "label": "Kimi",
        "costPerMToken": 0.02
      },
      "claude": {
        "provider": "claude-cli",
        "model": "claude-sonnet-4-5-20250929",
        "label": "Claude (Pro)",
        "costPerMToken": 3.00
      },
      "gpt4o-mini": {
        "provider": "openai",
        "apiKey": "sk-...",
        "model": "gpt-4o-mini",
        "label": "GPT-4o Mini",
        "costPerMToken": 0.15
      }
    }
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

**Key design decisions:**

- **Default is cheap** (Kimi). 90% of PM work is routine: grep, read,
  propose plan. Kimi handles this fine at $0.001/message.
- **Memory extraction always uses Claude** (`memoryExtraction` field).
  This is the most valuable output — learnings compound. Claude reads
  the conversation + git diff to find patterns. ~$0.02-0.05 per extraction.
  Override to another model if needed, but Claude is strongly recommended.
- **Claude as PM is available for hard tasks.** Complex architecture
  decisions, subtle bugs, cross-cutting refactors — when the user wants
  a smarter brain, they switch. $0.01-0.05/message.
- **Switch is per-thread.** Thread A can use Kimi while Thread B uses
  Claude. They're independent.
- **Switch is instant.** The new model gets the full conversation history.
  No context is lost.
- **Switch back anytime.** Use Claude to think through the hard part,
  switch back to Kimi for the mechanical planning.

#### The `/pm` Command

Users switch PM models mid-conversation with a Slack command:

```
/pm                     → show current PM model + available options
/pm claude              → switch to Claude as PM for this thread
/pm kimi                → switch back to Kimi
/pm gpt4o-mini          → switch to GPT-4o Mini
```

**What happens on switch:**

```
User: /pm claude
PM: "🔄 Switched to *Claude (Pro)* for this thread.
     I have the full conversation so far. What do you need?"
```

The new PM model receives:
1. The full system prompt (same for all PM models)
2. The complete conversation history (all user + PM messages)
3. The tool definitions (same tools available to all PM models)
4. Memory files (same memory available to all PM models)

Nothing is lost. The new model picks up exactly where the old one
left off — it just thinks differently.

#### When to Switch — The User Decides

```
Routine bugfix (stay on Kimi):
  User: "fix the typo in the README"
  PM (Kimi): "Found it. Line 42, 'teh' → 'the'. Say *yes*."
  → No need for Claude. $0.001.

Complex architecture (switch to Claude):
  User: "we need to redesign the auth system to support SSO"
  PM (Kimi): "I see the auth module. There's login.go, session.go,
     middleware.go. What SSO provider?"
  User: /pm claude
  PM (Claude): "🔄 Switched. Looking at the full auth architecture...
     I see three approaches:
     1. SAML-based SSO via auth middleware — minimal changes
     2. OAuth2 + OIDC — more flexible, requires new dependencies
     3. Auth0/Okta integration — fastest but vendor lock-in
     Given your JWT pattern (memory-pm.md) and chi router, I'd
     recommend approach 2. Here's a detailed plan:
     [detailed multi-file plan with trade-offs]
     Say *yes* to start."
  → Claude's deeper reasoning justifies the $0.05 cost.

Switch mid-planning:
  User: "add a notification system"
  PM (Kimi): "I see the project doesn't have notifications yet.
     What kind? Email? Push? In-app?"
  User: "in-app, real-time, with WebSockets"
  User: /pm claude
  PM (Claude): "🔄 Switched. I've read the full conversation.
     Real-time in-app notifications with WebSockets — here's what I'd do:
     [reads existing WebSocket code if any, checks go.mod for ws libs,
      proposes architecture with channels, subscriptions, and persistence]"
  User: "perfect, that's the plan"
  User: /pm kimi
  PM (Kimi): "🔄 Switched back. I have the plan Claude laid out.
     Ready to refine details or start Coder. Say *yes*."
  → Claude designed, Kimi manages execution. Best of both worlds.

Memory extraction (always Claude, no switch needed):
  User: "merge"
  → Memory extraction always uses Claude, regardless of active PM.
    Claude reads the full thread + git diff → proposes learnings.
    No need to switch — Claude handles this by default.
  → The PR summary (description update) uses the active PM.
```

#### Claude as PM — How It Works Technically

Claude as PM uses `claude -p` with the PM system prompt and read-only
permissions. This is fundamentally different from Claude as Coder:

```
Claude as Coder:
  claude -p "<task>" --output-format json --permission-mode bypassPermissions
  → Can edit files, run tests, create PRs
  → Full coding agent with all tools

Claude as PM:
  claude -p "<message>" --output-format json --permission-mode default
  → Read-only: can ReadFile, Grep, list files, git log
  → CANNOT edit, write, delete, or run commands
  → Same restrictions as Kimi — only the brain changes
```

```go
// internal/provider/claude/product_manager.go
//
// Claude CLI as ProductManager — same claude binary, different mode.
// Uses exec.Command like the Coder, but with PM system prompt and
// read-only tools.

type CLIProductManager struct {
    repoDir string
    model   string // "claude-sonnet-4-5-20250929", etc.
}

func NewCLIProductManager(repoDir, model string) models.ProductManager {
    return &CLIProductManager{repoDir: repoDir, model: model}
}

func (c *CLIProductManager) Chat(ctx context.Context, system string,
    messages []models.Message) (string, error) {
    // Build prompt from system + messages
    // Spawn: claude -p "<prompt>" --output-format json --model <model>
    // Parse JSON response
}

func (c *CLIProductManager) ChatWithTools(ctx context.Context, system string,
    messages []models.Message, tools []models.Tool) (string, error) {
    // Claude CLI handles tool-calling natively — no need for our own
    // tool loop. Pass the PM tool definitions and let Claude call them.
    // The tools are still sandboxed read-only by the executor.
}

func (c *CLIProductManager) Name() string {
    return "claude-cli:" + c.model
}
```

**Key insight:** When Claude is the PM, it uses its own built-in tool-calling
which is more sophisticated than the API-based loop. It can reason about
which tools to call, chain them naturally, and produce better plans. This
is why Claude as PM is worth the extra cost for complex tasks.

#### The PM Model Pool

The daemon initializes ALL configured PM models at startup and keeps
them ready. Switching is just changing a pointer — no initialization cost.

```go
// internal/daemon/daemon.go

type Daemon struct {
    // PM model pool — all initialized at startup
    pmModels    map[string]models.ProductManager // "kimi", "claude", "gpt4o-mini"
    pmDefault   string                           // config default

    // Per-thread active PM
    // (stored in thread state, not daemon-level)

    artist      models.Artist
    coder       models.Coder
    // ...
}

func NewDaemon(cfg Config) *Daemon {
    d := &Daemon{
        pmModels:  make(map[string]models.ProductManager),
        pmDefault: cfg.ProductManager.Default,
    }

    // Initialize all configured PM models
    for name, pmCfg := range cfg.ProductManager.Models {
        switch pmCfg.Provider {
        case "kimi":
            client := openai.NewKimi(pmCfg.APIKey)
            d.pmModels[name] = openai.NewProductManager(client, pmCfg.Model, cfg.RepoDir)
        case "openai":
            client := openai.NewOpenAI(pmCfg.APIKey)
            d.pmModels[name] = openai.NewProductManager(client, pmCfg.Model, cfg.RepoDir)
        case "claude-cli":
            d.pmModels[name] = claude.NewCLIProductManager(cfg.RepoDir, pmCfg.Model)
        }
    }

    return d
}
```

#### Thread State — Active PM Model

Each thread tracks which PM model is currently active:

```go
// internal/conflicts/tracker.go (Thread struct additions)

type Thread struct {
    // ... existing fields ...
    ActivePM     string   // current PM model name ("kimi", "claude", etc.)
    PMHistory    []PMSwap // log of model switches for the usage report
}

type PMSwap struct {
    At       time.Time
    From     string // "kimi"
    To       string // "claude"
    Message  int    // message index where the switch happened
}
```

When a thread starts, `ActivePM` is set to `pmDefault`. The `/pm`
command updates it and logs the swap.

#### Thread Usage Report — Multi-Model Breakdown

When a thread uses multiple PM models, the report shows cost per model:

```
📊 *Thread Summary: "redesign auth for SSO"*

*PM*
  Kimi: 3 calls · $0.003 (initial exploration + memory)
  Claude (Pro): 2 calls · $0.08 (architecture design)
  Total PM: $0.083

*Coder*
  Calls: 2 (8 turns) · $1.20

🔍 *Behind the scenes:*
  1. Coder: "SAML or OAuth2 for SSO?"
     → PM (Claude): "OAuth2+OIDC — we decided in the planning phase.
       See the plan step 2."
     ✅ Added to memory.

*Total: $1.283*
```

#### Cost Controls

To prevent accidental expensive usage, the config supports a cost cap:

```json
{
  "productManager": {
    "default": "kimi",
    "models": { ... },
    "costCap": {
      "perThread": 0.50,
      "perDay": 5.00,
      "warnAt": 0.80
    }
  }
}
```

- `perThread`: max PM cost per thread. At cap, PM warns and switches to default.
- `perDay`: max PM cost across all threads. At cap, all threads use default.
- `warnAt`: fraction (0-1) at which PM posts a cost warning in the thread.

```
PM (Claude): "⚠️ PM cost for this thread is $0.40 (cap: $0.50).
  Switching back to Kimi to stay within budget.
  Use /pm claude to override."
```

#### What This Changes

The product manager is no longer a single model — it's a **role with a
pool of brains**. The user picks the right brain for the job:

- **Kimi** (default): fast, cheap, good for routine work. $0.001/msg.
- **Claude** (pro): deep reasoning, complex architecture, subtle bugs. $0.01-0.05/msg.
- **GPT-4o-mini**: middle ground, OpenAI ecosystem. $0.002/msg.
- **DeepSeek, Gemini, local LLM**: future additions — just add to config.

The switch command is deliberately simple (`/pm claude`) because switching
should be frictionless. The user shouldn't have to think about it — they
just notice "this is getting complex" and switch.

When two roles share the same provider and API key, the daemon still
detects this and creates a single shared client automatically.

The artist is **optional**. If not configured, image generation is disabled
and PM tells the user: "Image generation is not configured."

The coder defaults to Claude CLI. It's the only battle-tested option today,
but the interface exists so it can be swapped if needed.

If the default PM model is down, the circuit breaker (section 25) tries
the next configured model before falling back to routing directly to the
Coder.

### 24.10 What This Means for CodeButler's Identity

CodeButler is a **multi-brain orchestrator** with Claude at its core.

```
Default flow (cheap):    message → Kimi PM → plan → Claude Coder → result
Pro flow (when needed):  message → Claude PM → plan → Claude Coder → result
Mixed flow:              message → Kimi PM → /pm claude → Claude PM → plan → Kimi PM → Claude Coder → result
```

The user talks to "the bot" in Slack. They see role prefixes (PM:,
Artist:, Coder:) and can switch the PM brain when they need more
power. The orchestration is visible but not intrusive — like choosing
between a calculator and a whiteboard.

The **Coder is always Claude**. It's the only model that touches code.
The PM (any model) runs **read-only tools** to explore the codebase
but never writes, edits, or deletes anything. The Artist generates
images but never modifies code. The separation of powers is absolute:
only the Coder writes, regardless of which brain the PM uses.

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

With the PM model pool (section 24.9), fallback is natural: if the
active PM model fails, try the next one in the pool.

The fallback chain:
1. **Try active model** — whichever the thread is using (e.g., Kimi)
2. **Try other pool models** — in order of cost (cheapest first)
3. **Skip to Coder** — if all PM models fail, route directly to Coder (v1 behavior)

For the Artist, a separate fallback is still configured:
```json
{
  "artist": {
    "provider": "openai", "model": "gpt-image-1", "apiKey": "sk-...",
    "fallback": { "provider": "stability", "model": "sd3", "apiKey": "sk-..." }
  }
}
```

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
    → PM: summary → PR description updated
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

### What Happens After PR Creation (Memory + Journal)

```go
func (d *Daemon) onPRCreated(threadTS string, prNumber int) {
    scope := d.tracker.Get(threadTS)

    // 1. Add thread URL to PR body (uses active PM — cheap)
    go d.addThreadLinkToPR(threadTS, prNumber)

    // 2. Journal: append "PR opened" entry + commit to branch
    d.journal.Append(threadTS, journal.Entry{
        Type: "pr_opened",
        Content: fmt.Sprintf("PR #%d opened. Changes: ...", prNumber),
    })

    // 3. Extract memory — ALWAYS uses Claude in PM role.
    //    Gets the diff to analyze what actually changed, not just what was discussed.
    go d.extractMemory(threadTS, scope.Branch) // Claude + diff → propose → approve → commit
}

// extractMemory always uses Claude in PM role (d.pmModels["claude"]).
// This is Claude acting AS the PM — same interface, same read-only tools,
// same memory file routing. It's NOT Claude as Coder. The only difference
// vs Kimi-as-PM is a smarter brain doing the analysis.
//
// Why Claude-as-PM for extraction:
// - Claude reads the diff better than any other model
// - Learnings compound — quality here pays dividends in every future thread
// - Cost is ~$0.02-0.05 per extraction — worth it
//
// Falls back to active PM if Claude is not in the pool.
func (d *Daemon) extractMemory(threadTS, branch string) {
    // 1. Get the diff
    diff := d.git.Diff("main", branch) // git diff main...<branch>

    // 2. Get conversation history
    thread := d.tracker.Get(threadTS)
    conversation := d.formatConversation(thread)

    // 3. Get current memory files
    pmMemory := d.readFileFromBranch(branch, "memory-pm.md")
    artistMemory := d.readFileFromBranch(branch, "memory-artist.md")

    // 4. Choose Claude for extraction (always)
    extractor := d.pmModels["claude"]
    if extractor == nil {
        extractor = d.pmForThread(thread) // fallback to active PM
    }

    // 5. Call Claude with conversation + diff + current memory
    prompt := buildExtractionPrompt(conversation, diff, pmMemory, artistMemory)
    result, _ := extractor.Chat(ctx, memoryExtractionSystemPrompt, prompt)

    // 6. Parse JSON ops, format for Slack, post in thread
    ops := parseMemoryOps(result)
    d.postMemoryProposal(threadTS, ops)

    // 7. Wait for user approval (handled by message handler)
    // 8. On approval: commit to branch + push
}
```

### What Happens When User Closes Thread

The user explicitly triggers the close ("merge", "done", "dale"):

```go
func (d *Daemon) onUserClose(threadTS string) {
    scope := d.tracker.Get(threadTS)

    // 1. Generate PR summary → update PR description
    d.updatePRDescription(threadTS, scope.PRNumber)

    // 2. Finalize thread journal: append close entry + cost table,
    //    commit to branch (this is the last commit before merge)
    d.journal.Finalize(threadTS, scope)

    // 3. Post thread usage report (tokens, costs, tips)
    //    See section 24.6 for format details.
    d.postThreadUsageReport(threadTS, scope)

    // 4. Merge the PR (journal is now part of the merge)
    d.github.MergePR(scope.PRNumber) // gh pr merge --squash

    // 5. Notify in thread
    d.slack.SendMessage(scope.ChannelID,
        fmt.Sprintf("PR #%d merged. Thread closed.", scope.PRNumber),
        threadTS)

    // 6. Cleanup: delete remote branch + remove worktree
    d.github.DeleteBranch(scope.Branch)
    worktree.Remove(d.repoDir, scope.Branch)

    // 7. Remove from active tracking
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
8. PR created → memory extraction (always Claude):
       - git diff main..branch → get full diff
       - Claude analyzes thread + diff → proposes learnings → user approves
       - memory.md committed to PR branch + pushed
       - Journal: "PR opened" entry appended + committed
       ↓
9. User: "dale merge" → THREAD CLOSED:
       - PM generates summary → updates PR description via gh pr edit
       - Journal: finalize (close entry + cost table) → committed
       - gh pr merge --squash (journal included in merge)
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

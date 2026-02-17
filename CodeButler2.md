# CodeButler 2

**Status**: Planning (implementation not started)

---

## 1. What is CodeButler

CodeButler is **a multi-role AI dev team accessible from Slack**. Multi-model, multi-role, with persistent memory that improves over time. You describe what you want in a Slack thread. A cheap model (the PM) plans the work, explores the codebase, and proposes a plan. You approve. The Coder executes — with a full agent loop, tool use, file editing, test running, and PR creation. No terminal needed. You can be on your phone.

### 1.1 The Two Loops

**Outer loop — CodeButler orchestration:** Decides WHAT to build and WHEN. The PM talks to the user, explores the repo with read-only tools, proposes a plan, gets approval, spawns the Coder, routes Coder questions back to the PM, extracts memory, and closes the thread.

**Inner loop — Coder's agent loop:** Decides HOW to build it. When the Coder runs, it executes its own agent loop: reading files, writing code, running tests, iterating on errors, committing, and pushing — all autonomously inside a git worktree. CodeButler provides all tools natively (Read, Write, Edit, Bash, Grep, Glob, etc.) and drives the LLM via OpenRouter API. No external CLI dependency.

`maxTurns` and `timeout` (from config) control the inner loop. CodeButler controls the outer loop.

### 1.2 The Three Roles

| Role | What it does | Model | Writes code? | Cost |
|------|-------------|-------|-------------|------|
| **PM** | Plans, explores, routes, extracts memory | Kimi / GPT-4o-mini / Claude (swappable via OpenRouter) | Never | ~$0.001/msg |
| **Artist** | Generates and edits images | OpenAI gpt-image-1 | Never | ~$0.02/img |
| **Coder** | Writes code, runs tests, creates PRs | Claude Opus 4.6 via OpenRouter | Always | ~$0.30-2.00/task |

Separation of powers is absolute: only the Coder writes code (PM has read-only tools), only the PM orchestrates (Coder doesn't know about other threads, memory, or Slack), only the user approves (no code runs without explicit "yes"/"dale"/"go").

### 1.3 What Makes It an Agent

CodeButler receives a message and then: classifies intent, selects a workflow from memory, explores the codebase autonomously with tools, detects conflicts with other active threads, proposes a plan with file:line references, waits for user approval, creates an isolated git worktree, runs the Coder with plan + context, routes Coder questions to the PM, detects PR creation, extracts learnings and proposes memory updates (user approves), merges PR, cleans up worktree, closes thread. Each step involves decisions, tool calls, and state management.

### 1.4 Architecture: OpenRouter + Native Tools (No CLI Dependencies)

**All LLM calls go through OpenRouter.** CodeButler is a standalone Go binary that owns its entire agent loop. It does NOT shell out to `claude` CLI or any external tool. Instead:

- **Coder**: Claude Opus 4.6 via OpenRouter API. CodeButler implements the full agent loop: system prompt → LLM call → tool use → execute tool → append result → next LLM call → repeat until done. All tools (Read, Write, Edit, Bash, Grep, Glob, WebFetch, etc.) are implemented natively in Go, identical to what Claude Code provides.
- **PM**: Any model via OpenRouter (Kimi, GPT-4o-mini, Claude Sonnet, etc.). Same tool-calling loop but with read-only tools only.
- **Artist**: OpenAI gpt-image-1 API directly (not via OpenRouter).

This means CodeButler has **zero dependency on Claude Code CLI**. It IS its own Claude Code — with the same tools, same capabilities, plus Slack integration, memory, multi-role orchestration, and more.

### 1.5 Per-Role System Prompts (MD Files Per Repo)

Each role has its own MD file per repo that defines its system prompt, personality, and available tools. Plus one shared MD for cross-role knowledge and interaction patterns.

```
<repo>/
  .codebutler/
    prompts/
      pm.md           # PM system prompt: personality, workflows, read-only tools
      coder.md        # Coder system prompt: personality, coding tools, restrictions
      artist.md       # Artist system prompt: personality, image gen conventions
      shared.md       # Shared knowledge: project facts, inter-role interaction rules
    memory/
      pm.md           # PM memory: workflows, project knowledge, planning notes
      artist.md       # Artist memory: style prefs, colors, icon conventions
```

**Why per-role MDs instead of CLAUDE.md:**
- Each role has a different personality, different tools, different restrictions
- The Coder's system prompt includes ALL tool definitions (Read, Write, Edit, Bash, Grep, Glob, etc.) — it's essentially a custom Claude Code system prompt
- The PM's system prompt includes only read-only tools + exploration guidelines
- The shared.md contains project-wide knowledge that all roles need
- Users can customize each role independently per project
- No dependency on CLAUDE.md — CodeButler manages its own prompts entirely

**On first run**, CodeButler seeds default prompts. Users can edit them. The PM proposes updates via the memory system.

### 1.6 MCP — Model Context Protocol

Claude Code supports MCP servers natively. Since CodeButler implements the same tool-calling loop, it can also support MCP servers. MCP config is read from `.claude/mcp.json` in the repo (standard location). This lets the Coder connect to databases, APIs, documentation servers, Figma, Linear, Jira, Sentry, etc.

The PM can also use MCP servers (future) for external knowledge during planning.

### 1.7 Why CodeButler Exists

**vs. Claude Code in terminal:** CodeButler wraps Claude Code's capabilities in Slack. PM planning is 100x cheaper (~$0.001 vs ~$0.10). Memory is automated. N parallel threads with isolated worktrees. Audit trail via Slack + PRs.

**vs. Cursor/Windsurf:** Fire-and-forget. Describe → approve → get PR. No IDE needed. Team-native.

**vs. Devin/OpenHands:** Uses Claude's native capabilities but self-hosted. PM-mediated with user approval. Cost-transparent. Memory system improves with every thread. Runs on your machine.

**vs. Simple Slack bots:** They generate text. CodeButler ships code with PRs.

---

## 2. Concept Mapping (WhatsApp → Slack)

| WhatsApp | Slack | Notes |
|----------|-------|-------|
| Group JID | Channel ID | Identifier |
| User JID | User ID | Identifier |
| QR code pairing | OAuth App + Bot Token | Auth |
| whatsmeow events | Slack Socket Mode / Events API | Reception |
| `SendMessage(jid, text)` | `chat.postMessage(channel, text)` | Send text |
| `SendImage(jid, png, caption)` | `files.upload` + message | Send images |
| Bot prefix `[BOT]` | Bot messages have `bot_id` | Slack filters bots natively |
| Voice messages (Whisper) | Audio files in Slack → Whisper | Same flow |

---

## 3. Architecture

```
Slack <-> slack-go SDK <-> Go daemon <-> OpenRouter API <-> Claude/Kimi/etc.
                               |              ↕
                           SQLite DB     git worktrees
                      (messages + sessions) (1 per thread)
```

The daemon is a single Go binary. All LLM calls go through OpenRouter (or direct API for images). The Coder's agent loop runs inside the daemon — no external CLI processes. Tools (Read, Write, Edit, Bash, Grep, Glob, etc.) are executed natively by the daemon in the worktree directory.

---

## 4. Dependencies

### Remove (from v1)
- `go.mau.fi/whatsmeow` and all subdependencies
- `github.com/skip2/go-qrcode`, `github.com/mdp/qrterminal/v3`

### Add
- `github.com/slack-go/slack` — Slack SDK (Socket Mode, Events, Web API)
- OpenRouter HTTP client (custom, in `internal/provider/openrouter/`)
- OpenAI HTTP client (for image gen — direct, not via OpenRouter)

### System Requirements
- `gh` (GitHub CLI) — required for PR operations. Must be installed and `gh auth login` done.

---

## 5. Slack App Setup

Create a Slack App with bot token scopes: `channels:history`, `channels:read`, `chat:write`, `files:read`, `files:write`, `groups:history`, `groups:read`, `reactions:write` (optional), `users:read`. Enable Socket Mode (generates `xapp-...` token). Enable Events (`message.channels`, `message.groups`). Install to Workspace → copy Bot Token (`xoxb-...`).

Required tokens: Bot Token (`xoxb-...`) for API ops, App Token (`xapp-...`) for Socket Mode.

---

## 6. Config

Two levels. Global holds shared keys, per-repo holds channel-specific settings.

**Global** (`~/.codebutler/config.json`):
```json
{
  "slack": { "botToken": "xoxb-...", "appToken": "xapp-..." },
  "openrouter": { "apiKey": "sk-or-..." },
  "openai": { "apiKey": "sk-..." }
}
```

**Per-repo** (`<repo>/.codebutler/config.json`):
```json
{
  "slack": { "channelID": "C0123ABCDEF", "channelName": "codebutler-myrepo" },
  "coder": { "model": "anthropic/claude-opus-4-6", "maxTurns": 25, "timeout": 30 },
  "productManager": {
    "default": "kimi",
    "memoryExtraction": "claude",
    "models": {
      "kimi": { "model": "moonshot/moonshot-v1-8k", "label": "Kimi" },
      "claude": { "model": "anthropic/claude-sonnet-4-5-20250929", "label": "Claude (Pro)" },
      "gpt4o-mini": { "model": "openai/gpt-4o-mini", "label": "GPT-4o Mini" }
    }
  },
  "artist": { "provider": "openai", "model": "gpt-image-1" },
  "access": { "maxConcurrentThreads": 5, "maxClaudeCallsPerHour": 20 }
}
```

All LLM calls (PM, Coder, memory extraction) route through OpenRouter using the single `openrouter.apiKey`. The model field uses OpenRouter model IDs (e.g., `anthropic/claude-opus-4-6`). Artist uses OpenAI directly (image gen not available via OpenRouter).

---

## 7. Storage

```
~/.codebutler/
  config.json                    # Global config

<repo>/.codebutler/              # (gitignored)
  config.json                    # Per-repo config
  store.db                       # Messages + sessions (SQLite)
  prompts/                       # Per-role system prompts
    pm.md, coder.md, artist.md, shared.md
  memory/                        # Per-role memory (committed to repo, NOT gitignored)
    pm.md, artist.md
  branches/                      # Git worktrees (1 per active thread)
    fix-login/
    add-2fa/
  images/                        # Generated images
  journals/                      # Thread journals (committed to repo)
```

**Note:** `prompts/` and `memory/` are committed to the repo (not gitignored). Everything else in `.codebutler/` is gitignored.

SQLite tables:

```sql
CREATE TABLE sessions (
    thread_ts   TEXT PRIMARY KEY,
    channel_id  TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    thread_ts  TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    from_user  TEXT NOT NULL,
    content    TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    acked      INTEGER DEFAULT 0
);
```

---

## 8. Files to Modify/Create/Delete

### Delete
All of `internal/whatsapp/` (replaced by Slack client).

### Create
- `internal/slack/` — client.go, handler.go, channels.go, snippets.go
- `internal/github/github.go` — PR detection, merge polling, description updates via `gh`
- `internal/journal/journal.go` — thread journal (incremental MD narrative)
- `internal/models/interfaces.go` — ProductManager, Artist, Coder interfaces + types
- `internal/provider/openrouter/client.go` — OpenRouter HTTP client (auth, rate limiting, retries)
- `internal/provider/openrouter/chat.go` — Chat completions with tool-calling support
- `internal/provider/openai/images.go` — Image gen/edit via OpenAI API
- `internal/tools/definition.go` — Tool definitions for PM (read-only) and Coder (full)
- `internal/tools/executor.go` — Sandboxed tool execution (read, write, edit, bash, grep, glob)
- `internal/tools/loop.go` — Provider-agnostic tool-calling loop
- `internal/router/router.go` — Message classifier
- `internal/conflicts/tracker.go` — Thread lifecycle tracking, file overlap detection
- `internal/conflicts/notify.go` — Slack notifications for conflicts
- `internal/worktree/worktree.go` — Create, remove, list, init worktrees
- `internal/worktree/init.go` — Per-platform init (npm ci, pod install, venv, etc.)

### Modify
- `cmd/codebutler/main.go` — Setup wizard for Slack tokens + channel selection
- `internal/config/` — SlackConfig, GlobalConfig, RepoConfig, OpenRouter config
- `internal/daemon/daemon.go` — Replace WhatsApp with Slack, delete state machine, add thread dispatch
- `internal/store/` — Column renames, thread_ts as session key

---

## 9. Coder Architecture: Native Agent Loop via OpenRouter

The Coder is NOT `claude -p`. CodeButler implements the full agent loop natively:

```
1. Build system prompt from coder.md + shared.md + task plan + memory
2. Call OpenRouter API: POST /chat/completions with tools
3. LLM returns tool_calls (e.g., Read, Write, Edit, Bash, Grep)
4. Execute each tool call locally (sandboxed to worktree)
5. Append tool results to conversation
6. Call OpenRouter again
7. Repeat until LLM returns text (no more tool calls) or maxTurns reached
8. Parse final response, extract PR info, return result
```

### Coder Tools (same as Claude Code)

The Coder has ALL the tools Claude Code has, implemented natively in Go:

| Tool | Description | Implementation |
|------|------------|----------------|
| **Read** | Read file contents with line numbers | `os.ReadFile` with line numbering, offset/limit support |
| **Write** | Write/create files | `os.WriteFile` with path validation |
| **Edit** | String replacement in files | Find old_string, replace with new_string, uniqueness check |
| **Bash** | Execute shell commands | `exec.Command` with timeout, output capture, working dir |
| **Grep** | Regex search across files | `exec.Command("rg", ...)` with output limits |
| **Glob** | Find files by pattern | `filepath.Glob` or `doublestar` library |
| **WebFetch** | Fetch URL content | `http.Get` with HTML-to-text conversion |
| **GitCommit** | Commit staged changes | `git add` + `git commit` |
| **GitPush** | Push branch | `git push -u origin <branch>` |
| **GHCreatePR** | Create pull request | `gh pr create` |

**Additional tools beyond Claude Code:**
- **SlackNotify** — Post status updates to the Slack thread during execution
- **ReadMemory** — Access role-specific memory files

### Tool Execution Safety

All tools execute in the worktree directory. Path validation ensures no escape. Write/Edit only within worktree. Bash has configurable timeout. Grep/Glob respect .gitignore.

### Coder System Prompt (`coder.md`)

The per-repo `coder.md` replaces what CLAUDE.md was for Claude Code. It contains:
- Personality and behavior rules
- Full tool documentation (auto-generated from tool definitions)
- Sandbox restrictions (no installs, no escape, no force push)
- Project-specific conventions (test framework, coding style, etc.)
- Build/test commands

The daemon assembles the final prompt: `coder.md` + `shared.md` + task plan + relevant context.

---

## 10. PM Architecture: Read-Only Agent Loop

The PM uses the same OpenRouter API and tool-calling loop, but with read-only tools only:

| Tool | Description |
|------|------------|
| **ReadFile** | Read file contents (capped at 500 lines) |
| **Grep** | Search patterns (max 100 results) |
| **ListFiles** | Glob pattern file search (max 200 results) |
| **GitLog** | Recent commit history (max 50 commits) |
| **GitDiff** | Uncommitted or ref-based changes |
| **ReadMemory** | Access pm.md or artist.md memory |
| **ListThreads** | See active threads and their files |
| **GHStatus** | Check PR/issue status via `gh` |

All tools are sandboxed to the repo, read-only, output-capped. The tool loop runs max 15 iterations.

The PM system prompt comes from per-repo `pm.md` + `shared.md` + memory.

---

## 11. Message Flow — Event-Driven Threads

The v1 state machine (AccumulationWindow, ReplyWindow, convActive, pollLoop) is eliminated. Slack threads provide natural conversation boundaries.

### Thread Dispatch

The daemon maintains a thread registry: `map[string]*ThreadWorker` mapping thread_ts to goroutine workers.

**Main loop** (goroutine principal): receives all Slack events, extracts thread_ts, routes to existing worker or spawns new one. Main never processes messages — it only routes.

**Thread worker** (goroutine per thread): receives messages via buffered channel (cap 100), processes sequentially. Each worker maintains its own Claude session. Workers are ephemeral — they die after 60s of inactivity (goroutines are cheap, ~2KB stack). On death, they notify the registry to remove themselves. Session ID persists in DB, so a new worker can `--resume` via the stored session.

**Panic recovery**: each goroutine is wrapped with `defer recover()` so one thread crashing doesn't affect others or the main loop.

### Message Flow Cases

**Case 1 — New thread, no worker:** Main creates buffered channel, registers in map, spawns goroutine, sends message. Worker starts, waits 3s accumulation window for more messages, processes batch with PM/Coder, saves session ID.

**Case 2 — Existing worker, idle:** Main sends to existing channel. Worker reads immediately, processes.

**Case 3 — Existing worker, busy:** Main sends to channel. Message sits in buffer. When current processing finishes, worker reads pending message as follow-up.

**Case 4 — Worker died, message arrives:** Main sees no entry in registry, creates new worker (Case 1). New worker loads session_id from DB for resume.

### Thread Phases

Every thread goes through: PM first (classify, explore, plan, get approval), then Coder (implement). Some threads never leave PM phase (questions, images).

```go
type ThreadPhase string
const (
    PhasePM    ThreadPhase = "pm"      // PM talking to user
    PhaseCoder ThreadPhase = "coder"   // User approved, Coder working
)
```

### Graceful Shutdown

Main receives SIGINT/SIGTERM → closes all channels (goroutines detect closed channel and terminate) → waits for active workers with timeout → cancels in-flight LLM calls via context → flushes pending memory → closes SQLite → disconnects Slack.

### Message Durability & Recovery

Messages are persisted to SQLite before processing (acked=0). On restart, unacked messages are reprocessed. Session IDs in DB allow resume.

---

## 12. Features that Change

### Bot Prefix → Role Prefix
Slack identifies bots natively. Outgoing messages get role prefix (`*PM:*`, `*Coder:*`, `*Artist:*`) so users know which role is talking.

### Reactions as Feedback
- 👀 when processing starts
- ✅ when done

### Threads = Sessions
Each Slack thread IS a session (1:1). Thread → branch → PR (1:1:1). Multiple threads run concurrently. No global lock, no state machine.

### Code Snippets
Short code (<20 lines) inline as Slack code blocks. Long code (≥20 lines) uploaded as file snippets with syntax highlighting.

---

## 13. Memory System

### Files — One Memory Per Role

```
<repo>/.codebutler/memory/
  pm.md           # Workflows, project knowledge, planning notes, inter-role tips
  artist.md       # Style preferences, colors, icon conventions, inter-role tips
```

The Coder doesn't have a separate memory file — its knowledge goes into `coder.md` (the system prompt) which users maintain. The PM can suggest additions to `coder.md` via the thread summary.

### Git Flow

Memory files follow PR flow: after PR creation, PM extracts learnings → proposes in thread → user approves → committed to PR branch → lands on main with merge. Versioned, reviewable, branch-isolated, conflict-resolved by git.

### Memory Extraction (Always via Claude)

After PR creation, the PM (always Claude for this step, regardless of active PM model) analyzes the full thread conversation + git diff to extract learnings. It proposes updates routed to the right memory file:

- Project facts, planning notes, workflow refinements → `pm.md`
- Visual style, colors, icon conventions → `artist.md`
- Coding conventions → suggested as tip for user to add to `coder.md`

User controls what gets remembered: "yes" saves all, "remove 3" skips item 3, "add: ..." adds custom learning, "no" discards all.

### Inter-Role Learning

Each memory file has a "Working with Other Roles" section. The PM learns what context the Coder needs (e.g., "always mention test framework in plans"). The Artist learns what formats the Coder expects. Over time, roles form a well-coordinated team.

### Workflows Are Living Documents

Workflows live in `pm.md` and evolve per project. Defaults seeded on first run (bugfix, feature, question, etc.). Users can add custom workflows. After each thread, memory extraction proposes workflow refinements.

---

## 14. Thread Lifecycle & Resource Cleanup

### The Rule: 1 Thread = 1 Branch = 1 PR

Non-negotiable. States: `created → working → pr_opened → merged (closed)`. Only the user closes a thread ("merge"/"done"/"dale"). No timeouts, no automatic close.

### After PR Creation
- Add thread URL to PR body
- Detect TODOs/FIXMEs in code, warn in thread
- Journal: append "PR opened" entry
- Memory extraction (Claude): analyze thread + diff → propose learnings → user approves → commit to PR branch

### On User Close
- Generate PR summary → update PR description via `gh pr edit`
- Finalize journal (close entry + cost table) → commit
- Merge PR (`gh pr merge --squash`)
- Delete remote branch, remove worktree
- Post thread usage report (stats, behind-the-scenes, tips)
- Notify overlapping threads to rebase

### Thread Usage Report

Posted at close. Shows: token/cost breakdown per role, tool call stats, "behind the scenes" (every PM↔Coder exchange — what Coder asked, how PM answered), and tips for more efficient prompting. User can correct wrong PM answers → PM updates memory immediately.

---

## 15. Conflict Coordination

### Detection

Three levels: same file overlap, same directory overlap, semantic overlap (PM analyzes). Checked when new thread starts (PM predicts files from message) and after each Coder response (extract modified files from output).

### Merge Order

When multiple threads have open PRs touching overlapping files, PM suggests merge order (smallest first to minimize rebase work). Posts in channel.

### Post-Merge Notifications

When a PR merges, PM notifies other active threads that touch overlapping files to rebase.

---

## 16. Worktree Isolation

Each thread gets its own git worktree in `.codebutler/branches/<branchName>/`. Worktrees share `.git` with root repo — instant creation, minimal disk, full isolation. The Coder runs inside its worktree and has no idea other threads exist.

Worktree is created only when user approves the plan (not during PM planning). Branch name: `codebutler/<slug>`.

### Per-Platform Init

Different project types need different initialization after worktree creation:

| Platform | Init | Build Isolation |
|----------|------|-----------------|
| iOS/Xcode | `pod install` (if needed) | `-derivedDataPath .derivedData` |
| Node.js | `npm ci` | `node_modules/` per worktree |
| Go | Nothing | Module cache is global + safe |
| Python | `venv + pip install` | `.venv/` per worktree |
| Rust | Nothing | `CARGO_TARGET_DIR=.target` |

Init overlaps with PM planning to hide latency. Resource profiles limit concurrent builds (e.g., only 1 Xcode build at a time).

---

## 17. Multi-Model Orchestration via OpenRouter

All LLM calls go through OpenRouter. Single API key, multiple models.

### Cost Structure
```
PM (Kimi, GPT-4o-mini, ...)  = brain   (~$0.001/call via OpenRouter)
Coder (Claude Opus 4.6)       = hands   (~$0.10-1.00/call via OpenRouter)
Artist (gpt-image-1)          = eyes    (~$0.01-0.04/image, direct OpenAI)
Whisper                        = ears    (~$0.006/min, direct OpenAI)
```

### PM Model Pool + Hot Swap

The PM role has a pool of models. Default is cheap (Kimi). Users switch mid-thread with `/pm claude` or `/pm kimi`. The new model gets full conversation history — nothing lost.

Memory extraction always uses Claude (configurable). It's the most valuable output — learnings compound.

### Cost Controls

Per-thread cap, per-day cap, per-user hourly limit. When exceeded, PM warns and switches to default/cheapest model.

---

## 18. Model Interfaces

Three Go interfaces, all swappable:

```go
type ProductManager interface {
    Chat(ctx, system, messages) (string, error)
    ChatJSON(ctx, system, messages, out) error
    ChatWithTools(ctx, system, messages, tools) (string, error)
    Name() string
}

type Artist interface {
    Generate(ctx, req) (*ImageResult, error)
    Edit(ctx, req) (*ImageResult, error)
    Name() string
}

type Coder interface {
    Run(ctx, req) (*CoderResult, error)
    Resume(ctx, sessionID, message) (*CoderResult, error)
    Name() string
}
```

### Provider Implementation

All providers use the shared OpenRouter client:

```go
// internal/provider/openrouter/client.go
type Client struct {
    httpClient  *http.Client
    apiKey      string
    rateLimiter *rate.Limiter
}

func (c *Client) ChatCompletion(ctx, model, messages, tools) (*Response, error)
```

Role adapters are thin wrappers (~30 lines each) that implement the interface and delegate to the OpenRouter client with the configured model ID.

For the **Coder adapter** specifically, the adapter implements the full agent loop:

```go
// internal/provider/openrouter/coder.go
type CoderAdapter struct {
    client   *Client
    model    string    // "anthropic/claude-opus-4-6"
    executor *tools.Executor
}

func (c *CoderAdapter) Run(ctx, req) (*CoderResult, error) {
    // 1. Load system prompt from coder.md + shared.md
    // 2. Build initial messages with task plan
    // 3. Run tool-calling loop:
    //    a. POST /chat/completions with tools
    //    b. Execute returned tool_calls via executor
    //    c. Append results, repeat
    // 4. Track turns, tokens, cost
    // 5. Return CoderResult with session info
}
```

---

## 19. Coder Sandboxing

The Coder's system prompt (from `coder.md`) starts with restrictions:

- MUST NOT install software/packages/dependencies
- MUST NOT leave the worktree directory
- MUST NOT modify system files or env vars
- MUST NOT run destructive commands (rm -rf, git push --force, DROP TABLE)
- If a task requires any of the above, explain and STOP

Allowed: `gh` (PRs, issues), `git` (on own branch only), project build/test tools.

Since CodeButler executes tools itself (not the LLM), it can enforce these restrictions at the tool execution layer — path validation, command filtering, etc. This is stronger than prompt-only sandboxing.

---

## 20. Logging

Single structured log format:

```
2026-02-14 15:04:05 INF  slack connected
2026-02-14 15:04:08 MSG  leandro: "fix the login bug"
2026-02-14 15:04:09 PM   thread 1707.123 → pm responding
2026-02-14 15:04:32 PM   thread 1707.123 → plan proposed
2026-02-14 15:04:40 INF  thread 1707.123 → approved, starting coder
2026-02-14 15:04:41 CLD  thread 1707.123 → coder running (new session)
2026-02-14 15:05:15 CLD  thread 1707.123 → done · 34s · 3 turns · $0.12
2026-02-14 15:05:16 MEM  thread 1707.123 → proposing 2 memory updates
```

Tags: INF, WRN, ERR, DBG, MSG (user message), PM (PM activity), IMG (image gen), CLD (Coder activity), RSP (response sent), MEM (memory ops).

Ring buffer + SSE for web dashboard. No ANSI/TUI — everything plain.

---

## 21. Service Install

Run as system service. macOS: LaunchAgent plist. Linux: systemd user service. Both run in user session (access to tools, keychain, PATH).

```bash
codebutler --install     # generate + load service
codebutler --uninstall   # unload + delete
codebutler --status      # check if running
codebutler --logs        # tail log file
```

Each repo is an independent service with its own WorkingDirectory and log file.

---

## 22. PR Description as Development Journal

PR description IS the history. When PR is created, PM generates summary of the Slack thread and puts it in PR body via `gh pr edit`.

Format: Summary, Changes (bullet list), Decisions, Participants, Slack Thread link. Bidirectional: Slack → PR link, PR → Slack link.

### Thread Journal

Detailed narrative MD committed to PR branch (`.codebutler/journals/thread-<ts>.md`). Built incrementally as thread progresses. Shows everything: PM reading files, internal PM↔Coder exchanges, model switches, cost breakdown. Lands on main with merge.

---

## 23. Knowledge Sharing via Memory + PR Merge

Memory files follow git flow. Thread A's learnings land on main when its PR merges. Thread B (branched after merge) inherits them. Thread C (branched before merge) gets them on next rebase.

No custom sync mechanism — git IS the knowledge transport. Isolation by default. Review gate (visible in PR diff). Audit trail (every learning is a commit).

---

## 24. Error Recovery & Resilience

| Failure | Recovery |
|---------|----------|
| Slack disconnect | Auto-reconnect (slack-go SDK) |
| Coder LLM call hangs | context.WithTimeout → kill, reply "timed out" |
| Coder LLM call fails | Reply with error, session preserved for retry |
| PM model unreachable | Try fallback model → if all fail, route to Coder directly |
| SQLite locked | Busy timeout + retry with backoff |
| Machine reboot | systemd/launchd restarts, unacked messages reprocessed |

### Circuit Breaker
If primary PM fails 3x in a row, switch to fallback for 5 minutes.

### Graceful Shutdown
SIGINT/SIGTERM → stop accepting messages → close channels → wait for active workers (with timeout) → cancel in-flight API calls → flush memory → close SQLite → disconnect Slack.

---

## 25. Access Control & Rate Limiting

Channel membership IS access control. Optional restrictions: allowed users list, max concurrent threads, max calls per hour, max per user, daily cost ceiling.

Four rate limiting layers: Slack API (platform-enforced, 1msg/s), concurrent Coder limit (configurable semaphore, depends on machine), per-user hourly limit, daily cost ceiling.

---

## 26. Testing Strategy

| Package | What to Test |
|---------|-------------|
| `internal/slack/snippets.go` | Code block extraction, size-based routing |
| `internal/tools/executor.go` | Tool execution, sandboxing, output limits |
| `internal/tools/loop.go` | Tool-calling loop, iteration, truncation |
| `internal/conflicts/tracker.go` | File overlap detection, merge ordering |
| `internal/github/github.go` | PR detection, merge polling |
| `internal/provider/openrouter/` | API client, request/response mapping, error handling |
| `internal/worktree/` | Create, init, remove, isolation |

Integration tests with mock OpenRouter responses. End-to-end with real Slack workspace (manual).

---

## 27. Migration Path: v1 → v2

Phase 1: Slack client + basic messaging (replace WhatsApp)
Phase 2: Thread dispatch + worktrees (replace state machine)
Phase 3: OpenRouter integration + native agent loop (replace `claude -p`)
Phase 4: PM tools + memory system
Phase 5: Artist integration + image flow
Phase 6: Conflict detection + merge coordination

---

## 28. Decisions Made

- [x] Threads = Sessions (1:1 mapping)
- [x] No state machine — event-driven thread dispatch
- [x] Concurrent threads with goroutine-per-thread model
- [x] **OpenRouter for all LLM calls** (no `claude -p` CLI dependency)
- [x] **CodeButler owns all tools natively** (same as Claude Code + more)
- [x] **Per-role system prompt MDs** (pm.md, coder.md, artist.md, shared.md per repo)
- [x] **No CLAUDE.md dependency** — CodeButler manages its own prompts
- [x] Per-role memory files (pm.md, artist.md) with git flow
- [x] PM model pool with hot swap (`/pm claude`, `/pm kimi`)
- [x] Memory extraction always uses Claude (best brain for compounding learnings)
- [x] Thread = Branch = PR (1:1:1, non-negotiable)
- [x] User closes thread explicitly (no timeouts)
- [x] Worktree isolation (one per thread)
- [x] Thread journal (incremental narrative, committed to PR branch)
- [x] Thread usage report with "behind the scenes" transparency
- [x] Conflict detection + merge order suggestions
- [x] `gh` CLI for all GitHub operations
- [x] Buffered channels (cap 100) for non-blocking main loop
- [x] Goroutine panic recovery (defer recover in worker wrapper)
- [x] 60s inactivity timeout for worker goroutines (ephemeral, ~2KB stack)

---

## 29. v1 vs v2 Comparison

| Aspect | v1 (WhatsApp) | v2 (Slack + OpenRouter) |
|--------|---------------|------------------------|
| Platform | WhatsApp (whatsmeow) | Slack (Socket Mode) |
| LLM execution | `claude -p` subprocess | OpenRouter API (native agent loop) |
| Tools | Delegated to Claude Code | Owned by CodeButler (Read, Write, Edit, Bash, etc.) |
| System prompts | CLAUDE.md | Per-role MDs (pm.md, coder.md, artist.md, shared.md) |
| Parallelism | 1 conversation at a time | N concurrent threads (goroutine per thread) |
| State machine | ~300 lines, 4 states, 3 timers | None (event-driven dispatch) |
| Goroutines | 1 (poll loop, permanent) | N (ephemeral, one per active thread) |
| Isolation | Shared directory | Git worktrees (1 per thread) |
| Session key | Chat JID | thread_ts |
| Memory | None | Role-specific, git flow, user-approved |
| Knowledge sharing | Local CLAUDE.md | Memory files via PR merge |
| UX | Flat chat, `[BOT]` prefix | Structured threads, native bot identity |
| Team support | Single user | Multi-user native |
| Authentication | QR code + phone linking | Bot token (one-time setup) |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |

# CodeButler 2

**Status**: Planning (implementation not started)

---

## 1. What is CodeButler

CodeButler is **a multi-role AI dev team accessible from Slack**. Multi-model, multi-role, with persistent memory that improves over time. You describe what you want in a Slack thread. A cheap model (the PM) plans the work, explores the codebase, and proposes a plan. You approve. The Coder executes — with a full agent loop, tool use, file editing, test running, and PR creation. No terminal needed. You can be on your phone.

### 1.1 The Two Loops

**Outer loop — CodeButler orchestration:** Decides WHAT to build and WHEN. The PM talks to the user, explores the repo with read-only tools, proposes a plan, gets approval, spawns the Coder, routes Coder questions back to the PM. At close, the Lead runs retrospective (summary, memory extraction, per-role improvements).

**Inner loop — Coder's agent loop:** Decides HOW to build it. When the Coder runs, it executes its own agent loop: reading files, writing code, running tests, iterating on errors, committing, and pushing — all autonomously inside a git worktree. CodeButler provides all tools natively (Read, Write, Edit, Bash, Grep, Glob, etc.) and drives the LLM via OpenRouter API. No external CLI dependency.

CodeButler controls the outer loop. The inner loop is self-contained within the Coder's agent implementation.

### 1.2 The Four Roles

| Role | What it does | Model | Writes code? | Cost |
|------|-------------|-------|-------------|------|
| **PM** | Plans, explores, routes | Kimi / GPT-4o-mini / Claude (swappable via OpenRouter) | Never | ~$0.001/msg |
| **Artist** | Generates and edits images | OpenAI gpt-image-1 | Never | ~$0.02/img |
| **Coder** | Writes code, runs tests, creates PRs | Claude Opus 4.6 via OpenRouter | Always | ~$0.30-2.00/task |
| **Lead** | Thread retrospective: summary, memory extraction, improvement proposals per role | Claude Sonnet via OpenRouter | Never | ~$0.01-0.05/thread |

All roles share the same tool set (Read, Write, Edit, Bash, Grep, Glob, etc.) — the restriction is behavioral via system prompts, not structural. The PM's prompt says "only read," the Coder's says "do whatever it takes." Separation of powers: only the Coder writes code, only the PM orchestrates, only the user approves.

### 1.3 What Makes It an Agent

CodeButler receives a message and then: classifies intent, selects a workflow from memory, explores the codebase autonomously with tools, detects conflicts with other active threads, proposes a plan with file:line references, waits for user approval, creates an isolated git worktree, runs the Coder with plan + context, routes Coder questions to the PM, detects PR creation, runs the Lead for retrospective (summary, memory extraction, improvement proposals per role — user approves), merges PR, cleans up worktree, closes thread. Each step involves decisions, tool calls, and state management.

### 1.4 Architecture: OpenRouter + Native Tools (No CLI Dependencies)

**All LLM calls go through OpenRouter.** CodeButler is a standalone Go binary that owns its entire agent loop. It does NOT shell out to `claude` CLI or any external tool. Instead:

- **PM**: Any model via OpenRouter (Kimi, GPT-4o-mini, Claude Sonnet, etc.). Tool-calling loop with read-only tools.
- **Coder**: Claude Opus 4.6 via OpenRouter API. CodeButler implements the full agent loop: system prompt → LLM call → tool use → execute tool → append result → next LLM call → repeat until done. All tools (Read, Write, Edit, Bash, Grep, Glob, WebFetch, etc.) are implemented natively in Go, identical to what Claude Code provides.
- **Lead**: Claude Sonnet via OpenRouter. Runs at thread close with full visibility: user messages, PM planning, Coder tool calls, Artist outputs. Produces summary, memory updates, and per-role improvement proposals.
- **Artist**: OpenAI gpt-image-1 API directly (not via OpenRouter).

This means CodeButler has **zero dependency on Claude Code CLI**. It IS its own Claude Code — with the same tools, same capabilities, plus Slack integration, memory, multi-role orchestration, and more.

### 1.5 Per-Role System Prompts (MD Files Per Repo)

Each role has its own MD file per repo that defines its system prompt, personality, and available tools. Plus one shared MD for cross-role knowledge and interaction patterns.

Under `<repo>/.codebutler/`: `prompts/` has pm.md, coder.md, lead.md, artist.md, shared.md (system prompts per role + shared knowledge). `memory/` has pm.md, lead.md, and artist.md (evolving knowledge per role).

**Why per-role MDs instead of CLAUDE.md:**
- Each role has a different personality, different tools, different restrictions
- The Coder's system prompt includes ALL tool definitions (Read, Write, Edit, Bash, Grep, Glob, etc.) — it's essentially a custom Claude Code system prompt
- The PM's system prompt includes only read-only tools + exploration guidelines
- The Lead's system prompt defines retrospective structure, memory extraction patterns, and cross-role analysis
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

Slack connects via slack-go SDK to the Go daemon, which calls OpenRouter API for all LLM operations (Claude, Kimi, etc.). The daemon stores messages and sessions in SQLite, and manages one git worktree per active thread. The Coder's agent loop runs inside the daemon — no external CLI processes. Tools are executed natively by the daemon in the worktree directory.

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

**Global** (`~/.codebutler/config.json`): Slack tokens (botToken, appToken), OpenRouter API key, OpenAI API key (for images).

**Per-repo** (`<repo>/.codebutler/config.json`): Slack channel ID/name, Coder model, PM model pool (default + map of available models with OpenRouter model IDs), Lead model, Artist config, access limits (maxConcurrentThreads, maxCallsPerHour).

All LLM calls (PM, Coder, Lead) route through OpenRouter using a single API key. Model fields use OpenRouter model IDs (e.g., `anthropic/claude-opus-4-6`). Artist uses OpenAI directly (image gen not available via OpenRouter).

---

## 7. Storage

Global: `~/.codebutler/config.json`.

Per-repo: `<repo>/.codebutler/` contains config.json, store.db (SQLite), `prompts/` (pm.md, coder.md, lead.md, artist.md, shared.md), `memory/` (pm.md, lead.md, artist.md), `branches/` (git worktrees, 1 per active thread), `images/` (generated), `journals/` (thread narratives).

**Note:** `prompts/` and `memory/` are committed to the repo (not gitignored). Everything else in `.codebutler/` is gitignored.

SQLite tables: `sessions` (PK: thread_ts, with channel_id, session_id, updated_at) and `messages` (PK: id, with thread_ts, channel_id, from_user, content, timestamp, acked flag).

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

The Coder is NOT `claude -p`. CodeButler implements the full agent loop natively: build system prompt → call OpenRouter with tools → execute returned tool_calls locally → append results → repeat until LLM returns text with no more tool calls. All in Go, no subprocess.

### Tools — Shared Across All Roles

CodeButler implements ALL the tools Claude Code has, natively in Go. **All roles can use all tools** — the difference is what the system prompt allows, not what's technically available:

**Core tools** (same as Claude Code): Read, Write, Edit, Bash, Grep, Glob, WebFetch.

**Git/GitHub tools:** GitCommit, GitPush, GHCreatePR, GHStatus.

**CodeButler-specific tools:** SlackNotify (post to thread), ReadMemory (access role memory), ListThreads (see active threads), GenerateImage / EditImage (Artist-specific).

Each role's system prompt defines which tools to use and how. The PM's prompt says "use Read, Grep, Glob, GitLog for exploration — do NOT write files." The Coder's prompt says "use all tools freely to implement the task." The Artist's prompt adds image generation tools. But technically any role could use any tool — the restriction is behavioral, not structural. This keeps the architecture simple (one tool executor, shared by all roles) and allows future flexibility.

### Tool Execution Safety

All tools execute in the worktree directory. Path validation ensures no escape. Write/Edit only within worktree. Bash has configurable timeout. Grep/Glob respect .gitignore.

### Coder System Prompt (`coder.md`)

The per-repo `coder.md` replaces CLAUDE.md. Contains: personality and behavior rules, tool documentation (auto-generated from tool definitions), sandbox restrictions, project-specific conventions, build/test commands. The daemon assembles the final prompt: `coder.md` + `shared.md` + task plan + relevant context.

---

## 10. PM Architecture: Read-Only Agent Loop

The PM uses the same OpenRouter API, tool-calling loop, and tool set as the Coder. The difference is behavioral — the PM's system prompt (`pm.md`) instructs it to only use read-only tools: Read, Grep, Glob, GitLog, GitDiff, ReadMemory, ListThreads, GHStatus. Output-capped, max 15 tool-calling iterations.

The PM system prompt comes from per-repo `pm.md` + `shared.md` + memory.

---

## 11. Lead Architecture: Thread Retrospective

The Lead runs once per thread, at close time (after PR creation, before merge). It receives the **full thread transcript**: user messages, PM planning + tool calls, Coder tool calls + outputs, Artist requests + results, and the git diff. No other role has this complete picture.

**What the Lead produces:**

- **Thread summary** → PR description via `gh pr edit` (replaces PM doing this)
- **Memory extraction** → proposes updates to pm.md, lead.md, artist.md, and suggests coder.md additions. User approves/edits before commit.
- **Per-role improvement proposals** → "PM should have mentioned X in the plan", "Coder wasted 3 turns on Y because the system prompt doesn't mention Z", "Artist generated wrong size because the prompt doesn't specify dimensions"
- **Thread usage report** → token/cost breakdown, tool call stats, behind-the-scenes (PM↔Coder exchanges), tips for better prompting
- **Journal finalization** → close entry + cost table, committed to PR branch

The Lead uses Claude Sonnet (smart enough to analyze cross-role patterns, cheaper than Opus). Its system prompt comes from `lead.md` + `shared.md` + its own memory (`memory/lead.md`). Over time, the Lead learns what makes threads efficient and what patterns to flag.

---

## 12. Message Flow — Event-Driven Threads

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

Every thread goes through: PM first (classify, explore, plan, get approval), then Coder (implement), then Lead (retrospective at close). Some threads never leave PM phase (questions, images). Three phases: `pm` (PM talking to user), `coder` (user approved, Coder working), `lead` (thread closing, retrospective running).

### Graceful Shutdown

Main receives SIGINT/SIGTERM → closes all channels (goroutines detect closed channel and terminate) → waits for active workers with timeout → cancels in-flight LLM calls via context → flushes pending memory → closes SQLite → disconnects Slack.

### Message Durability & Recovery

Messages are persisted to SQLite before processing (acked=0). On restart, unacked messages are reprocessed. Session IDs in DB allow resume.

---

## 13. Features that Change

### Bot Prefix → Role Prefix
Slack identifies bots natively. Outgoing messages get role prefix (`*PM:*`, `*Coder:*`, `*Artist:*`, `*Lead:*`) so users know which role is talking.

### Reactions as Feedback
- 👀 when processing starts
- ✅ when done

### Threads = Sessions
Each Slack thread IS a session (1:1). Thread → branch → PR (1:1:1). Multiple threads run concurrently. No global lock, no state machine.

### Code Snippets
Short code (<20 lines) inline as Slack code blocks. Long code (≥20 lines) uploaded as file snippets with syntax highlighting.

---

## 14. Memory System

### Files — One Memory Per Role

Three memory files: `pm.md` (workflows, project knowledge, planning notes), `lead.md` (retrospective patterns, what makes threads efficient, cross-role coordination insights), and `artist.md` (style preferences, colors, icon conventions). The Coder doesn't have a separate memory file — its knowledge goes into `coder.md` (the system prompt) which users maintain. The Lead suggests additions to `coder.md` during retrospective.

### Git Flow

Memory files follow PR flow: after PR creation, the Lead extracts learnings → proposes in thread → user approves → committed to PR branch → lands on main with merge. Versioned, reviewable, branch-isolated, conflict-resolved by git.

### Memory Extraction (Lead Role)

After PR creation, the Lead analyzes the full thread transcript (user messages, PM planning, Coder tool calls, Artist outputs, git diff). It proposes updates routed to the right memory file:

- Planning workflows, project knowledge → `pm.md`
- Retrospective patterns, cross-role coordination → `lead.md`
- Visual style, colors, icon conventions → `artist.md`
- Coding conventions → suggested as tip for user to add to `coder.md`

User controls what gets remembered: "yes" saves all, "remove 3" skips item 3, "add: ..." adds custom learning, "no" discards all.

### Inter-Role Learning

Each memory file has a "Working with Other Roles" section. The PM learns what context the Coder needs (e.g., "always mention test framework in plans"). The Artist learns what formats the Coder expects. Over time, roles form a well-coordinated team.

### Workflows Are Living Documents

Workflows live in `pm.md` and evolve per project. Defaults seeded on first run (bugfix, feature, question, etc.). Users can add custom workflows. After each thread, memory extraction proposes workflow refinements.

---

## 15. Thread Lifecycle & Resource Cleanup

### The Rule: 1 Thread = 1 Branch = 1 PR

Non-negotiable. States: `created → working → pr_opened → merged (closed)`. Only the user closes a thread ("merge"/"done"/"dale"). No timeouts, no automatic close.

### After PR Creation
- Add thread URL to PR body
- Detect TODOs/FIXMEs in code, warn in thread
- Journal: append "PR opened" entry
- Lead retrospective: analyze full thread → propose memory updates + per-role improvements → user approves → commit to PR branch

### On User Close
- Lead runs: generates PR summary → `gh pr edit`, finalizes journal, posts usage report with per-role improvement proposals
- User approves memory updates → committed to PR branch
- Merge PR (`gh pr merge --squash`)
- Delete remote branch, remove worktree
- Notify overlapping threads to rebase

### Thread Usage Report (Lead)

Posted by the Lead at close. Shows: token/cost breakdown per role, tool call stats, "behind the scenes" (every PM↔Coder exchange), per-role improvement proposals, and tips for better prompting. User can correct wrong PM answers → Lead updates memory immediately.

---

## 16. Conflict Coordination

### Detection

Three levels: same file overlap, same directory overlap, semantic overlap (PM analyzes). Checked when new thread starts (PM predicts files from message) and after each Coder response (extract modified files from output).

### Merge Order

When multiple threads have open PRs touching overlapping files, PM suggests merge order (smallest first to minimize rebase work). Posts in channel.

### Post-Merge Notifications

When a PR merges, PM notifies other active threads that touch overlapping files to rebase.

---

## 17. Worktree Isolation

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

## 18. Multi-Model Orchestration via OpenRouter

All LLM calls go through OpenRouter. Single API key, multiple models.

### Cost Structure

PM = brain (~$0.001/call), Coder = hands (~$0.10-1.00/call), Lead = retrospective (~$0.01-0.05/thread), Artist = eyes (~$0.02/image), Whisper = ears (~$0.006/min).

### PM Model Pool + Hot Swap

The PM role has a pool of models. Default is cheap (Kimi). Users switch mid-thread with `/pm claude` or `/pm kimi`. The new model gets full conversation history — nothing lost.

Memory extraction is the Lead's job (Claude Sonnet). It's the most valuable output — learnings compound.

### Cost Controls

Per-thread cap, per-day cap, per-user hourly limit. When exceeded, PM warns and switches to default/cheapest model.

---

## 19. Model Interfaces

Four Go interfaces, all swappable:

**ProductManager**: Chat (simple text), ChatJSON (parsed JSON), ChatWithTools (tool-calling loop with read-only tools), Name.

**Coder**: Run (execute coding task in worktree), Resume (continue previous session), Name.

**Lead**: Retrospective (full thread transcript + diff → summary, memory proposals, per-role improvements), Name.

**Artist**: Generate (new image from prompt), Edit (modify existing image), Name.

### Provider Implementation

All providers use a shared OpenRouter client (HTTP, auth, rate limiting). Role adapters are thin wrappers (~30 lines each) that implement the interface and delegate to the client with the configured model ID. The Coder adapter implements the full agent loop internally: load system prompt → build messages → run tool-calling loop → track turns/tokens/cost → return result.

---

## 20. Coder Sandboxing

The Coder's system prompt (from `coder.md`) starts with restrictions:

- MUST NOT install software/packages/dependencies
- MUST NOT leave the worktree directory
- MUST NOT modify system files or env vars
- MUST NOT run destructive commands (rm -rf, git push --force, DROP TABLE)
- If a task requires any of the above, explain and STOP

Allowed: `gh` (PRs, issues), `git` (on own branch only), project build/test tools.

Since CodeButler executes tools itself (not the LLM), it can enforce these restrictions at the tool execution layer — path validation, command filtering, etc. This is stronger than prompt-only sandboxing.

---

## 21. Logging

Single structured log format with tags: INF, WRN, ERR, DBG, MSG (user message), PM (PM activity), CLD (Coder activity), LED (Lead activity), IMG (image gen), RSP (response sent), MEM (memory ops). Each line: timestamp + tag + thread ID + description.

Ring buffer + SSE for web dashboard. No ANSI/TUI — everything plain.

---

## 22. Service Install

Run as system service. macOS: LaunchAgent plist. Linux: systemd user service. Both run in user session (access to tools, keychain, PATH). CLI flags: `--install`, `--uninstall`, `--status`, `--logs`. Each repo is an independent service with its own WorkingDirectory and log file.

---

## 23. PR Description as Development Journal

PR description IS the history. When the thread closes, the Lead generates a summary of the full thread and puts it in the PR body via `gh pr edit`.

Format: Summary, Changes (bullet list), Decisions, Participants, Slack Thread link. Bidirectional: Slack → PR link, PR → Slack link.

### Thread Journal

Detailed narrative MD committed to PR branch (`.codebutler/journals/thread-<ts>.md`). Built incrementally as thread progresses. Shows everything: PM reading files, internal PM↔Coder exchanges, model switches, cost breakdown. Lands on main with merge.

---

## 24. Knowledge Sharing via Memory + PR Merge

Memory files follow git flow. Thread A's learnings land on main when its PR merges. Thread B (branched after merge) inherits them. Thread C (branched before merge) gets them on next rebase.

No custom sync mechanism — git IS the knowledge transport. Isolation by default. Review gate (visible in PR diff). Audit trail (every learning is a commit).

---

## 25. Error Recovery & Resilience

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

## 26. Access Control & Rate Limiting

Channel membership IS access control. Optional restrictions: allowed users list, max concurrent threads, max calls per hour, max per user, daily cost ceiling.

Four rate limiting layers: Slack API (platform-enforced, 1msg/s), concurrent Coder limit (configurable semaphore, depends on machine), per-user hourly limit, daily cost ceiling.

---

## 27. Testing Strategy

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

## 28. Migration Path: v1 → v2

Phase 1: Slack client + basic messaging (replace WhatsApp)
Phase 2: Thread dispatch + worktrees (replace state machine)
Phase 3: OpenRouter integration + native agent loop (replace `claude -p`)
Phase 4: PM tools + memory system
Phase 5: Artist integration + image flow
Phase 6: Conflict detection + merge coordination

---

## 29. Decisions Made

- [x] Threads = Sessions (1:1 mapping)
- [x] No state machine — event-driven thread dispatch
- [x] Concurrent threads with goroutine-per-thread model
- [x] **OpenRouter for all LLM calls** (no `claude -p` CLI dependency)
- [x] **CodeButler owns all tools natively** (same as Claude Code + more)
- [x] **Per-role system prompt MDs** (pm.md, coder.md, artist.md, shared.md per repo)
- [x] **No CLAUDE.md dependency** — CodeButler manages its own prompts
- [x] Per-role memory files (pm.md, artist.md) with git flow
- [x] PM model pool with hot swap (`/pm claude`, `/pm kimi`)
- [x] **Lead role** for thread retrospective (summary, memory, per-role improvements)
- [x] Memory extraction by Lead (full thread visibility, not PM)
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

## 30. v1 vs v2 Comparison

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
| Roles | 1 (Claude) | 4 (PM, Coder, Lead, Artist) |
| Memory | None | Role-specific, git flow, Lead-extracted, user-approved |
| Knowledge sharing | Local CLAUDE.md | Memory files via PR merge |
| UX | Flat chat, `[BOT]` prefix | Structured threads, native bot identity |
| Team support | Single user | Multi-user native |
| Authentication | QR code + phone linking | Bot token (one-time setup) |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |

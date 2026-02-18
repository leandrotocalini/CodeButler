# CodeButler 2

**Status**: Planning (implementation not started)

---

## 1. What is CodeButler

CodeButler is **a multi-agent AI dev team accessible from Slack**. One runtime, multiple agents, each with its own personality, context, and memory — all parameterized from the same Go code. You describe what you want in a Slack thread. A cheap agent (the PM) plans the work, explores the codebase, and proposes a plan. You approve. The Coder agent executes — with a full agent loop, tool use, file editing, test running, and PR creation. At close, the Lead agent mediates a retrospective between all agents to improve workflows. No terminal needed. You can be on your phone.

### 1.1 One Daemon, One Instance Per Agent

CodeButler is a single Go binary. You run one instance per agent, each parameterized by role:

```bash
codebutler --role pm          # always running, listens for Slack messages
codebutler --role coder       # activates only when PM sends it a task
codebutler --role reviewer    # activates after Coder finishes, reviews the diff
codebutler --role researcher  # activates only when PM requests research
codebutler --role lead        # activates at thread close
codebutler --role artist      # activates for UI/UX design or image generation
```

Same code, same agent loop (system prompt → LLM call → tool use → execute tool → append result → repeat), different parameters:

- **System prompt** — from `prompts/<role>.md` + `shared.md`
- **Model** — from per-repo config (Kimi for PM, Opus for Coder, Sonnet for Lead, etc.)
- **Memory** — from `memory/<role>.md` (if the agent has one)
- **Tool permissions** — behavioral (system prompt says what to use), not structural

**The PM is always on** — it listens for Slack messages, talks to the user, explores the codebase, orchestrates. All other agents are dormant until someone calls them. The PM sends a task to the Coder, the Coder activates, works, and can message back. When a thread closes, the Lead activates, studies the transcript, and tells the team what to improve.

**Mediation:** when agents disagree or hit a decision they can't make alone, they escalate to the Lead. The Lead is the arbiter — it decides based on the common good and workflow improvement. If neither can resolve it, the Lead asks the user.

### 1.2 The Six Agents

| Agent | What it does | Model | Writes code? | Cost |
|-------|-------------|-------|-------------|------|
| **PM** | Plans, explores codebase, orchestrates, talks to user | Kimi / GPT-4o-mini / Claude (swappable via OpenRouter) | Never | ~$0.001/msg |
| **Researcher** | Subagent: web research on demand (spawned, returns, dies) | Cheap model via OpenRouter (same tier as PM) | Never | ~$0.001/search |
| **Artist** | UI/UX designer: proposes layouts, component structure, UX flows, interaction patterns. Also generates and edits images | Claude Sonnet via OpenRouter (UX reasoning) + OpenAI gpt-image-1 (image gen) | Never | ~$0.02-0.10/task |
| **Coder** | Writes code, runs tests, creates PRs | Claude Opus 4.6 via OpenRouter | Always | ~$0.30-2.00/task |
| **Reviewer** | Code review: reads diff, checks quality/security/tests, sends feedback to Coder | Claude Sonnet via OpenRouter | Never | ~$0.02-0.10/review |
| **Lead** | Thread retrospective: mediates discussion, evolves workflows | Claude Sonnet via OpenRouter | Never | ~$0.01-0.05/thread |

All agents run the same Go code with the same tool set (Read, Write, Edit, Bash, Grep, Glob, WebSearch, WebFetch, etc.) — the restriction is behavioral via system prompts, not structural. The PM's prompt says "explore code, delegate web research," the Researcher's says "search the web, return structured findings," the Artist's says "design UI/UX, propose layouts and interaction patterns," the Coder's says "do whatever it takes." Separation of powers: the PM explores code + orchestrates, the Researcher searches the web, the Artist designs UI/UX, the Coder writes code, the Reviewer checks quality, the Lead reviews and mediates, only the user approves.

### 1.3 What Makes It a Multi-Agent System

Agents are not isolated workers — they communicate. The PM sends a task to the Coder. The Coder asks the PM for missing context. The Lead talks to each agent about what to improve. They can discuss, disagree, and reach consensus — with the Lead mediating and the user having final say.

CodeButler receives a message and then: the PM classifies intent, selects a workflow from `workflows.md`, follows the workflow steps (interviewing the user, exploring codebase, messaging the Researcher for web research), detects conflicts with other active threads, if the feature has a UI component sends requirements to the Artist for UI/UX design (layouts, component structure, UX flows), proposes a plan with file:line references + Artist design, waits for user approval, creates an isolated git worktree, messages the Coder with plan + Artist design as input, routes Coder questions back to the PM, detects PR creation, activates the Reviewer who reviews the diff and sends feedback to the Coder until satisfied, activates the Lead who leads a retrospective discussion with all agents (workflow evolution proposals — user approves), merges PR, cleans up worktree, closes thread. For discovery threads: PM interviews the user, Artist designs UX for visual features, then hands off to the Lead who produces a roadmap.

### 1.4 Architecture: OpenRouter + Native Tools (No CLI Dependencies)

**All LLM calls go through OpenRouter.** CodeButler is a standalone Go binary with a single agent runtime. Each agent is an instance of this runtime with different config. It does NOT shell out to `claude` CLI or any external tool:

- **PM agent**: Any model via OpenRouter (Kimi, GPT-4o-mini, Claude Sonnet, etc.). Read-only codebase tools. Stays alive during the thread.
- **Researcher agent**: Any cheap model via OpenRouter. Spawned on demand — uses WebSearch + WebFetch, synthesizes findings, returns result, dies. Multiple can run in parallel.
- **Coder agent**: Claude Opus 4.6 via OpenRouter API. Full tool set (Read, Write, Edit, Bash, Grep, Glob, etc.) implemented natively in Go. Can message the PM when it needs more context.
- **Reviewer agent**: Claude Sonnet via OpenRouter. Activates after Coder finishes. Reads the diff, checks code quality/security/tests, sends feedback to Coder. Review loop until satisfied.
- **Lead agent**: Claude Sonnet via OpenRouter. Runs at thread close. Messages other agents to discuss improvements, then presents proposals to user.
- **Artist agent**: Dual-model. Claude Sonnet via OpenRouter for UI/UX reasoning (layouts, component structure, UX flows, interaction patterns). OpenAI gpt-image-1 API directly for image generation/editing. Activates when PM sends it a feature to design or when images are needed. Its UI/UX output becomes part of the Coder's task prompt.

This means CodeButler has **zero dependency on Claude Code CLI**. It IS its own Claude Code — with the same tools, same capabilities, plus Slack integration, inter-agent communication, memory, and multi-agent orchestration.

### 1.5 Per-Agent System Prompts (MD Files Per Repo)

Each agent has its own MD file per repo that defines its system prompt, personality, and behavioral rules. Plus one shared MD for cross-agent knowledge and interaction patterns.

Under `<repo>/.codebutler/`: `prompts/` has pm.md, researcher.md, coder.md, reviewer.md, lead.md, artist.md, shared.md (system prompts per agent + shared knowledge). `memory/` has workflows.md (process playbook), pm.md, reviewer.md, lead.md, and artist.md (evolving knowledge per agent). The Researcher and Coder don't have memory files — the Researcher is stateless (each search is self-contained), the Coder's knowledge goes into `coder.md`.

**Why per-agent MDs instead of CLAUDE.md:**
- Each agent has a different personality, different tools, different restrictions
- The Coder's system prompt includes ALL tool definitions (Read, Write, Edit, Bash, Grep, Glob, etc.) — it's essentially a custom Claude Code system prompt
- The PM's system prompt includes only read-only tools + exploration guidelines
- The Researcher's system prompt defines web search strategies and structured output format
- The Reviewer's system prompt defines code review checklist, security patterns, quality standards, and feedback format
- The Lead's system prompt defines retrospective structure, memory extraction patterns, mediation rules, and cross-agent analysis
- The shared.md contains project-wide knowledge that all agents need
- Users can customize each agent independently per project
- No dependency on CLAUDE.md — CodeButler manages its own prompts entirely

**On first run**, CodeButler seeds default prompts. Users can edit them. The Lead proposes updates via the memory system after each thread.

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

Slack connects via slack-go SDK to the daemon. One daemon binary, one running instance per agent (parameterized by `--role`). The PM instance is always on; others activate on demand. All instances share SQLite (messages + sessions) and communicate via inter-agent messaging. Each instance calls OpenRouter API with its configured model. Git worktrees provide isolation per thread. Tools are executed natively — no external CLI processes.

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

Two levels. Global holds secrets (tokens, API keys). Per-repo holds project-specific settings (channel, models per role, limits). The per-repo config is committed to git — it's part of the project, not a secret.

**Global** (`~/.codebutler/config.json`) — gitignored, never committed:

```json
{
  "slack": { "botToken": "xoxb-...", "appToken": "xapp-..." },
  "openrouter": { "apiKey": "sk-or-..." },
  "openai": { "apiKey": "sk-..." }
}
```

**Per-repo** (`<repo>/.codebutler/config.json`) — committed to git:

```json
{
  "slack": { "channelID": "C0123...", "channelName": "codebutler-myproject" },
  "models": {
    "pm": {
      "default": "moonshotai/kimi-k2",
      "pool": {
        "kimi": "moonshotai/kimi-k2",
        "claude": "anthropic/claude-sonnet-4-5-20250929",
        "gpt": "openai/gpt-4o-mini"
      }
    },
    "researcher": { "model": "moonshotai/kimi-k2" },
    "coder": { "model": "anthropic/claude-opus-4-6" },
    "reviewer": { "model": "anthropic/claude-sonnet-4-5-20250929" },
    "lead": { "model": "anthropic/claude-sonnet-4-5-20250929" },
    "artist": {
      "uxModel": "anthropic/claude-sonnet-4-5-20250929",
      "imageModel": "openai/gpt-image-1"
    }
  },
  "limits": { "maxConcurrentThreads": 3, "maxCallsPerHour": 100 }
}
```

All LLM calls (PM, Researcher, Coder, Reviewer, Lead, Artist) route through OpenRouter using the global API key. Model fields use OpenRouter model IDs. Agents that need more than one model define them explicitly in config (e.g., Artist has `uxModel` for UI/UX reasoning via OpenRouter + `imageModel` for image generation via OpenAI directly). The PM has a model pool for hot swap (`/pm claude`, `/pm kimi`); other roles have a single model each (or multiple named models if needed).

---

## 7. Storage

Global: `~/.codebutler/config.json`.

Per-repo: `<repo>/.codebutler/` contains config.json, store.db (SQLite), `prompts/` (pm.md, researcher.md, coder.md, reviewer.md, lead.md, artist.md, shared.md), `memory/` (workflows.md, pm.md, reviewer.md, lead.md, artist.md), `branches/` (git worktrees, 1 per active thread), `images/` (generated), `journals/` (thread narratives).

**Committed to git:** `config.json`, `prompts/`, `memory/`. **Gitignored:** `store.db`, `branches/`, `images/`, `journals/`.

SQLite tables: `sessions` (PK: thread_ts, with channel_id, session_id, updated_at) and `messages` (PK: id, with thread_ts, channel_id, from_user, content, timestamp, acked flag).

---

## 8. Files to Modify/Create/Delete

### Delete
All of `internal/whatsapp/` (replaced by Slack client).

### Create
- `internal/slack/` — client.go, handler.go, channels.go, snippets.go
- `internal/github/github.go` — PR detection, merge polling, description updates via `gh`
- `internal/journal/journal.go` — thread journal (incremental MD narrative)
- `internal/models/interfaces.go` — ProductManager, Researcher, Coder, Lead, Artist interfaces + types
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

**Core tools** (same as Claude Code): Read, Write, Edit, Bash, Grep, Glob.

**Web tools:** WebSearch, WebFetch (primarily used by Researcher).

**Git/GitHub tools:** GitCommit, GitPush, GHCreatePR, GHStatus.

**Inter-agent tools:** SendMessage (any agent→any agent: ask questions, delegate work, discuss improvements), Research (PM→Researcher: delegate web search — shortcut for spawning a Researcher subagent).

**CodeButler-specific tools:** SlackNotify (post to thread), ReadMemory (access agent memory), ListThreads (see active threads), GenerateImage / EditImage (Artist-specific).

Each agent's system prompt defines which tools to use and how. The PM uses Read, Grep, Glob, GitLog for codebase exploration and Research for web queries. The Researcher uses WebSearch + WebFetch exclusively. The Coder uses all tools freely to implement. The Artist uses its UX model for design reasoning and its image model for generation. But technically any agent could use any tool — the restriction is behavioral, not structural. This keeps the architecture simple (one tool executor, shared by all agents) and allows future flexibility.

### Tool Execution Safety

All tools execute in the worktree directory. Path validation ensures no escape. Write/Edit only within worktree. Bash has configurable timeout. Grep/Glob respect .gitignore.

### Inter-Agent Communication — Everything in the Thread

**All inter-agent messages are Slack messages in the same thread.** There is no hidden channel, no behind-the-scenes bus. When `@codebutler.coder` asks `@codebutler.pm` a question, that message appears in the thread. The user sees it in real-time. The thread IS the source of truth.

This means:
- The user can follow every conversation as it happens
- The user can intervene at any point ("actually, use JWT not OAuth")
- The thread is the complete record — no separate journal needed for inter-agent comms
- Scroll up = full audit trail

**Agent identity — each agent has an @mention:**

| Agent | Identity | Display |
|-------|----------|---------|
| PM | `@codebutler.pm` | **PM** |
| Coder | `@codebutler.coder` | **Coder** |
| Reviewer | `@codebutler.reviewer` | **Reviewer** |
| Lead | `@codebutler.lead` | **Lead** |
| Researcher | `@codebutler.researcher` | **Researcher** |
| Artist | `@codebutler.artist` | **Artist** |

One Slack bot app, but each agent posts with its own display name and icon. When an agent addresses another, it @mentions them. The daemon parses the mention and routes to the target agent.

**SendMessage(to, message, waitForReply):**
- `to` — target agent identity (e.g., `"coder"`)
- `message` — posted to the thread as a Slack message from the sending agent, @mentioning the target
- `waitForReply` — if true, sending agent pauses until the target replies in the thread

The daemon only routes — it does NOT orchestrate phase transitions. **Agents drive the flow themselves.** Each agent decides when to pass work forward.

### Natural Conversation Flows

**PM ↔ Coder** — the most frequent conversation. Visible in the thread:

```
@codebutler.pm: @codebutler.coder implement this plan: [plan]
@codebutler.coder: @codebutler.pm the plan says to use REST but this
  project uses GraphQL everywhere. should I adapt?
@codebutler.pm: @codebutler.coder good catch, use GraphQL. here's the
  existing schema: [context]
@codebutler.coder: (continues working...)
```

If they can't agree, either one @mentions the Lead:

```
@codebutler.coder: @codebutler.lead PM wants me to add a new table but I
  think we should extend the existing one. here's why: [reasoning]
@codebutler.lead: @codebutler.pm the Coder has a point — extending the
  existing table avoids a migration. thoughts?
@codebutler.pm: @codebutler.lead agreed, let's go with that
@codebutler.lead: @codebutler.coder extend the existing table, PM agrees
```

**PM → Artist** — PM sends the feature to the Artist for UI/UX design:

```
@codebutler.pm: @codebutler.artist feature: notification settings page.
  requirements: user can toggle per-channel notifications, set quiet
  hours, preview notification style. existing UI uses shadcn/ui + tailwind
@codebutler.artist: @codebutler.pm here's my UX proposal:
  - layout: settings page with tabbed sections (channels, schedule, preview)
  - channel toggles: grouped by category with bulk on/off
  - quiet hours: time range picker with timezone display
  - preview: live notification mock that updates as user changes settings
  - interaction: changes auto-save with subtle toast confirmation
  - mobile: tabs collapse to accordion, time picker uses native input
@codebutler.pm: (incorporates Artist's design into the plan for the Coder)
```

**Coder → Reviewer** — Coder initiates the review when it's done:

```
@codebutler.coder: @codebutler.reviewer PR ready for review.
  branch: codebutler/add-notifications. key changes: [summary]
@codebutler.reviewer: @codebutler.coder 3 issues found:
  1. [security] executor.go:47 — unsanitized input
  2. [test] handler_test.go — missing empty thread_ts test
  3. [quality] daemon.go:120 — duplicated error handler
@codebutler.coder: @codebutler.reviewer fixed all 3, pushed
@codebutler.reviewer: @codebutler.coder approved ✓
```

**Coder ↔ Reviewer disagreement** — escalate to Lead:

```
@codebutler.reviewer: @codebutler.coder this function is too complex, split it
@codebutler.coder: @codebutler.reviewer it's a state machine, splitting makes
  it harder to follow
@codebutler.reviewer: @codebutler.lead disagreement on daemon.go:150 complexity.
  I say split, Coder says it's clearer as one block
@codebutler.lead: @codebutler.reviewer Coder is right — state machines read
  better as one block. @codebutler.coder keep it, but add a comment
  explaining the state transitions
```

**Lead retrospective** — also in the thread, user watches live:

```
@codebutler.lead: @codebutler.pm Coder asked you 3 times about auth. should
  we add an auth-check step to the implement workflow?
@codebutler.pm: @codebutler.lead I didn't know this project uses JWT. add it
  to my memory too
@codebutler.lead: @codebutler.reviewer you missed the unhandled error on
  line 47. add error-return check to your patterns?
@codebutler.reviewer: @codebutler.lead yes, add to reviewer.md
@codebutler.lead: here's what we learned: [proposals for user]
```

The user sees all of this as it happens. They can jump in at any point, correct an agent, add context, or override a decision.

### Escalation Rule

**When two agents disagree → they @mention the Lead.** The Lead reads the thread context, evaluates based on the common good (code quality, team efficiency, workflow improvement), and decides. If the Lead can't decide → it asks the user.

The escalation chain: `@agent ↔ @agent → @lead → user`.

**But the user outranks everyone.** Since all conversation happens in the thread, the user can jump in at any point — correct an agent, override a decision, add context, settle a disagreement directly. When the user speaks, all agents listen. The user overrides the Lead, the Lead overrides individual agents. This is the hierarchy:

```
User (final authority)
  └── Lead (mediator, arbiter between agents)
        ├── PM (orchestrator, talks to user)
        ├── Coder (builder)
        ├── Reviewer (quality gate)
        ├── Researcher (web knowledge)
        └── Artist (UI/UX design + images)
```

If the user says "use REST, not GraphQL" mid-conversation between PM and Coder, both agents adapt immediately. No escalation needed — the user IS the escalation.

### Cost Control

Inter-agent exchanges are capped per thread (configurable). The Lead's retrospective discussion has a turn budget. Agents are instructed to be concise — every message costs tokens and occupies the thread.

Every inter-agent message is a learning signal. The Lead sees all of them in the thread at retrospective time.

### Coder System Prompt (`coder.md`)

The per-repo `coder.md` replaces CLAUDE.md. Contains: personality and behavior rules, tool documentation (auto-generated from tool definitions), sandbox restrictions, project-specific conventions, build/test commands. The daemon assembles the final prompt: `coder.md` + `shared.md` + task plan + relevant context.

---

## 10. PM Architecture: Always-Online Orchestrator

The PM is the primary agent — it's the first to activate on every thread and stays available throughout. Same runtime as every other agent, parameterized with `pm.md` as system prompt, read-only codebase tools, and a cheap model (Kimi by default).

**The PM's job:** talk to the user, explore the codebase, select and follow a workflow from `workflows.md`, delegate web research to Researcher agents (via `SendMessage` or `Research` tool), propose plans, and send tasks to the Coder. When the Coder messages back with questions, the PM answers — from its own knowledge, by exploring the codebase, by asking the user, or by spawning a Researcher.

**Always available:** while the Coder works, the PM is not "dead" — it can receive messages from the Coder (questions, blockers) and respond. The daemon keeps the PM's context alive for the duration of the thread. Output-capped at 15 tool-calling iterations per activation.

The PM system prompt comes from per-repo `pm.md` + `shared.md` + `memory/pm.md` + `memory/workflows.md`. The workflows guide the PM's behavior for each task type.

---

## 11. Researcher Architecture: Subagent for Web Research

The Researcher is a **subagent** — spawned by the PM on demand, runs a short focused loop, returns results, dies. Same pattern as Claude Code's `Task` tool with specialized subagents.

**How it works:** The PM calls a `Research` tool with a structured query (topic, context, what it needs to know). CodeButler spawns the Researcher as a subagent with that query. The Researcher runs its own agent loop: WebSearch → WebFetch → synthesize → return structured findings. The PM receives the synthesized result and continues planning.

**Subagent advantages:**

- **Context protection** — web search results are verbose and noisy. The Researcher synthesizes externally and returns only what's relevant, keeping the PM's context window clean.
- **Parallel research** — the PM can spawn multiple Researchers concurrently (e.g., search API docs + compare two libraries at the same time). Each runs independently.
- **Non-blocking** — the PM can continue exploring the codebase while Researchers search. Results arrive when ready.
- **Independently swappable** — use a model that's good at search synthesis without affecting PM planning quality.

**What the Researcher uses:** WebSearch, WebFetch. No codebase tools — the Researcher doesn't read project files. Its job is purely external knowledge.

**Lifecycle:** Spawned → runs loop → returns result → dies. Stateless (no memory file, no session persistence). Each search is self-contained. Its system prompt comes from `researcher.md` + `shared.md`.

---

## 12. Reviewer Architecture: Code Review Loop

The Reviewer activates when the Coder messages it — not by daemon trigger, but by the Coder itself deciding it's done. The Coder sends `SendMessage("reviewer", "PR ready: [branch, summary]")`. The Reviewer's job: read the diff, catch what the Coder missed. It does NOT write code — it sends feedback to the Coder, who fixes.

### What the Reviewer Checks

- **Code quality** — readability, naming, duplication, dead code, overly complex logic
- **Security** — injection vectors, hardcoded secrets, unsafe patterns (OWASP top 10)
- **Test coverage** — are the new paths tested? Are edge cases covered? Can it run the tests and see if they pass?
- **Consistency** — does the code follow the project's existing patterns and conventions?
- **Best practices** — error handling, resource cleanup, race conditions, performance pitfalls
- **Plan compliance** — does the implementation match what the PM planned and the user approved?

### The Review Loop

```
1. Coder finishes → SendMessage("reviewer", "PR ready: branch, key changes summary")
2. Reviewer reads diff (git diff main...branch)
3. Reviewer runs linters/tests if configured (Bash, read-only intent)
4. If issues → SendMessage("coder", "issues: [list]") → Coder fixes → pushes → Reviewer re-reviews
5. If disagreement on an issue → either escalates to Lead → Lead decides
6. Reviewer approves → SendMessage("lead", "review done, N rounds, summary") → Lead activates
```

The Reviewer is thorough but not adversarial. It's a safety net, not a gatekeeper. It catches the things that slip through when the Coder is focused on making things work — the forgotten null check, the SQL injection in a query builder, the test that only covers the happy path.

### Review Feedback Format

The Reviewer sends structured feedback to the Coder via `SendMessage`:

```
Issues found in PR:
1. [security] internal/tools/executor.go:47 — command string is interpolated
   without sanitization, allows injection via user-supplied path
2. [test] internal/slack/handler_test.go — missing test for message with
   empty thread_ts (edge case that causes nil pointer)
3. [quality] internal/daemon/daemon.go:120 — duplicated error handling block,
   extract to helper
```

The Coder receives this, fixes, and signals completion. Max review rounds configurable (default: 3) — if still issues after 3 rounds, Reviewer summarizes remaining concerns in the PR and the Lead decides during retrospective.

### Reviewer Configuration

Uses Claude Sonnet (same tier as Lead — smart enough to spot issues, cheaper than Opus). Its system prompt comes from `reviewer.md` + `shared.md` + `memory/reviewer.md`. Over time, the Reviewer learns project-specific patterns: "this project always forgets to handle context cancellation" or "SQL queries here always need parameterized inputs."

### Reviewer Memory (`memory/reviewer.md`)

The Reviewer has its own memory file. The Lead updates it during retrospective based on what the Reviewer caught (and what it missed). Patterns accumulate: recurring issues become checklist items, project-specific conventions become review rules.

---

## 13. Lead Architecture: Mediator + Retrospective

The Lead is the team's mediator. It runs at thread close (after PR creation, before merge) with the **full thread transcript**: user messages, PM planning + tool calls, Artist UI/UX designs + image results, Coder tool calls + outputs, Reviewer feedback, inter-agent messages, and the git diff. No other agent has this complete picture.

### What the Lead Does

**Phase 1 — Analysis** (solo): reads the full transcript, identifies friction points, wasted turns, missing context, escalation patterns. Produces draft proposals.

**Phase 2 — Discussion** (multi-agent): the Lead @mentions each relevant agent and discusses what went wrong and what to improve. This is a real conversation — agents respond, explain their reasoning, suggest alternatives:

```
Lead → PM: "The Coder asked you 3 times about auth. Should we add an auth-check
           step to the implement workflow?"
PM → Lead: "I didn't know this project uses JWT. Add it to my memory too."
Lead → Coder: "You spent 5 turns debugging a type mismatch. Would stricter
              TypeScript config help?"
Coder → Lead: "Yes, suggest enabling strict mode in coder.md."
Lead → Reviewer: "You missed the unhandled error on line 47 that the user
                  found manually. Add error-handling check to your patterns?"
Reviewer → Lead: "Yes, add to reviewer.md: always check error returns in Go."
Lead → User: "Here's what we learned: [proposals]"
```

The Lead mediates — its goal is the common good and continuous workflow improvement. It's not adversarial. If the PM says "I couldn't have known that," the Lead doesn't blame — it finds how to prevent it next time.

**Phase 3 — Proposals** (to user): the Lead synthesizes the discussion into concrete proposals, grouped by type.

### What the Lead Produces

- **Thread summary** → PR description via `gh pr edit`
- **Memory updates** → proposes updates to workflows.md, pm.md, reviewer.md, lead.md, artist.md, and suggests coder.md additions. All informed by the discussion with agents.
- **Workflow evolution** → the Lead's most valuable output (see below)
- **Thread usage report** → token/cost breakdown, tool call stats, summary of key exchanges (all already visible in thread), tips for better prompting
- **Journal finalization** → close entry + cost table, committed to PR branch

### Workflow Evolution

The Lead reads the current `workflows.md`, compares it against what actually happened in the thread (including every inter-agent message), and proposes changes. Three types of proposals:

**Add step to existing workflow** — "The Coder asked about database migrations. Add step 2.5 to `refactor` workflow: PM should check for pending migrations before proposing plan." (Confirmed with PM during discussion.)

**Create new workflow** — "This thread was about setting up a new CI pipeline. No workflow matched. Proposed new workflow: `ci-setup` with steps: 1. PM: check current CI config... 2. Researcher: latest best practices for [platform]... 3. Coder: implement..." (Built collaboratively with PM and Coder during discussion.)

**Automate a step** — "In the last 3 `feature` threads, the PM always messaged the Researcher about API rate limits. Make this automatic: when workflow is `feature` and plan mentions external API, always spawn Researcher for rate limit check." (PM confirmed this should be automatic.)

The Lead presents these to the user. The user approves, edits, or rejects. Approved changes get committed to `workflows.md` on the PR branch and land on main with merge.

**The flywheel:** rough workflow → thread runs → friction → Lead discusses with agents → proposes improvement → user approves → better workflow → smoother next thread. Each thread makes the team more efficient.

### Lead Configuration

The Lead uses Claude Sonnet (smart enough to analyze cross-role patterns and mediate discussions, cheaper than Opus). Its system prompt comes from `lead.md` + `shared.md` + `workflows.md` + its own memory (`memory/lead.md`). The retrospective discussion has a configurable turn budget to control cost.

---

## 13. Message Flow — Event-Driven Threads

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

The phase depends on the workflow. Implementation threads go through all five phases. Discovery and question threads may only use `pm` and `lead`.

- **`pm`** — PM talking to user, exploring codebase, planning. Also: PM interviewing for discovery, PM answering questions. If feature has UI → PM sends to Artist for UX design
- **`coder`** — user approved plan, Coder working in worktree. Receives PM plan + Artist UI/UX design as input. PM stays available for Coder questions (inter-agent)
- **`review`** — Coder finished, PR created. Reviewer reads diff, sends feedback, Coder fixes. Loop until approved
- **`lead`** — thread closing, Lead runs retrospective. Discusses improvements with all agents. For discovery: Lead builds roadmap
- **`closed`** — PR merged (or roadmap delivered), worktree cleaned, thread archived

### Graceful Shutdown

Main receives SIGINT/SIGTERM → closes all channels (goroutines detect closed channel and terminate) → waits for active workers with timeout → cancels in-flight LLM calls via context → flushes pending memory → closes SQLite → disconnects Slack.

### Message Durability & Recovery

Messages are persisted to SQLite before processing (acked=0). On restart, unacked messages are reprocessed. Session IDs in DB allow resume.

---

## 14. Features that Change

### Agent Identity
One Slack bot app, six agent identities: `@codebutler.pm`, `@codebutler.coder`, `@codebutler.reviewer`, `@codebutler.researcher`, `@codebutler.lead`, `@codebutler.artist`. Each agent posts with its own display name and icon. Inter-agent messages use @mentions — everything visible in the thread.

### Reactions as Feedback
- 👀 when processing starts
- ✅ when done

### Threads = Sessions
Each Slack thread IS a session (1:1). Thread → branch → PR (1:1:1). Multiple threads run concurrently. No global lock, no state machine.

### Code Snippets
Short code (<20 lines) inline as Slack code blocks. Long code (≥20 lines) uploaded as file snippets with syntax highlighting.

---

## 15. Memory System

### Files

Five memory files, all in `memory/`:

| File | What it holds | Who reads it | Who updates it |
|------|--------------|-------------|---------------|
| `workflows.md` | Process playbook: step-by-step workflows for each task type | PM (selects workflow), Lead (proposes changes) | Lead (via user approval) |
| `pm.md` | Project knowledge, planning notes, inter-agent coordination tips | PM | Lead (via user approval) |
| `reviewer.md` | Code review patterns, recurring issues, project-specific quality rules | Reviewer | Lead (via user approval) |
| `lead.md` | Retrospective patterns, what makes threads efficient, mediation patterns | Lead | Lead (via user approval) |
| `artist.md` | UI/UX patterns, component conventions, layout preferences, interaction patterns, style guide, colors, icon conventions, dimension defaults | Artist | Lead (via user approval) |

The Coder and Researcher don't have memory files — the Coder's knowledge goes into `coder.md` (the system prompt) which users maintain, the Researcher is stateless. The Lead suggests `coder.md` additions during retrospective.

### `workflows.md` — The Process Playbook

Workflows are the team's learned process for each type of task. They live in their own file, separate from role memory, because they're cross-role — a workflow defines what the PM does, what the Researcher investigates, what the Coder builds, what the Lead checks.

**Seeded on first run** with defaults:

```markdown
## implement
The standard workflow. User requests a feature or change. PM interviews
until fully defined, then Coder builds, Reviewer checks, Lead learns.

1. PM: read message, classify as implement
2. PM: interview user — ask clarifying questions until requirements are
   unambiguous (acceptance criteria, edge cases, constraints)
3. PM: explore codebase — find integration points, existing patterns,
   related code (Read, Grep, Glob)
4. PM: if unfamiliar tech/API → Researcher: docs, best practices, examples
5. PM: if feature has UI/visual component → Artist: design UI/UX
   (layouts, component structure, UX flows, interaction patterns).
   Artist returns design proposal to PM
6. PM: propose plan — file:line references, acceptance criteria, estimated
   scope, Artist's UI/UX design (if applicable). Post to thread for
   user review
7. User: approve (or request changes → back to step 2)
8. Coder: implement in worktree — receives PM plan + Artist design as
   input prompt. Write code, write tests, run test suite
9. Coder: create PR
10. Reviewer: review diff — code quality, security, tests, plan compliance
11. Reviewer: if issues → send feedback to Coder → Coder fixes → re-review
12. Reviewer: approved
13. Lead: retrospective — discuss with agents, propose workflow/memory updates
14. User: approve learnings, merge

## discovery
Multi-feature discussion. No code, no worktree, no PR. PM interviews the
user to define specs. Lead produces a roadmap with ordered tasks.

1. PM: read message, classify as discovery
2. PM: lead structured discussion — ask about goals, constraints, priorities,
   user stories. Iterate until user says "that's all"
3. PM: for each feature discussed, if needs external context →
   Researcher: market research, technical feasibility, API availability
4. PM: for features with UI/visual component → Artist: propose
   UX flows, layouts, interaction patterns. Artist output included
   in the proposal
5. PM: produce structured proposals — numbered list, each with:
   summary, user story, acceptance criteria, Artist UI/UX design
   (if applicable), estimated complexity, dependencies
6. PM: present proposals to user for review/refinement
7. User: approve final set of proposals
8. PM → Lead: hand off proposals
9. Lead: create roadmap — order proposals by priority and dependencies,
   group into milestones if applicable. Consider: what blocks what,
   what's highest value, what's quick wins vs big efforts
10. Lead: present roadmap to user
11. User: approve (or reorder/modify)
12. Lead: create GitHub issues (one per task, labeled, ordered) or
    commit roadmap document to repo — user decides format
13. Lead: retrospective on discovery process — propose workflow improvements

Each roadmap item becomes a future `implement` thread. Three ways to start:

1. **User starts manually** — new Slack message: "implement task 3 from
   the notifications roadmap". PM reads the GitHub issue, uses it as
   starting spec, may skip some interview steps (already defined).

2. **User says "start next"** — PM checks the roadmap, picks the next
   unstarted task by priority, starts the implement workflow.

3. **User says "start all"** — PM creates one thread per roadmap item
   (respecting maxConcurrentThreads). Each runs independently with
   its own worktree.

The PM links back: when starting from a roadmap item, it references the
discovery thread and GitHub issue in the plan. The Lead links forward:
when closing an implement thread, it updates the roadmap issue as done.

## bugfix
1. PM: reproduce description, find relevant code (Read, Grep), identify
   root cause hypothesis
2. PM: if external API involved → Researcher: check known issues/changelogs
3. PM: propose fix plan with file:line references
4. User: approve
5. Coder: implement fix, add regression test, run test suite
6. Coder: create PR
7. Reviewer: review diff — verify fix is correct, no regressions introduced,
   test covers the bug
8. Reviewer: if issues → Coder fixes → re-review
9. Lead: retrospective

## question
No code, no worktree, no PR. PM answers directly.

1. PM: explore codebase, answer directly
2. PM: if needs external context → Researcher: search
3. PM: respond to user
4. (No Coder, no Reviewer, no Lead — unless user says "actually implement that")

## refactor
1. PM: analyze current code, identify scope and risk
2. PM: propose refactor plan with before/after structure
3. User: approve
4. Coder: refactor, ensure all existing tests pass, add tests if needed
5. Coder: create PR
6. Reviewer: review diff — verify behavior preservation, no regressions,
   improved structure
7. Reviewer: if issues → Coder fixes → re-review
8. Lead: retrospective
```

**How the PM uses it:** When a message arrives, the PM classifies intent and selects the matching workflow. The workflow steps guide the PM's behavior — which tools to call, when to spawn the Researcher, what to include in the plan, what to tell the Coder. This is how the system gets more structured over time.

**How workflows evolve:** See section 15.5 — the Lead proposes workflow changes based on what it observes.

### Git Flow

Memory files (including `workflows.md`) follow PR flow: after PR creation, the Lead extracts learnings → proposes in thread → user approves → committed to PR branch → lands on main with merge. Versioned, reviewable, branch-isolated, conflict-resolved by git.

### Memory Extraction (Lead Role)

After PR creation, the Lead analyzes the full thread transcript (user messages, PM planning, inter-agent messages, Coder tool calls, Artist outputs, git diff). It discusses improvements with each agent, then proposes updates routed to the right file:

- Workflow refinements (new steps, new workflows, automations) → `workflows.md`
- Project knowledge, inter-agent coordination → `pm.md`
- Code review patterns, recurring issues → `reviewer.md`
- Retrospective patterns, mediation patterns → `lead.md`
- UI/UX patterns, component conventions, visual style, colors → `artist.md`
- Coding conventions → suggested as tip for user to add to `coder.md`

User controls what gets remembered: "yes" saves all, "remove 3" skips item 3, "add: ..." adds custom learning, "no" discards all.

### Message-Driven Learning

Every inter-agent message is a learning signal. If the Coder keeps messaging the PM "what's the auth method?" for feature tasks, the Lead notices and proposes: "add step to `feature` workflow: PM should always check auth requirements before proposing plan." Over time, these questions decrease because the workflows get more complete.

The pattern: **Coder question today → Lead discusses with PM → workflow improvement → no question next time.**

### Inter-Role Learning

Each memory file has a "Working with Other Roles" section. The PM learns what context the Coder needs (e.g., "always mention test framework in plans"). The Artist learns what formats the Coder expects. Over time, roles form a well-coordinated team.

---

## 16. Thread Lifecycle & Resource Cleanup

### The Rule: 1 Thread = 1 Branch = 1 PR

Non-negotiable. States: `created → working → pr_opened → reviewing → approved → merged (closed)`. Only the user closes a thread ("merge"/"done"/"dale"). No timeouts, no automatic close.

### After PR Creation
- Coder adds thread URL to PR body, detects TODOs/FIXMEs, warns in thread
- Coder → Reviewer: "PR ready" (agent-driven handoff)
- Reviewer ↔ Coder: review loop until approved (escalate to Lead if disagreement)
- Reviewer → Lead: "approved" (agent-driven handoff)
- Lead: retrospective — discuss with agents, propose memory/workflow updates → user approves → commit to PR branch

### On User Close
- Lead runs: generates PR summary → `gh pr edit`, finalizes journal, posts usage report with per-role improvement proposals
- User approves memory updates → committed to PR branch
- Merge PR (`gh pr merge --squash`)
- Delete remote branch, remove worktree
- Notify overlapping threads to rebase

### Thread Usage Report (Lead)

Posted by the Lead at close. Shows: token/cost breakdown per agent, tool call stats, summary of key inter-agent exchanges (already visible in the thread, but condensed here), improvement proposals, and tips for better prompting. User can correct wrong PM answers → Lead updates memory immediately.

---

## 17. Conflict Coordination

### Detection

Three levels: same file overlap, same directory overlap, semantic overlap (PM analyzes). Checked when new thread starts (PM predicts files from message) and after each Coder response (extract modified files from output).

### Merge Order

When multiple threads have open PRs touching overlapping files, PM suggests merge order (smallest first to minimize rebase work). Posts in channel.

### Post-Merge Notifications

When a PR merges, PM notifies other active threads that touch overlapping files to rebase.

---

## 18. Worktree Isolation

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

## 19. Multi-Model Orchestration via OpenRouter

All LLM calls go through OpenRouter. Single API key, multiple models.

### Cost Structure

PM = brain (~$0.001/call), Researcher = web eyes (~$0.001/search), Coder = hands (~$0.10-1.00/call), Reviewer = quality gate (~$0.02-0.10/review), Lead = retrospective (~$0.01-0.05/thread), Artist = UI/UX designer + image gen (~$0.02-0.10/task for UX reasoning via Sonnet, ~$0.02/image for gen via OpenAI), Whisper = ears (~$0.006/min).

### PM Model Pool + Hot Swap

The PM role has a pool of models. Default is cheap (Kimi). Users switch mid-thread with `/pm claude` or `/pm kimi`. The new model gets full conversation history — nothing lost.

Memory extraction is the Lead's job (Claude Sonnet). It's the most valuable output — learnings compound.

### Cost Controls

Per-thread cap, per-day cap, per-user hourly limit. When exceeded, PM warns and switches to default/cheapest model.

---

## 20. Agent Interface

One Go interface, shared by all agents:

```go
type Agent interface {
    Run(ctx context.Context, task Task) (Result, error)     // execute a task
    Resume(ctx context.Context, id string) (Result, error)  // resume previous session
    SendMessage(ctx context.Context, msg Message) error     // receive inter-agent message
    Name() string                                            // agent identity
}
```

Each agent instance is the same `AgentRunner` struct parameterized by config (role, model, system prompt, memory, tool permissions). The runner implements the standard loop: load prompt → LLM call → tool use → execute → append → repeat.

**Task** contains: what to do (plan, question, research query), context (relevant files, thread history), workflow step being executed.

**Message** contains: from (sender agent), content (free-form text), waitForReply (sync vs async).

**Result** contains: output text, tool calls made, tokens used, cost, session ID for resume.

### Provider Implementation

All agents use a shared OpenRouter client (HTTP, auth, rate limiting). The `AgentRunner` delegates LLM calls to the client with the configured model ID. Each instance runs its own agent loop: load system prompt → build messages → run tool-calling loop → handle SendMessage from other agents → track turns/tokens/cost → return result.

---

## 21. Coder Sandboxing

The Coder's system prompt (from `coder.md`) starts with restrictions:

- MUST NOT install software/packages/dependencies
- MUST NOT leave the worktree directory
- MUST NOT modify system files or env vars
- MUST NOT run destructive commands (rm -rf, git push --force, DROP TABLE)
- If a task requires any of the above, explain and STOP

Allowed: `gh` (PRs, issues), `git` (on own branch only), project build/test tools.

Since CodeButler executes tools itself (not the LLM), it can enforce these restrictions at the tool execution layer — path validation, command filtering, etc. This is stronger than prompt-only sandboxing.

---

## 22. Logging

Single structured log format with tags: INF, WRN, ERR, DBG, MSG (user message), PM (PM activity), RSH (Researcher activity), CLD (Coder activity), LED (Lead activity), IMG (image gen), RSP (response sent), MEM (memory ops), AGT (inter-agent message). Each line: timestamp + tag + thread ID + description.

Ring buffer + SSE for web dashboard. No ANSI/TUI — everything plain.

---

## 23. Service Install

Run as system service. macOS: LaunchAgent plist. Linux: systemd user service. Both run in user session (access to tools, keychain, PATH). CLI flags: `--install`, `--uninstall`, `--status`, `--logs`. Each repo is an independent service with its own WorkingDirectory and log file.

---

## 24. PR Description as Development Journal

PR description IS the history. When the thread closes, the Lead generates a summary of the full thread and puts it in the PR body via `gh pr edit`.

Format: Summary, Changes (bullet list), Decisions, Participants, Slack Thread link. Bidirectional: Slack → PR link, PR → Slack link.

### Thread Journal

Detailed narrative MD committed to PR branch (`.codebutler/journals/thread-<ts>.md`). Built incrementally as thread progresses. Records what tools agents used, files read/written, model switches, cost breakdown. Inter-agent conversation is already in the Slack thread (the source of truth) — the journal captures the tool-level detail that doesn't appear in Slack. Lands on main with merge.

---

## 25. Knowledge Sharing via Memory + PR Merge

Memory files follow git flow. Thread A's learnings land on main when its PR merges. Thread B (branched after merge) inherits them. Thread C (branched before merge) gets them on next rebase.

No custom sync mechanism — git IS the knowledge transport. Isolation by default. Review gate (visible in PR diff). Audit trail (every learning is a commit).

---

## 26. Error Recovery & Resilience

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

## 27. Access Control & Rate Limiting

Channel membership IS access control. Optional restrictions: allowed users list, max concurrent threads, max calls per hour, max per user, daily cost ceiling.

Four rate limiting layers: Slack API (platform-enforced, 1msg/s), concurrent Coder limit (configurable semaphore, depends on machine), per-user hourly limit, daily cost ceiling.

---

## 28. Testing Strategy

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

## 29. Migration Path: v1 → v2

Phase 1: Slack client + basic messaging (replace WhatsApp)
Phase 2: Thread dispatch + worktrees (replace state machine)
Phase 3: OpenRouter integration + native agent loop (replace `claude -p`)
Phase 4: PM tools + memory system
Phase 5: Artist integration + image flow
Phase 6: Conflict detection + merge coordination

---

## 30. Decisions Made

- [x] Threads = Sessions (1:1 mapping)
- [x] No state machine — event-driven thread dispatch
- [x] Concurrent threads with goroutine-per-thread model
- [x] **OpenRouter for all LLM calls** (no `claude -p` CLI dependency)
- [x] **CodeButler owns all tools natively** (same as Claude Code + more)
- [x] **Per-role system prompt MDs** (pm.md, researcher.md, coder.md, lead.md, artist.md, shared.md per repo)
- [x] **No CLAUDE.md dependency** — CodeButler manages its own prompts
- [x] **Multi-agent architecture** — one daemon binary, one instance per agent, parameterized by role
- [x] **PM always on, others on-demand** — Coder/Reviewer/Researcher/Artist/Lead activate only when called
- [x] **Reviewer agent** — code review loop after Coder, before Lead. Catches security/quality/test issues
- [x] **Discovery workflow** — PM interviews user → Lead builds roadmap → each item becomes future implement thread
- [x] **Thread is source of truth** — all inter-agent messages are Slack messages in the thread, visible in real-time
- [x] **Agent identities** — `@codebutler.pm`, `@codebutler.coder`, `@codebutler.reviewer`, etc. One bot, six identities
- [x] **Agent-driven flow** — agents pass work to each other via @mentions, daemon only routes
- [x] **User outranks everyone** — can jump into any agent conversation, override any decision
- [x] **Escalation hierarchy** — user > Lead > individual agents
- [x] **Artist as UI/UX designer** — dual-model (Sonnet for UX reasoning, OpenAI for images). Designs layouts, UX flows, interaction patterns before Coder implements. Artist output + PM plan = Coder input
- [x] **Multi-model agents** — agents can define multiple models in config (e.g., Artist has uxModel + imageModel)
- [x] **Inter-agent communication** — agents @mention each other in the thread via SendMessage
- [x] **Lead as mediator** — arbitrates agent disagreements, focused on common good + workflow improvement
- [x] **Researcher agent** for web research on PM demand (WebSearch/WebFetch)
- [x] **workflows.md** — standalone process playbook, separate from agent memory, evolved by Lead
- [x] **Escalation-driven learning** — Coder questions today → workflow improvements tomorrow
- [x] Per-agent memory files (workflows.md, pm.md, lead.md, artist.md) with git flow
- [x] Per-repo config committed to git (models per role, channel, limits)
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

## 31. v1 vs v2 Comparison

| Aspect | v1 (WhatsApp) | v2 (Slack + OpenRouter) |
|--------|---------------|------------------------|
| Platform | WhatsApp (whatsmeow) | Slack (Socket Mode) |
| LLM execution | `claude -p` subprocess | OpenRouter API (native agent loop) |
| Tools | Delegated to Claude Code | Owned by CodeButler (Read, Write, Edit, Bash, etc.) |
| System prompts | CLAUDE.md | Per-role MDs (pm.md, researcher.md, coder.md, lead.md, artist.md, shared.md) |
| Parallelism | 1 conversation at a time | N concurrent threads (goroutine per thread) |
| State machine | ~300 lines, 4 states, 3 timers | None (event-driven dispatch) |
| Goroutines | 1 (poll loop, permanent) | N (ephemeral, one per active thread) |
| Isolation | Shared directory | Git worktrees (1 per thread) |
| Session key | Chat JID | thread_ts |
| Agents | 1 (Claude) | 6 (PM, Researcher, Coder, Reviewer, Lead, Artist/Designer) — same binary, parameterized |
| Communication | N/A (single model) | Inter-agent messaging (SendMessage) |
| Config | Flat file, gitignored | Global (secrets) + per-repo (committed to git) |
| Memory | None | Per-agent, git flow, Lead-extracted, user-approved |
| Knowledge sharing | Local CLAUDE.md | Memory files via PR merge |
| UX | Flat chat, `[BOT]` prefix | Structured threads, native bot identity |
| Team support | Single user | Multi-user native |
| Authentication | QR code + phone linking | Bot token (one-time setup) |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |

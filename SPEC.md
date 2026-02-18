# CodeButler — Spec

---

## 1. What is CodeButler

CodeButler is **a multi-agent AI dev team accessible from Slack**. One Go binary, multiple agents, each with its own personality, context, and memory — all parameterized from the same code. You describe what you want in a Slack thread. A cheap agent (the PM) plans the work, explores the codebase, and proposes a plan. You approve. The Coder agent executes — with a full agent loop, tool use, file editing, test running, and PR creation. At close, the Lead agent mediates a retrospective between all agents to improve workflows. No terminal needed. You can be on your phone.

### 1.1 Process Model: Separate Processes, Goroutines Per Thread

**Six independent OS processes**, one per agent. Same Go binary, parameterized by `--role`:

```bash
codebutler --role pm          # always running, listens for Slack
codebutler --role coder       # always running, listens for @codebutler.coder
codebutler --role reviewer    # always running, listens for @codebutler.reviewer
codebutler --role researcher  # always running, listens for @codebutler.researcher
codebutler --role lead        # always running, listens for @codebutler.lead
codebutler --role artist      # always running, listens for @codebutler.artist
```

**Each process:**
1. Connects to Slack via Socket Mode (its own listener)
2. Filters messages: only responds to @mentions directed at it (or user messages for PM)
3. Maintains a **thread registry** (`map[string]*ThreadWorker`) — one goroutine per active thread
4. Executes tools locally in its own process (Read, Write, Bash, etc.)
5. Reads its own MD + `global.md` as system prompt
6. Calls OpenRouter with its configured model

**Communication between agents is 100% via Slack messages.** No IPC, no RPC, no shared memory. When PM needs Coder, it posts `@codebutler.coder implement...` in the thread. The Coder process picks it up from its Slack listener. Same for all agent-to-agent communication.

**No shared database.** The Slack thread is the source of truth for inter-agent communication — what agents say to each other and the user. But each agent also maintains a **local conversation file** in the thread's worktree (`conversations/<role>.json`). This file holds the full back-and-forth with the model: system prompt, tool calls, tool results, intermediate reasoning — most of which never appears in Slack. The model returns many things (tool calls, partial thoughts, retries) that the agent processes internally; only the final curated output gets posted to the thread.

The worktree already maps 1:1 to the thread (via the branch), so conversation files just live there — no separate thread-id directory needed. On restart, agents read active threads from Slack to find unprocessed @mentions, and resume model conversations from the worktree's JSON files. No SQLite needed.

Same agent loop in every process (system prompt → LLM call → tool use → execute → append → repeat), different parameters:
- **System prompt** — from `<role>.md` + `global.md`. One file per agent that IS the system prompt and evolves with learnings
- **Model** — from per-repo config (Kimi for PM, Opus for Coder, Sonnet for others)
- **Tool permissions** — behavioral (system prompt says what to use), not structural

**The PM is the entry point** — it handles all user messages. Other agents are idle until @mentioned. **Mediation:** when agents disagree, they escalate to the Lead. If the Lead can't resolve it, it asks the user.

### 1.2 The Six Agents

| Agent | What it does | Model | Writes code? | Cost |
|-------|-------------|-------|-------------|------|
| **PM** | Plans, explores codebase, orchestrates, talks to user | Kimi / GPT-4o-mini / Claude (swappable) | Never | ~$0.001/msg |
| **Researcher** | Subagent: web research on demand (spawned, returns, dies) | Cheap model (same tier as PM) | Never | ~$0.001/search |
| **Artist** | UI/UX designer + image generation | Claude Sonnet (UX reasoning) + OpenAI gpt-image-1 (images) | Never | ~$0.02-0.10/task |
| **Coder** | Writes code, runs tests, creates PRs | Claude Opus 4.6 | Always | ~$0.30-2.00/task |
| **Reviewer** | Code review: quality, security, tests, plan compliance | Claude Sonnet | Never | ~$0.02-0.10/review |
| **Lead** | Thread retrospective: mediates, evolves workflows and agent MDs | Claude Sonnet | Never | ~$0.01-0.05/thread |

All agents share the same tool set. Separation is behavioral via system prompts: the PM explores + orchestrates, the Researcher searches the web, the Artist designs UI/UX, the Coder builds, the Reviewer checks quality, the Lead mediates. Only the user approves.

### 1.3 End-to-End Flow

PM creates worktree (conversation persistence from the start) → classifies intent → selects workflow from `workflows.md` → interviews user → explores codebase → spawns Researcher for web research if needed → sends to Artist for UI/UX design if feature has visual component → proposes plan (with Artist design) → user approves → sends plan + Artist design to Coder → Coder implements + creates PR → Reviewer reviews diff (loop with Coder until approved) → Lead runs retrospective (discusses with agents, proposes learnings) → user approves learnings → merge PR → cleanup.

For discovery: PM interviews → Artist designs UX for visual features → Lead builds roadmap → GitHub issues.

### 1.4 Architecture: OpenRouter + Native Tools

**All LLM calls go through OpenRouter.** CodeButler implements the full agent loop natively in Go — no `claude` CLI, no subprocess. Each agent is the same runtime with different config.

All tools (Read, Write, Edit, Bash, Grep, Glob, WebSearch, WebFetch, GitCommit, GitPush, GHCreatePR, SendMessage, Research, GenerateImage, etc.) are implemented natively. The Artist is dual-model: Claude Sonnet via OpenRouter for UX reasoning + OpenAI gpt-image-1 directly for image generation.

### 1.5 Agent MDs (System Prompt = Memory)

Each agent has **one MD file** in `<repo>/.codebutler/` that is both its system prompt and its evolving memory. Seeded with defaults on first run, then the Lead appends learnings after each PR — only to agents that need them.

**Each agent MD has three sections:**
1. **Personality + rules** — behavioral instructions, tool permissions (seeded, rarely changes)
2. **Project map** — the project from that agent's perspective (evolves as the project grows)
3. **Behavioral learnings** — how to work better, interact with other agents, avoid past mistakes (from Lead retrospectives or direct user feedback)

This is how agents stay coherent — the Artist never proposes UX wildly different from what exists because its MD contains the current UI state. The Coder knows the conventions because they're in its MD.

Plus two shared files all agents read: `global.md` (shared project knowledge: architecture, tech stack, conventions) and `workflows.md` (process playbook).

### 1.6 MCP — Model Context Protocol

CodeButler implements the same tool-calling loop as Claude Code, so it can support MCP servers. Config from `.claude/mcp.json`. Lets agents connect to databases, APIs, Figma, Linear, Jira, Sentry, etc.

### 1.7 Why CodeButler Exists

**vs. Claude Code:** Slack-native. PM planning 100x cheaper. Automated memory. N parallel threads. Audit trail.
**vs. Cursor/Windsurf:** Fire-and-forget. No IDE needed. Team-native.
**vs. Devin/OpenHands:** Self-hosted. PM-mediated. Cost-transparent. Memory improves per thread.
**vs. Simple Slack bots:** They generate text. CodeButler ships code with PRs.

---

## 2. Slack Integration

### Concept Mapping (v1 → v2)

| WhatsApp | Slack | Notes |
|----------|-------|-------|
| Group JID | Channel ID | Identifier |
| QR code pairing | OAuth App + Bot Token | Auth |
| whatsmeow events | Slack Socket Mode | Reception |
| `SendMessage` | `chat.postMessage` | Send text |
| Bot prefix `[BOT]` | Bot messages have `bot_id` | Native filtering |

### Slack App Setup

Bot token scopes: `channels:history`, `channels:read`, `chat:write`, `files:read`, `files:write`, `groups:history`, `groups:read`, `reactions:write`, `users:read`. Socket Mode enabled. Events: `message.channels`, `message.groups`. Tokens: Bot (`xoxb-...`) + App (`xapp-...`).

---

## 3. Installation

### Install Binary

```bash
go install github.com/leandrotocalini/codebutler/cmd/codebutler@latest
```

### First Run — Automatic Wizard

Run `codebutler --role <any>` in a git repo. The binary detects missing config and triggers the wizard automatically. No separate `init` command.

**Step 1: Global tokens** (only once per machine — `~/.codebutler/config.json` doesn't exist):

1. **Slack app** — guides you through creating the Slack app (scopes, Socket Mode, bot user). Asks for Bot Token (`xoxb-...`) + App Token (`xapp-...`)
2. **OpenRouter** — asks for API key (`sk-or-...`). Used for all LLM calls
3. **OpenAI** — asks for API key (`sk-...`). Used for image generation (Artist) and voice transcription (Whisper). **Required**
4. Saves all tokens to `~/.codebutler/config.json`

**Step 2: Repo setup** (once per repo — `<repo>/.codebutler/` doesn't exist):

1. **Seed `.codebutler/`** — creates folder, copies seed MDs (`pm.md`, `coder.md`, `reviewer.md`, `lead.md`, `artist.md`, `researcher.md`, `global.md`, `workflows.md`), creates `config.json` with default models, creates `artist/assets/`, `branches/`, `images/`
2. **Channel selection** — recommends creating `codebutler-<reponame>`. User can pick an existing channel or accept the recommendation
3. **`.gitignore`** — adds `.codebutler/branches/`, `.codebutler/images/` if not present
4. Saves channel to per-repo `config.json`

**Step 3: Service install** (once per repo):

1. Detects OS (macOS / Linux)
2. Installs 6 services — one per agent, `WorkingDirectory=<repo>`, restart on failure:
   - macOS: 6 LaunchAgent plists (`~/Library/LaunchAgents/codebutler.<repo>.<role>.plist`)
   - Linux: 6 systemd user units (`~/.config/systemd/user/codebutler.<repo>.<role>.service`)
3. Starts all 6 services

**Subsequent repos:** Step 1 is skipped (tokens exist). Only steps 2-3 run. Same Slack app, different channel, 6 new services.

### Service Management

```bash
codebutler start              # start all 6 agents for current repo
codebutler stop               # stop all 6
codebutler status             # show running agents, active threads per agent
codebutler --role <role>      # run single agent in foreground (dev mode)
```

On machine reboot, all services restart automatically. If an agent crashes, the service manager restarts it. The agent reads active threads from Slack and picks up where it left off.

---

## 4. Config & Storage

### Config — Two Levels

**Global** (`~/.codebutler/config.json`) — secrets, never committed:
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
      "pool": { "kimi": "moonshotai/kimi-k2", "claude": "anthropic/claude-sonnet-4-5-20250929", "gpt": "openai/gpt-4o-mini" }
    },
    "researcher": { "model": "moonshotai/kimi-k2" },
    "coder": { "model": "anthropic/claude-opus-4-6" },
    "reviewer": { "model": "anthropic/claude-sonnet-4-5-20250929" },
    "lead": { "model": "anthropic/claude-sonnet-4-5-20250929" },
    "artist": { "uxModel": "anthropic/claude-sonnet-4-5-20250929", "imageModel": "openai/gpt-image-1" }
  },
  "limits": { "maxConcurrentThreads": 3, "maxCallsPerHour": 100 }
}
```

All LLM calls route through OpenRouter. Agents needing multiple models define them explicitly (e.g., Artist has `uxModel` + `imageModel`). PM has a model pool for hot swap (`/pm claude`, `/pm kimi`).

### Storage — `.codebutler/` Folder

```
<repo>/.codebutler/
  config.json                    # Per-repo settings (committed)
  # Agent MDs — each is system prompt + project map + learnings
  pm.md                          # PM agent
  coder.md                       # Coder agent
  researcher.md                  # Researcher agent
  reviewer.md                    # Reviewer agent
  lead.md                        # Lead agent
  artist.md                      # Artist agent
  global.md                      # Shared project knowledge (all agents read)
  workflows.md                   # Process playbook
  artist/
    assets/                      # Screenshots, mockups, visual references
  branches/                      # Git worktrees, 1 per active thread (gitignored)
    <branchName>/                # One worktree per thread
      conversations/             # Agent↔model conversation files
        pm.json                  # PM's full model conversation for this thread
        coder.json               # Coder's full model conversation
        reviewer.json            # ...etc
  images/                        # Generated images (gitignored)
```

**Committed to git:** `config.json`, all `.md` files, `artist/assets/`. **Gitignored:** `branches/` (including conversation files), `images/`.

**Two layers of state:**
1. **Slack thread** — inter-agent messages + user interaction. The public record. Source of truth for what was communicated.
2. **Conversation files** (`conversations/<role>.json`) — agent↔model back-and-forth. Tool calls, results, reasoning, retries. Private to each agent. Lives in the worktree, dies with it.

OpenRouter is stateless — full message history sent on every call from the conversation file. On restart, agents scan active threads for unprocessed @mentions and resume model conversations from their JSON files.

---

## 5. Dependencies

**Remove** (from v1): `whatsmeow`, QR code libs.
**Add**: `github.com/slack-go/slack`, OpenRouter HTTP client, OpenAI HTTP client (image gen).
**Requires**: `gh` CLI (GitHub operations).

---

---

## 7. Inter-Agent Communication

**All inter-agent messages are Slack messages in the same thread.** No hidden bus. The user sees everything in real-time. The thread IS the source of truth.

### Agent Identities

One Slack bot app, six display identities: `@codebutler.pm`, `@codebutler.coder`, `@codebutler.reviewer`, `@codebutler.lead`, `@codebutler.researcher`, `@codebutler.artist`. Each posts with its own display name and icon.

### SendMessage(to, message, waitForReply)

Posted to the thread as a Slack message with @mention. The daemon only routes — **agents drive the flow themselves**.

### Conversation Examples

**PM ↔ Coder:**
```
@codebutler.pm: @codebutler.coder implement this plan: [plan]
@codebutler.coder: @codebutler.pm the plan says REST but this project uses GraphQL. adapt?
@codebutler.pm: @codebutler.coder good catch, use GraphQL. here's the schema: [context]
```

**PM → Artist:**
```
@codebutler.pm: @codebutler.artist feature: notification settings. requirements: [details]
@codebutler.artist: @codebutler.pm UX proposal:
  - layout: tabbed sections (channels, schedule, preview)
  - interaction: auto-save with toast confirmation
  - mobile: tabs collapse to accordion
```

**Coder → Reviewer:**
```
@codebutler.coder: @codebutler.reviewer PR ready. branch: codebutler/add-notifications
@codebutler.reviewer: @codebutler.coder 3 issues: [security] executor.go:47, [test] missing edge case, [quality] duplicated handler
@codebutler.coder: @codebutler.reviewer fixed all 3, pushed
@codebutler.reviewer: @codebutler.coder approved ✓
```

**Disagreement → Lead:**
```
@codebutler.reviewer: @codebutler.lead disagreement on daemon.go:150 complexity
@codebutler.lead: Coder is right — state machines read better as one block. Add a comment.
```

### Escalation Hierarchy

```
User (final authority)
  └── Lead (mediator, arbiter)
        ├── PM (orchestrator)
        ├── Coder (builder)
        ├── Reviewer (quality gate)
        ├── Researcher (web knowledge)
        └── Artist (UI/UX design + images)
```

When two agents disagree → Lead decides. **The user outranks everyone** — can jump in at any point, override any decision. The user IS the escalation.

---

## 8. Agent Architectures

Each agent is an independent OS process with its own Slack listener. All run the same binary. All execute tools locally. All communicate via Slack messages in the thread.

### Agent-Model Conversation (the temp file)

Each agent activation involves multiple round-trips with the model: tool calls, tool results, reasoning steps, retries. **Most of this never reaches Slack.** The full conversation lives in a JSON file per agent per thread:

```
.codebutler/branches/<branchName>/conversations/
  pm.json
  coder.json
  reviewer.json
  ...
```

**What's in the file:** system prompt, every user/assistant message, every tool call + result, every model response — the complete transcript of agent↔model interaction for that thread.

**What goes to Slack:** only the agent's curated output — the plan summary, the question to the user, the "PR ready" message. The agent decides what to post; the model's raw responses are not forwarded verbatim.

**Why this matters:**
- The Coder might make 20 tool calls (read files, write code, run tests, fix errors, retry) before posting "PR ready" in Slack. Those 20 rounds live in `coder.json`, not in the thread.
- On restart, the agent resumes from its conversation file — no context lost, no need to re-read the entire Slack thread.
- The Lead reads Slack thread messages for retrospective (what agents *said*), but could also read conversation files for deeper analysis (what agents *did*).

**Lifecycle:** created when the agent first processes a message in the thread. Lives in the worktree. Archived or deleted when the thread closes and worktree is cleaned up.

### PM — Always-Online Orchestrator

The entry point for all user messages. Talks to user, explores codebase, selects workflow, delegates to other agents via @mentions in the thread. Cheap model (Kimi by default). System prompt: `pm.md` + `global.md` + `workflows.md`. Capped at 15 tool-calling iterations per activation.

The PM's goroutine for a thread stays alive while the Coder works — when the Coder @mentions PM with a question, the PM's Slack listener routes it to that thread's goroutine and responds.

### Researcher — Subagent for Web Research

Listens for @mentions from PM. Runs WebSearch + WebFetch → synthesizes → posts result back in thread. Stateless, parallel-capable (multiple goroutines for concurrent research requests). Protects PM's context from noisy web results. System prompt: `researcher.md` + `global.md`.

### Artist — UI/UX Designer + Image Gen

Dual-model. Listens for @mentions from PM. Claude Sonnet for UX reasoning (layouts, component structure, UX flows). OpenAI gpt-image-1 for image gen/editing. Posts design proposals back in the thread. Reads `artist/assets/` for visual references to stay coherent with existing UI. System prompt: `artist.md` + `global.md`.

### Coder — Builder

Claude Opus 4.6. Listens for @mentions from PM (task) and Reviewer (feedback). Full tool set, executes locally in isolated worktree. Creates PRs. When it needs context, @mentions PM in the thread. When done, @mentions Reviewer. System prompt: `coder.md` + `global.md` + task context from thread.

**Sandboxing:** MUST NOT install packages, leave worktree, modify system files, or run destructive commands. Enforced at tool execution layer (path validation, command filtering) — stronger than prompt-only.

### Reviewer — Code Review Loop

Listens for @mentions from Coder ("PR ready"). Checks: code quality, security (OWASP), test coverage, consistency, plan compliance. Sends structured feedback back to Coder via @mention. Loop until approved (max 3 rounds). When approved, @mentions Lead. Disagreements escalate to Lead. System prompt: `reviewer.md` + `global.md`.

### Lead — Mediator + Retrospective

Listens for @mentions from Reviewer ("approved") or from agents in disagreement. At thread close, reads **full thread transcript** from Slack. Three phases:

1. **Analysis** (solo) — identifies friction, wasted turns, escalation patterns
2. **Discussion** (multi-agent) — @mentions each agent in the thread, discusses improvements
3. **Proposals** (to user) — concrete updates to agent MDs, `global.md`, `workflows.md`

**Produces:** PR description, learnings for agent MDs, workflow evolution, usage report.

**Workflow evolution** — add step, create new workflow, or automate a step. Built collaboratively with agents during discussion.

**The flywheel:** rough workflow → friction → Lead discusses → improvement → user approves → smoother next thread.

System prompt: `lead.md` + `global.md` + `workflows.md`. Turn budget configurable.

---

## 9. Message Flow

No state machine. Slack threads provide natural conversation boundaries. Each agent process handles its own events independently.

### How a Task Flows

```
User posts in Slack thread
  → PM creates worktree + starts conversation file
  → PM plans, explores, proposes
  → PM posts: "@codebutler.coder implement: [plan]"
  → Coder implements in worktree (its own conversation file there too)
  → Coder posts: "@codebutler.pm what auth method?" (question)
  → PM responds
  → Coder posts: "@codebutler.reviewer PR ready: [branch]"
  → Reviewer reads diff, posts feedback
  → (loop until approved)
  → Reviewer posts: "@codebutler.lead review done"
  → Lead reads full thread, runs retrospective
```

Every step is a Slack message. Agents drive the flow themselves via @mentions.

### Thread Phases

- **`pm`** — PM planning. If feature has UI → @mentions Artist
- **`coder`** — Coder working in worktree. PM available for questions
- **`review`** — Reviewer ↔ Coder feedback loop
- **`lead`** — Lead retrospective
- **`closed`** — PR merged, worktree cleaned

---

## 10. Memory System

### One File Per Agent = System Prompt + Memory

| File | What it holds | Who updates it |
|------|--------------|---------------|
| `pm.md` | Personality + rules. **Project map:** features, domains. **Learnings:** interview techniques, what Coder needs | Lead + user |
| `coder.md` | Personality + rules. Tool defs, sandbox. **Project map:** architecture, patterns. **Learnings:** coding patterns | Lead + user |
| `reviewer.md` | Personality + rules. Review checklist. **Project map:** quality hotspots. **Learnings:** recurring issues | Lead + user |
| `lead.md` | Personality + rules. Retrospective structure. **Project map:** efficiency patterns. **Learnings:** mediation strategies | Lead + user |
| `artist.md` | Personality + rules. Design guidelines. **Project map:** UI components, screens, design system. **Learnings:** what Coder needs | Lead + user |
| `researcher.md` | Personality + rules. Search strategies. Stateless — no learnings | Rarely changes |
| `global.md` | Shared project knowledge: architecture, tech stack, conventions, deployment | Lead + user |
| `workflows.md` | Process playbook: step-by-step workflows per task type | Lead + user |
| `artist/assets/` | Screenshots, mockups, visual references | Artist + Lead |

**Learnings only go where needed.** If the Reviewer didn't participate, its MD doesn't change. User approves what gets saved.

### workflows.md — Process Playbook

Seeded on first run:

```markdown
## implement
1. PM: create worktree + conversation file
2. PM: classify as implement
3. PM: interview user (acceptance criteria, edge cases, constraints)
4. PM: explore codebase (integration points, patterns)
5. PM: if unfamiliar tech → Researcher: docs, best practices
6. PM: if UI component → Artist: design UI/UX. Artist returns proposal
7. PM: propose plan (file:line refs, Artist design if applicable)
8. User: approve
9. Coder: implement in worktree (PM plan + Artist design as input)
10. Coder: create PR
11. Reviewer: review diff (quality, security, tests, plan compliance)
12. Reviewer: if issues → Coder fixes → re-review
13. Reviewer: approved
14. Lead: retrospective (discuss with agents, propose learnings)
15. User: approve learnings, merge

## discovery
1. PM: classify as discovery
2. PM: structured discussion (goals, constraints, priorities, user stories)
3. PM: if needs external context → Researcher
4. PM: if UI features → Artist: propose UX flows
5. PM: produce proposals (summary, user story, criteria, Artist design, complexity, dependencies)
6. User: approve proposals
7. PM → Lead: hand off
8. Lead: create roadmap (priority, dependencies, milestones)
9. User: approve roadmap
10. Lead: create GitHub issues or commit roadmap
11. Lead: retrospective

Each roadmap item → future implement thread. Start: manually, "start next", or "start all".

## bugfix
1. PM: find relevant code, root cause hypothesis
2. PM: if external API → Researcher
3. PM: propose fix plan
4. User: approve
5. Coder: fix + regression test
6. Reviewer: review → loop
7. Lead: retrospective

## question
1. PM: explore codebase, answer directly
2. PM: if needs context → Researcher
3. (No Coder, no Reviewer, no Lead — unless user escalates)

## refactor
1. PM: analyze code, propose before/after
2. User: approve
3. Coder: refactor, ensure tests pass
4. Reviewer: review → loop
5. Lead: retrospective
```

### Memory Extraction (Lead)

After PR creation, Lead proposes updates routed to the right file:
- Architecture decisions, shared conventions → `global.md`
- Workflow refinements, new workflows, automations → `workflows.md`
- Agent-specific learnings → the relevant agent's MD
- New UI screenshots → `artist/assets/`
- Coding conventions → `coder.md`

**Project maps evolve:** when a thread adds a screen, changes an API, or introduces a pattern, the Lead updates the relevant agent's project map. User approves.

### Learning Patterns

**Message-driven:** Coder keeps asking PM about auth → Lead proposes workflow step for auth check → no question next time.

**Inter-agent:** Each agent's MD accumulates how to work with other agents. PM learns what Coder needs. Artist learns what detail level Coder expects. Cross-cutting knowledge goes to `global.md`.

### Git Flow

All MDs follow PR flow: Lead proposes → user approves → committed to PR branch → lands on main with merge. Git IS the knowledge transport.

---

## 11. Thread Lifecycle

### 1 Thread = 1 Branch = 1 PR

Non-negotiable. Only the user closes a thread. No timeouts.

### After PR Creation
1. Coder → Reviewer: "PR ready" (agent-driven handoff)
2. Reviewer ↔ Coder: review loop until approved
3. Reviewer → Lead: "approved"
4. Lead: retrospective, proposes learnings → user approves → commit to PR branch

### On User Close
- Lead: PR summary via `gh pr edit`, usage report, journal finalization
- Merge PR (`gh pr merge --squash`)
- Delete remote branch, remove worktree
- Notify overlapping threads to rebase

---

## 12. Conflict Coordination

**Detection:** file overlap, directory overlap, semantic overlap (PM analyzes). Checked at thread start and after each Coder response.

**Merge order:** PM suggests smallest-first when multiple PRs touch overlapping files.

**Post-merge:** PM notifies other active threads to rebase.

---

## 13. Worktree Isolation

Each thread gets a git worktree in `.codebutler/branches/<branchName>/`. **Created early by the PM** — as soon as the PM starts working on a thread, it creates the worktree and begins saving its conversation file there. This way every agent that touches the thread has a place to persist its model conversation from the start. Branch: `codebutler/<slug>`.

| Platform | Init | Build Isolation |
|----------|------|-----------------|
| iOS/Xcode | `pod install` | `-derivedDataPath .derivedData` |
| Node.js | `npm ci` | `node_modules/` per worktree |
| Go | Nothing | Module cache is global |
| Python | `venv + pip install` | `.venv/` per worktree |
| Rust | Nothing | `CARGO_TARGET_DIR=.target` |

---

## 14. Multi-Model Orchestration

All via OpenRouter. PM has model pool with hot swap. Cost controls: per-thread cap, per-day cap, per-user hourly limit. Circuit breaker: 3x PM failure → fallback for 5 minutes.

---

---

## 16. Operational Details

### Slack Features
- Agent identity: one bot, six display names + icons
- Reactions: 👀 processing, ✅ done
- Threads = sessions (1:1). Multiple concurrent
- Code snippets: <20 lines inline, ≥20 lines as file uploads

### Logging
Structured tags: INF, WRN, ERR, DBG, MSG, PM, RSH, CLD, LED, IMG, RSP, MEM, AGT. Ring buffer + SSE for web dashboard.

### PR Description
Lead generates summary at close via `gh pr edit`. Thread journal (`.codebutler/journals/thread-<ts>.md`) captures tool-level detail not visible in Slack.

### Error Recovery

Each process is independent — one crash doesn't affect others.

| Failure | Recovery |
|---------|----------|
| Agent process crashes | Service restarts it. Reads active threads from Slack, processes unresponded @mentions |
| Slack disconnect | Auto-reconnect per process (SDK handles) |
| LLM call hangs | context.WithTimeout per goroutine → kill, reply error in thread |
| LLM call fails | Error reply in thread, session preserved for retry |
| Agent not running | @mention sits in thread. When agent starts, it reads thread and processes |
| Machine reboot | All 6 services restart, each reads active threads from Slack |

### Access Control
Channel membership = access. Optional: allowed users, max concurrent threads, hourly/daily limits.

---

---

## 18. Migration Path

1. Slack client + messaging (replace WhatsApp)
2. Thread dispatch + worktrees (replace state machine)
3. OpenRouter + native agent loop (replace `claude -p`)
4. PM tools + memory system
5. Artist integration + image/UX flow
6. Conflict detection + merge coordination

---

## 19. Decisions

- [x] **Separate OS processes** — one per agent, each with its own Slack listener, goroutines per thread
- [x] **Communication 100% via Slack** — no IPC, no RPC. Tasks are @mentions in the thread
- [x] **No database** — Slack thread is the source of truth. No SQLite. OpenRouter is stateless
- [x] **OpenAI-compatible tool calling** — OpenRouter normalizes all models to OpenAI tool calling format
- [x] **Each agent executes tools locally** — no RPC to a central executor
- [x] **All agents always running** — idle until @mentioned, pick up pending messages on restart
- [x] Multi-agent architecture — one binary, parameterized by `--role`
- [x] One MD per agent = system prompt + project map + learnings (seeded on first run, evolved by Lead)
- [x] `global.md` — shared project knowledge for all agents
- [x] `workflows.md` — process playbook, evolved by Lead
- [x] OpenRouter for all LLM calls (no CLI dependency)
- [x] Native tools in Go (same as Claude Code + more)
- [x] Artist as UI/UX designer — dual-model (Sonnet for UX, OpenAI for images). Artist output + PM plan = Coder input
- [x] Reviewer agent — code review loop after Coder, before Lead
- [x] Thread is source of truth — all inter-agent messages visible in Slack thread
- [x] Agent identities — `@codebutler.pm`, `@codebutler.coder`, etc. One bot, six identities
- [x] Agent-driven flow — agents pass work via @mentions, daemon only routes
- [x] Escalation hierarchy — user > Lead > individual agents
- [x] Discovery workflow — PM interviews → Artist designs → Lead builds roadmap → GitHub issues
- [x] Escalation-driven learning — questions today → workflow improvements tomorrow
- [x] Project map per agent — each knows the project from its perspective
- [x] Artist visual memory — `artist/assets/` for screenshots, mockups
- [x] Thread = Branch = PR (1:1:1, non-negotiable)
- [x] User closes thread explicitly (no timeouts)
- [x] Worktree isolation, per-platform init
- [x] Git flow for all MDs — learnings land on main with merge
- [x] PM model pool with hot swap
- [x] `gh` CLI for GitHub operations
- [x] Goroutine-per-thread, buffered channels, panic recovery
- [x] **Automatic first-run wizard** — no separate `init` command. Detects missing config and guides token setup + repo seed + service install
- [x] **OpenAI key mandatory** — required for image generation (Artist) and voice transcription (Whisper). OpenRouter can't generate images
- [x] **OS services with auto-restart** — LaunchAgent (macOS) / systemd (Linux). 6 services per repo. Survive reboots, restart on crash
- [x] **Multi-repo = same Slack app, different channels** — global tokens shared, per-repo config separate
- [x] **Agent↔model conversation files** — per-agent, per-thread JSON in worktree. Full model transcript (tool calls, reasoning, retries) separate from Slack messages. Agent decides what to post publicly

---

## 20. v1 vs v2

| Aspect | v1 (WhatsApp) | v2 (Slack + OpenRouter) |
|--------|---------------|------------------------|
| Platform | WhatsApp (whatsmeow) | Slack (Socket Mode) |
| LLM execution | `claude -p` subprocess | OpenRouter API (native loop) |
| Tools | Delegated to Claude Code | Owned by CodeButler |
| System prompts | CLAUDE.md | Per-agent MDs (= prompt + memory) + global.md + workflows.md |
| Parallelism | 1 conversation | N concurrent threads |
| State machine | ~300 lines, 4 states | None (event-driven) |
| Isolation | Shared directory | Git worktrees |
| Agents | 1 (Claude) | 6 (PM, Researcher, Artist, Coder, Reviewer, Lead) |
| Communication | N/A | Inter-agent messaging in thread |
| Memory | None | Per-agent, git flow, Lead-extracted |
| Team support | Single user | Multi-user native |
| Code complexity | ~630 lines daemon.go | ~200 lines estimated |

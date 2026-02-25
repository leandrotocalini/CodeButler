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

For discovery: PM interviews → Artist designs UX for visual features → Lead builds roadmap in `.codebutler/roadmap.md`. Each roadmap item → future implement thread, or `develop` for all at once.

For learn (onboarding): all agents explore codebase in parallel from their perspective → populate project maps + `global.md` → user approves.

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

MCP lets agents use external tools beyond the built-in set. An MCP server is a child process that exposes tools over stdio — database queries, API calls, file system extensions, whatever the server implements. CodeButler's agent loop already does tool-calling; MCP tools appear alongside native tools in the same loop.

**Config:** `.codebutler/mcp.json` (per-repo, committed to git). Defines available servers and which agents can use them.

```json
{
  "servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}" },
      "roles": ["pm", "coder", "reviewer", "lead"]
    },
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres"],
      "env": { "DATABASE_URL": "${DATABASE_URL}" },
      "roles": ["coder", "reviewer"]
    },
    "linear": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-linear"],
      "env": { "LINEAR_API_KEY": "${LINEAR_API_KEY}" },
      "roles": ["pm", "lead"]
    },
    "figma": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-figma"],
      "env": { "FIGMA_TOKEN": "${FIGMA_TOKEN}" },
      "roles": ["artist"]
    },
    "sentry": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sentry"],
      "env": { "SENTRY_AUTH_TOKEN": "${SENTRY_AUTH_TOKEN}" },
      "roles": ["coder", "reviewer", "pm"]
    }
  }
}
```

**Per-agent access (`roles` field):** each server lists which agents can use it. When an agent process starts, it only launches the MCP servers assigned to its role. The Coder gets database access; the PM doesn't. The Artist gets Figma; the Coder doesn't. If `roles` is omitted, all agents get access.

**Environment variables:** MCP server configs can reference env vars with `${VAR}` syntax. Secrets come from the environment (set in the service unit or shell), never stored in the committed config file.

**Lifecycle:**
1. Agent process starts → reads `mcp.json` → filters servers by its role → launches each as a child process (stdio transport)
2. Agent discovers tools from each server (MCP `tools/list`)
3. MCP tools are added to the tool list sent to the LLM alongside native tools
4. When the LLM calls an MCP tool → agent routes the call to the right server process → returns result to LLM
5. Agent process stops → all child MCP server processes are killed

**What this enables:**
- **GitHub:** PM triages issues, Reviewer posts inline PR comments, Lead links retrospective findings to issues — all via GitHub API, not just `gh` CLI
- **Databases:** Coder queries schemas, checks data, runs migrations
- **Project management:** PM reads/updates Linear or Jira tickets, Lead creates issues from retrospectives
- **Design:** Artist pulls components and styles from Figma
- **Monitoring:** Coder and Reviewer check Sentry for error context when debugging
- **Custom servers:** any stdio MCP server works — teams can build project-specific servers

**What this does NOT replace:** native tools (Read, Write, Bash, Grep, etc.) stay native. MCP is for external integrations, not for reimplementing built-in capabilities.

### 1.7 Skills — Custom Commands

Skills are project-specific, reusable commands defined as markdown files. While workflows define multi-step processes (implement, bugfix, discover), skills are more atomic — focused actions that teams use repeatedly. Think of them as custom slash commands with full agent backing.

**Where they live:** `.codebutler/skills/` — one `.md` file per skill, committed to git.

**Example skill — `deploy.md`:**

```markdown
# deploy

Deploy the project to an environment.

## Trigger
deploy, deploy to {environment}

## Agent
coder

## Prompt
Deploy the project to {{environment | default: "staging"}}.

1. Run the test suite first. If tests fail, stop and report
2. Run `./scripts/deploy.sh {{environment}}`
3. Verify the deployment is healthy (curl the /health endpoint)
4. Report: deployed version, URL, any warnings
```

**Example skill — `db-migrate.md`:**

```markdown
# db-migrate

Run database migrations.

## Trigger
migrate, run migrations, db migrate

## Agent
coder

## Prompt
Run database migrations.

1. Check for pending migrations: `npx prisma migrate status`
2. If pending: run `npx prisma migrate deploy`
3. If dev environment: run `npx prisma generate` to update client
4. Report: migrations applied, current state
```

**Example skill — `changelog.md`:**

```markdown
# changelog

Generate a changelog entry for the latest release.

## Trigger
changelog, what changed, release notes

## Agent
pm

## Prompt
Generate a changelog entry from recent git history.

1. Read the latest tag: `git describe --tags --abbrev=0`
2. List commits since that tag: `git log <tag>..HEAD --oneline`
3. Group by type (features, fixes, refactors)
4. Write a structured changelog entry in CHANGELOG.md
5. Post the entry in the thread for review
```

**Skill file format:**

| Section | Required | Description |
|---------|----------|-------------|
| `# name` | Yes | Skill name (matches filename) |
| Description | Yes | One-line description (shown in help) |
| `## Trigger` | Yes | Comma-separated trigger phrases. `{param}` captures variables |
| `## Agent` | Yes | Which agent executes: `pm`, `coder`, `reviewer`, `researcher`, `artist`, `lead` |
| `## Prompt` | Yes | The instructions sent to the agent. `{{param}}` resolves captured variables. `{{param \| default: "value"}}` for defaults |

**How it works:**

1. User posts in Slack: "deploy to production"
2. PM classifies intent — checks workflows first, then scans skills (trigger match)
3. PM resolves variables: `{environment}` → `"production"`
4. PM routes to the skill's target agent with the resolved prompt
5. Agent executes following the prompt instructions
6. If the skill triggers code changes → normal flow continues (PR, Reviewer, Lead)
7. If no code changes → agent reports result in the thread, done

**Skills vs workflows:**

| | Workflows | Skills |
|---|-----------|--------|
| Scope | Multi-agent, multi-step processes | Single-agent, focused actions |
| Definition | `workflows.md` (one file, all workflows) | `skills/*.md` (one file per skill) |
| Who creates | Seeded on init, evolved by Lead | Seeded on init + team members + Lead |
| Parameters | None (PM interviews for context) | Captured from trigger phrases |
| Examples | implement, bugfix, discover, refactor | explain, test, hotfix, changelog, docs, security-scan, status |

**PM intent classification order:**
1. Exact workflow match → run workflow
2. Skill trigger match → run skill
3. Ambiguous → present options (workflows + skills) to user

**Seeded skills** (included on `codebutler init`):

| Skill | Agent | What it does |
|-------|-------|-------------|
| `explain` | PM | Explain how a part of the codebase works |
| `test` | Coder | Write tests for a specific file, function, or module |
| `changelog` | PM | Generate a changelog entry from recent git history |
| `hotfix` | PM | Investigate and quick-fix a bug (PM finds root cause, then sends to Coder) |
| `docs` | Coder | Generate or update documentation for a part of the codebase |
| `security-scan` | Reviewer | Run a security audit (OWASP Top 10 + project-specific risks) |
| `self-document` | Lead | Document recent work in JOURNEY.md |
| `status` | PM | Report project status (roadmap, active PRs, recent activity) |
| `triage-issue` | PM | Triage a GitHub issue: analyze, label, prioritize, route (GitHub MCP) |
| `review-pr` | Reviewer | Review a PR with inline comments on GitHub (GitHub MCP) |
| `release` | PM | Create a GitHub release with auto-generated notes (GitHub MCP) |

These live in `seeds/skills/` and are copied to `.codebutler/skills/` on init. Teams can modify, delete, or add new ones.

Skills marked **(GitHub MCP)** require the GitHub MCP server configured in `mcp.json`. They use GitHub API tools (`get_issue`, `create_pull_request_review`, `create_release`, etc.) for richer interaction than `gh` CLI alone — inline PR comments, issue labeling, release publishing.

**Who creates skills:**
- **Seeded** — 11 default skills cover common development tasks and GitHub workflows
- **Team members** — create `.md` files in `skills/` manually for project-specific operations
- **Lead** — proposes new skills during retrospective when it spots recurring patterns ("you've asked for deploys 4 times — want me to create a deploy skill?")
- **PM** — suggests skills when users repeatedly describe the same type of task

**Skill validation:** `codebutler validate` checks all skill files in `.codebutler/skills/` and reports errors:

| Check | Error |
|-------|-------|
| Missing required section (`# name`, `## Trigger`, `## Agent`, `## Prompt`) | `deploy.md: missing ## Agent section` |
| Invalid agent name | `deploy.md: agent "builder" is not a valid role (pm, coder, reviewer, researcher, artist, lead)` |
| Duplicate trigger phrases across skills | `deploy.md and release.md: duplicate trigger "deploy"` |
| Undefined `{{variable}}` in prompt (not captured by any trigger `{param}`) | `deploy.md: {{env}} used in prompt but no {env} in triggers` |
| Empty prompt | `deploy.md: ## Prompt section is empty` |

Validation runs automatically during `codebutler init` and when the PM reloads skills (file change detected). Errors are logged as warnings — invalid skills are skipped, not fatal.

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

### `codebutler init`

Run `codebutler init` in a git repo. If the repo isn't configured, this is the only way to set it up — running `codebutler --role <any>` in an unconfigured repo tells you to run `init` first.

**Step 1: Global tokens** (only once per machine — `~/.codebutler/config.json` doesn't exist):

1. **Slack app** — guides you through creating the Slack app (scopes, Socket Mode, bot user). Asks for Bot Token (`xoxb-...`) + App Token (`xapp-...`)
2. **OpenRouter** — asks for API key (`sk-or-...`). Used for all LLM calls
3. **OpenAI** — asks for API key (`sk-...`). Used for image generation (Artist) and voice transcription (Whisper). **Required**
4. Saves all tokens to `~/.codebutler/config.json`

**Step 2: Repo setup** (once per repo — `<repo>/.codebutler/` doesn't exist):

1. **Seed `.codebutler/`** — creates folder, copies seed MDs (`pm.md`, `coder.md`, `reviewer.md`, `lead.md`, `artist.md`, `researcher.md`, `global.md`, `workflows.md`), copies seed skills (`skills/explain.md`, `test.md`, `changelog.md`, `hotfix.md`, `docs.md`, `security-scan.md`, `self-document.md`, `status.md`, `triage-issue.md`, `review-pr.md`, `release.md`), seeds `mcp.json` with GitHub MCP server (other servers commented as examples), creates `config.json` with default models, creates `artist/assets/`, `branches/`, `images/`
2. **Channel selection** — recommends creating `codebutler-<reponame>`. User can pick an existing channel or accept the recommendation
3. **`.gitignore`** — adds `.codebutler/branches/`, `.codebutler/images/` if not present
4. Saves channel to per-repo `config.json`

**Step 3: Service install** (once per machine):

1. Asks which agents to install on this machine (default: all 6). Different agents can run on different machines
2. Detects OS (macOS / Linux)
3. Installs selected services — one per agent, `WorkingDirectory=<repo>`, restart on failure:
   - macOS: LaunchAgent plists (`~/Library/LaunchAgents/codebutler.<repo>.<role>.plist`)
   - Linux: systemd user units (`~/.config/systemd/user/codebutler.<repo>.<role>.service`)
4. Starts selected services

**Subsequent repos:** Step 1 is skipped (tokens exist). Only steps 2-3 run. Same Slack app, different channel, new services.

**Subsequent machines:** Step 2 is skipped (`.codebutler/` already exists in git). Only steps 1 + 3 run. Same repo, different machine, different agents.

### `codebutler configure`

For post-init changes. Run `codebutler configure` in a configured repo.

- **Change Slack channel** — switch to a different channel
- **Add agent** — install a new agent service on this machine (e.g., add Coder to a more powerful machine)
- **Remove agent** — stop and uninstall an agent service from this machine
- **Update tokens** — change API keys
- **Show config** — display current config (machine + repo)

### Service Management

```bash
codebutler init               # first-time setup (tokens + repo + services)
codebutler configure           # change config post-init
codebutler start              # start all agents installed on this machine
codebutler stop               # stop all agents on this machine
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
  mcp.json                       # MCP server config — servers + per-agent access (committed)
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
  skills/                        # Custom commands — one .md per skill (committed)
    explain.md                   # Seeded: explain how code works (PM)
    test.md                      # Seeded: write tests (Coder)
    changelog.md                 # Seeded: generate changelog from git (PM)
    hotfix.md                    # Seeded: investigate + quick-fix a bug (PM → Coder)
    docs.md                      # Seeded: generate/update documentation (Coder)
    security-scan.md             # Seeded: security audit (Reviewer)
    self-document.md             # Seeded: document work in JOURNEY.md (Lead)
    status.md                    # Seeded: project status report (PM)
    triage-issue.md              # Seeded: triage GitHub issues (PM, GitHub MCP)
    review-pr.md                 # Seeded: review PRs with inline comments (Reviewer, GitHub MCP)
    release.md                   # Seeded: create GitHub releases (PM, GitHub MCP)
  research/                      # Researcher findings (committed, merged with PRs)
    stripe-api.md                # Example: Stripe best practices
    vite-plugins.md              # Example: Vite plugin system docs
  roadmap.md                     # Planned work items with status + dependencies
  branches/                      # Git worktrees, 1 per active thread (gitignored)
    <branchName>/                # One worktree per thread
      conversations/             # Agent↔model conversation files
        pm.json                  # PM's full model conversation for this thread
        coder.json               # Coder's full model conversation
        reviewer.json            # ...etc
  images/                        # Generated images (gitignored)
```

**Committed to git:** `config.json`, `mcp.json`, all `.md` files, `skills/`, `artist/assets/`, `research/`, `roadmap.md`. **Gitignored:** `branches/` (including conversation files), `images/`.

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

### SendMessage(message, waitForReply)

LLM tool for posting to the current Slack thread. To direct a message to another agent, the LLM includes `@codebutler.<role>` in the message body — same as a human would. No `to` parameter; routing is handled by each process's message filter when it receives the Slack event.

**Sender identification:** the system automatically prefixes `@codebutler.<self>:` to the posted message. The LLM only writes the body — it doesn't need to identify itself. This guarantees consistent formatting and the receiving agent always knows who sent the message.

```json
{
  "name": "SendMessage",
  "parameters": {
    "message": "string — the text to post (include @codebutler.<role> to target an agent)",
    "waitForReply": "boolean — block the agent loop until a reply arrives in the thread"
  }
}
```

Example: PM calls `SendMessage(message: "@codebutler.coder implement: [plan]")` → posted to Slack as `@codebutler.pm: @codebutler.coder implement: [plan]`.

The daemon only routes — **agents drive the flow themselves**.

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

### Message Redaction

Agents have access to MCP servers (databases, APIs, monitoring) and tools that can return sensitive data. Before any message is posted to Slack via `SendMessage`, the runtime applies a redaction filter.

**What gets redacted:**

| Pattern | Example | Replacement |
|---------|---------|-------------|
| API keys | `sk-or-v1-abc123...`, `sk-...`, `xoxb-...` | `[REDACTED:api_key]` |
| JWT tokens | `eyJhbG...` (base64 with dots) | `[REDACTED:jwt]` |
| Private keys | `-----BEGIN.*PRIVATE KEY-----` | `[REDACTED:private_key]` |
| Connection strings | `postgres://user:pass@host/db` | `[REDACTED:connection_string]` |
| Common secret patterns | `password=`, `secret=`, `token=` followed by value | `[REDACTED:secret]` |
| IP addresses + ports (internal) | `192.168.*`, `10.*`, `172.16-31.*` with ports | `[REDACTED:internal_ip]` |

**How it works:**

1. Agent calls `SendMessage(message, waitForReply)`
2. Before posting to Slack, the message passes through `redact.Filter(text) string`
3. The filter runs a set of compiled regexes against the text
4. Matches are replaced with `[REDACTED:<type>]`
5. If any redaction occurred, the original unredacted text is logged locally (structured log, `Debug` level) for debugging
6. The redacted message is posted to Slack

**What stays unredacted:** code snippets, file paths, git hashes, branch names, URLs to public services. The filter is tuned to avoid false positives on normal development content.

**Per-repo custom patterns:** `.codebutler/policy.json` can define additional redaction patterns:

```json
{
  "redaction": {
    "patterns": [
      {"name": "internal_url", "regex": "https?://internal\\.company\\.com[^\\s]*"},
      {"name": "customer_id", "regex": "cust_[a-zA-Z0-9]{20,}"}
    ]
  }
}
```

The redactor is a pure function — regex match + replace, no network calls, no LLM. Runs in microseconds. Applied to every outbound Slack message, no exceptions.

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

The entry point for all user messages. Talks to user, explores codebase, selects workflow or skill, delegates to other agents via @mentions in the thread. Cheap model (Kimi by default). System prompt: `pm.md` + `global.md` + `workflows.md` + skill index from `skills/`. Capped at 15 tool-calling iterations per activation.

The PM's goroutine for a thread stays alive while the Coder works — when the Coder @mentions PM with a question, the PM's Slack listener routes it to that thread's goroutine and responds.

### Researcher — On-Demand Web Research

Listens for @mentions from **any agent** (not just PM). Any agent can `@codebutler.researcher` when it needs external context — docs, best practices, API references, vulnerability databases, design patterns. Runs WebSearch + WebFetch → synthesizes → posts summary back in thread. Parallel-capable (multiple goroutines for concurrent research requests).

**Research persistence:** findings are saved to `.codebutler/research/` as individual MD files. When the Researcher persists a finding, it also adds a one-line entry to the `## Research Index` section in `global.md` using `@` references (e.g., `- Stripe API v2024 — @.codebutler/research/stripe-api-v2024.md`). Since all agents read `global.md` as part of their system prompt, they can see what research exists and read the full file when they need depth. Persisted research is committed to git and merges with PRs — knowledge accumulates across threads. The Researcher checks the index before searching again.

**The Researcher never acts on its own initiative.** It only activates when another agent asks. Its knowledge grows on-demand, driven by real questions from agents who encounter something they don't know.

System prompt: `researcher.md` + `global.md`.

### Artist — UI/UX Designer + Image Gen

Dual-model. Listens for @mentions from PM. Claude Sonnet for UX reasoning (layouts, component structure, UX flows). OpenAI gpt-image-1 for image gen/editing. Posts design proposals back in the thread. Reads `artist/assets/` for visual references to stay coherent with existing UI. System prompt: `artist.md` + `global.md`.

### Coder — Builder

Claude Opus 4.6. Listens for @mentions from PM (task) and Reviewer (feedback). Full tool set, executes locally in isolated worktree. Creates PRs. When it needs context, @mentions PM in the thread. When done, @mentions Reviewer. System prompt: `coder.md` + `global.md` + task context from thread.

**Sandboxing:** MUST NOT install packages, leave worktree, modify system files, or run destructive commands. Enforced at tool execution layer (path validation, command filtering) — stronger than prompt-only.

### Reviewer — Code Review Loop

Listens for @mentions from Coder ("PR ready"). Sends structured feedback back to Coder via @mention. Loop until approved (max 3 rounds). When approved, @mentions Lead. Disagreements escalate to Lead. System prompt: `reviewer.md` + `global.md`.

**Structured review protocol — the Reviewer produces three artifacts before commenting on the diff:**

1. **Invariants list** — what MUST NOT break. Extracted from the PM's plan, existing tests, and the codebase context. Example: "auth middleware must reject expired tokens", "API response format must not change for existing endpoints". These become the Reviewer's acceptance criteria
2. **Risk matrix** — categorizes changes by risk area:
   - **Security**: auth, input validation, secrets, permissions
   - **Performance**: N+1 queries, unbounded loops, missing pagination
   - **Compatibility**: API changes, schema migrations, config format changes
   - **Correctness**: edge cases, error handling, race conditions
3. **Test plan** — minimum tests that should exist for the changes. Categorized as unit / integration / e2e. The Reviewer checks if the Coder wrote them; if not, the first review round requests them before reviewing logic

**Only after producing these three artifacts does the Reviewer comment on the diff.** This prevents "looks good" reviews and catches subtle bugs that line-by-line comments miss. The invariants and risk matrix are posted in the thread (visible to Lead for retrospective). The test plan is included in feedback to the Coder.

### Lead — Mediator + Retrospective

Listens for @mentions from Reviewer ("approved") or from agents in disagreement. At thread close, reads **full thread transcript** from Slack. Three phases:

1. **Analysis** (solo) — identifies friction, wasted turns, escalation patterns
2. **Discussion** (multi-agent) — @mentions each agent in the thread, discusses improvements
3. **Proposals** (to user) — concrete updates to agent MDs, `global.md`, `workflows.md`

**Produces:** PR description, learnings for agent MDs, workflow evolution, usage report.

**Structured retrospective output — the Lead produces a fixed format:**

- **3 things that went well** — with evidence (thread message links, metrics)
- **3 friction points** — root cause, not symptoms. "Coder used 40 turns because PM didn't include auth middleware location" not "Coder took too long"
- **1 process change** — concrete workflow step to add, remove, or modify
- **1 prompt change** — specific update to an agent's seed or learnings
- **1 new skill or skill tweak** — if a pattern in this thread would benefit from a reusable skill (optional — only if applicable)
- **1 guardrail** — new check, limit, or safety measure if the thread revealed a risk (optional)

Each item includes: severity (low/medium/high), estimated impact on future threads, and the specific file + section to update. This structured output makes it easy for the user to approve or reject individual proposals.

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
| `researcher.md` | Personality + rules. Search strategies. **Research index:** what it has investigated, where findings are stored | Lead + self |
| `research/` | Persisted research findings as individual MD files. Committed to git, merges with PRs. Researcher decides what to persist | Researcher |
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
- Roadmap status updates → `roadmap.md`

**Project maps evolve:** when a thread adds a screen, changes an API, or introduces a pattern, the Lead updates the relevant agent's project map. User approves.

### Learning Patterns

**Message-driven:** Coder keeps asking PM about auth → Lead proposes workflow step for auth check → no question next time.

**Inter-agent:** Each agent's MD accumulates how to work with other agents. PM learns what Coder needs. Artist learns what detail level Coder expects. Cross-cutting knowledge goes to `global.md`.

### Learnings Schema

Learnings are the most volatile part of agent MDs. Without structure, they accumulate into an incoherent blob of contradictory rules. Every learning the Lead proposes must follow a structured format in the markdown:

```markdown
### [Learning title]
- **When:** [situation/trigger that makes this relevant]
- **Rule:** [the concrete instruction]
- **Example:** [specific case from the thread where this was learned]
- **Confidence:** high | medium | low
- **Source:** thread-<ts>, <date>
```

**Example:**

```markdown
### Always check for existing auth middleware before creating new
- **When:** implementing features that need authentication
- **Rule:** before writing auth code, search for existing middleware in internal/auth/ and reuse it
- **Example:** thread-1709312345: Coder wrote a new JWT validator when internal/auth/middleware.go already had one, causing 4 extra review rounds
- **Confidence:** high
- **Source:** thread-1709312345, 2026-02-20
```

### Learnings Pruning

Agent MDs grow over time. The Lead manages this growth with periodic pruning — same as re-learn is knowledge GC for project maps, pruning is knowledge GC for learnings.

**When pruning runs:**
- During every retrospective, the Lead reviews existing learnings in the agents it's updating
- During re-learn, the Lead reviews all learnings across all agents
- The Lead can also propose pruning standalone if it detects MD bloat

**Pruning criteria:**

| Signal | Action |
|--------|--------|
| Learning contradicts a newer learning | Remove the older one, keep the newer. Note the replacement in the new learning |
| Learning references code/patterns that no longer exist | Archive (move to a `## Archived Learnings` section at the bottom of the MD, collapsed) |
| Learning has `confidence: low` and hasn't been reinforced in 10+ threads | Archive |
| Learning is redundant with project conventions in `global.md` | Remove from agent MD (it's already covered globally) |
| Agent MD exceeds ~30K tokens | Lead must prune before adding new learnings. Net-zero or net-negative token growth |

**Archived learnings** stay in the MD but in a collapsed section that's excluded from the system prompt (the prompt builder skips `## Archived Learnings`). They remain in git history and can be restored if needed.

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

## 13. Worktree Isolation & Git Sync

Each thread gets a git worktree in `.codebutler/branches/<branchName>/`. **Created early by the PM** — as soon as the PM starts working on a thread, it creates the worktree, pushes the branch to remote, and begins saving its conversation file there. Branch: `codebutler/<slug>`.

### Git Sync Protocol

**Every change an agent makes to the worktree must be committed and pushed.** This is non-negotiable — it ensures all agents (whether on the same machine or different machines) see the latest state.

1. **After any file change** (code, MDs, conversation files, research): `git add` + `git commit` + `git push`
2. **Before reading shared state** (agent MDs, global.md, research/): `git pull` to get the latest
3. **On branch creation** (PM starts a thread): create worktree, push branch to remote immediately
4. **When an agent is @mentioned for a branch it doesn't have locally**: pull the branch, create local worktree, start working

### Divergence Handling

If two agents push to the same branch concurrently (e.g., Coder pushed code while Researcher pushed a research file):

1. The second push fails (non-fast-forward)
2. Agent pulls with rebase: `git pull --rebase`
3. If conflicts: agent resolves automatically (each agent knows its own files — conversation files never conflict, agent MDs rarely conflict)
4. If unresolvable: agent posts in thread asking for help, marks the issue

In practice, conflicts are rare because agents work on different files: Coder writes code, Researcher writes to `research/`, Lead writes to MDs. The main risk is two agents editing the same MD — the Lead handles most MD writes, so this is serialized naturally.

### Distributed Agents

Agents can run on different machines. The default is all 6 on one machine, but the architecture supports distribution:

- **Same machine (default):** all agents share the filesystem. Git sync still applies — agents commit and push after every change. This is the simplest setup and works for most cases
- **Multiple machines:** each machine runs a subset of agents (e.g., Coder on a powerful GPU machine, PM on a cheap always-on server). Each machine has its own clone of the repo. Agents coordinate through git (code/files) and Slack (messages)

**What changes with distribution:**
- `codebutler init` on each machine — same repo, same Slack app, different agents installed as services
- Each machine creates local worktrees for active branches on demand (pull from remote)
- Conversation files are per-agent, so no conflicts — each agent owns its own `conversations/<role>.json`
- The PM pushes the branch immediately after creation so other agents on other machines can pull it

**What doesn't change:** Slack is still the communication bus. @mentions still drive the flow. The agent loop is the same. The only difference is that "read file" might require a `git pull` first.

| Platform | Init | Build Isolation |
|----------|------|-----------------|
| iOS/Xcode | `pod install` | `-derivedDataPath .derivedData` |
| Node.js | `npm ci` | `node_modules/` per worktree |
| Go | Nothing | Module cache is global |
| Python | `venv + pip install` | `.venv/` per worktree |
| Rust | Nothing | `CARGO_TARGET_DIR=.target` |

### Worktree Garbage Collection

Normal cleanup happens when the user closes a thread (Lead deletes worktree + remote branch). But orphans accumulate: PM crashes before cleanup, threads are abandoned, close sequence fails. The PM runs garbage collection on startup and every 6 hours.

**A worktree is orphaned when:** no Slack activity for 48h + thread is not in `coder` phase + no open PR exists for the branch.

**GC sequence:** warn in thread (tag participants, 24h grace period) → wait → archive reports/decisions to main → remove worktree → delete remote branch → close stale PR if any.

**On any agent restart:** reconcile local worktrees with Slack threads — if thread is gone, clean up immediately. See ARCHITECTURE.md for full implementation details.

---

## 14. Multi-Model Orchestration

All via OpenRouter. PM has model pool with hot swap. Cost controls: per-thread cap, per-day cap, per-user hourly limit. Circuit breaker: 3x PM failure → fallback for 5 minutes.

### Dynamic Model Routing

Not every task needs Opus. The PM classifies task complexity during planning and assigns models accordingly. This reduces cost without sacrificing quality where it matters.

**Complexity tiers:**

| Tier | Examples | Coder model | Estimated cost |
|------|----------|-------------|----------------|
| **Simple** | Rename variable, fix typo, update config value, add comment | Sonnet | ~$0.02-0.10 |
| **Medium** | Add endpoint, write tests for existing code, refactor function, fix straightforward bug | Sonnet | ~$0.10-0.50 |
| **Complex** | New feature with multiple files, architecture change, complex bug with unclear root cause, integration work | Opus | ~$0.50-2.00 |

**How it works:**

1. PM classifies complexity as part of the plan (already estimates cost — this makes it precise)
2. PM includes the model recommendation in the plan: "Complexity: simple → Sonnet"
3. The Coder process reads the model recommendation from the PM's message and uses it for that activation
4. If the Coder hits issues that suggest the task is harder than estimated (stuck detection fires, >50% of maxTurns used, multiple test failures), it can self-escalate to the more capable model mid-task

**Config:**

```json
{
  "models": {
    "coder": {
      "default": "anthropic/claude-opus-4-6",
      "simple": "anthropic/claude-sonnet-4-5-20250929",
      "medium": "anthropic/claude-sonnet-4-5-20250929",
      "complex": "anthropic/claude-opus-4-6"
    }
  }
}
```

**The PM decides, not the Coder.** The PM has already explored the codebase and understands the scope. The Coder receives the model assignment and uses it. This avoids the Coder always choosing the most capable (expensive) model for itself.

**Two-pass review optimization:** for the Reviewer, the first pass can use a cheaper model to catch obvious issues (linting, formatting, missing tests). If the first pass finds nothing, the review is done. If it finds issues, a second pass with the full model does deep analysis (security, architecture, subtle bugs). Configured per-repo — teams that want thorough reviews on every PR can disable two-pass.

---

---

## 16. Operational Details

### Slack Features
- Agent identity: one bot, six display names + icons
- Reactions: 👀 processing, ✅ done
- Threads = sessions (1:1). Multiple concurrent
- Code snippets: <20 lines inline, ≥20 lines as file uploads

### Block Kit Interactive Messages

For decision points that need user input, agents use Slack Block Kit instead of plain text. This replaces "reply with 1, 2, or 3" with actual buttons.

**Where Block Kit is used:**

| Decision point | Agent | Block Kit element |
|---------------|-------|------------------|
| Plan approval | PM | Buttons: `Approve` / `Modify` / `Reject` |
| Learnings approval | Lead | Buttons per learning: `Accept` / `Reject` (multi-select) |
| Destructive tool approval | Any | Buttons: `Approve` / `Reject` + command preview in code block |
| Workflow selection (ambiguous intent) | PM | Button group with workflow names + descriptions |
| Merge readiness | Lead | Buttons: `Merge` / `Hold` / `Close without merge` |
| GC warning | PM | Buttons: `Keep open` / `Close & clean` |

**How it works:**

1. Agent calls `SendMessage` with a `blocks` parameter (JSON array of Block Kit blocks)
2. Slack renders the interactive message with buttons
3. When user clicks a button, Slack sends an `interaction` event (separate from message events)
4. The agent process receives the interaction, extracts the `action_id` and `value`
5. The agent resumes its loop with the user's choice as input

**Fallback:** if Block Kit rendering fails or the Slack app doesn't have `interactions` scope, agents fall back to plain text with numbered options. The user replies with text as before.

**Reactions as signals:** beyond Block Kit buttons, emoji reactions on agent messages are lightweight signals:
- 👍 on a learning = approve it
- 🛑 on any agent message = stop the agent immediately (agent checks reactions before each tool call)
- 👀 is set automatically by the agent when it starts processing a message

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

## 15b. Tool Risk Tiers & Approval Gates

All agents share the same tool set, but not all tools carry the same risk. A file read is free; a `git push` is visible to the team; a database migration is potentially destructive. The tool executor classifies every tool call by risk tier and enforces approval gates for dangerous operations.

### Risk Tiers

| Tier | Tools | Behavior |
|------|-------|----------|
| **READ** | Read, Grep, Glob, WebSearch, WebFetch | Execute immediately. No approval needed. Zero side effects |
| **WRITE_LOCAL** | Write, Edit, Bash (safe subset: test runners, linters, build commands) | Execute immediately. Changes stay in the worktree. Reversible via git |
| **WRITE_VISIBLE** | GitCommit, GitPush, GHCreatePR, SendMessage | Execute with logging. These are visible to the team (Slack thread, GitHub). Agent proceeds autonomously but every action is logged in the decision log |
| **DESTRUCTIVE** | Bash (dangerous subset: `rm -rf`, `DROP TABLE`, package installs, deploy scripts, credential rotation) | **Requires explicit user approval.** Agent posts the exact command + explanation in the thread, waits for user confirmation before executing |

### Classification

Tool risk is determined by a combination of:

1. **Tool name** — Read, Grep, Glob are always READ. GitPush is always WRITE_VISIBLE
2. **Bash command analysis** — the executor parses the command before execution and classifies it:
   - Safe list: `go test`, `npm test`, `make build`, `go vet`, `eslint`, `pytest`, linters, compilers → WRITE_LOCAL
   - Dangerous patterns: `rm -rf`, `DROP`, `DELETE FROM`, `deploy`, `docker`, `sudo`, `chmod`, `curl | sh`, package managers with install flags → DESTRUCTIVE
   - Unknown commands → WRITE_LOCAL (conservative default for unknown, but not blocking)
3. **Per-repo overrides** — `.codebutler/policy.json` can promote or demote specific commands:

```json
{
  "tool_overrides": {
    "bash": {
      "destructive": ["npm publish", "yarn deploy", "./scripts/migrate.sh"],
      "safe": ["docker compose up -d"]
    }
  },
  "require_approval": {
    "git_push_to": ["main", "master", "release/*"],
    "create_pr_to": ["main"]
  }
}
```

### Approval Flow

When a DESTRUCTIVE tool call is detected:

1. Agent pauses the loop
2. Posts in thread: "I need to run a potentially destructive command. Please approve or reject."
   ```
   Command: ./scripts/migrate.sh --env production
   Risk: DESTRUCTIVE (matches policy.json override)
   Reason: Coder needs to run migration as part of the implementation plan
   ```
3. User replies with approval (or the agent detects 👍 reaction) → execute
4. User rejects → agent finds alternative or escalates to PM

### Per-Role Restrictions

On top of tiers, agents have role-based restrictions enforced at the executor level (not just in seeds):

| Agent | Cannot use |
|-------|-----------|
| **PM** | Write, Edit, GitCommit, GitPush, GHCreatePR (PM never writes code) |
| **Researcher** | Write, Edit, Bash, GitCommit, GitPush (Researcher only reads web + writes to `research/` via a dedicated tool) |
| **Artist** | Bash, GitCommit, GitPush (Artist produces designs, not code) |
| **Reviewer** | Write, Edit, Bash (Reviewer reads and comments, never modifies code) |
| **Lead** | Bash (Lead writes to MDs via Write/Edit, but never runs commands) |
| **Coder** | No restrictions (full tool access within worktree sandbox) |

These restrictions are structural — the executor rejects the call before it reaches the tool. Stronger than prompt-only instructions.

---

## 15. Learn Workflow — Onboarding & Re-learn

### First Run (Automatic)

When `.codebutler/` is seeded for the first time on a repo that already has code (detected: files exist outside `.codebutler/`), the learn workflow triggers automatically after the wizard completes. A Slack thread is created in the project channel for the learn session.

### Re-learn (Manual or Suggested)

- **Manual:** user posts "re-learn" or "refresh knowledge" in the channel
- **Suggested:** the Lead proposes a re-learn when it detects project maps are significantly out of sync with the codebase (after many threads, major refactors, etc.)

### How It Works

All agents participate in parallel in a single thread, each exploring the codebase from their own perspective:

| Agent | What it explores | What it populates |
|-------|-----------------|-------------------|
| **PM** | Project structure, README, entry points, features, domains | PM project map |
| **Coder** | Architecture, patterns, conventions, build system, test framework, dependencies | Coder project map |
| **Reviewer** | Test coverage, CI config, linting, security patterns, quality hotspots | Reviewer project map |
| **Artist** | UI components, design system, styles, screens, responsive patterns | Artist project map |
| **Lead** | Reads what all agents found, synthesizes shared knowledge | `global.md` |
| **Researcher** | Nothing on its own — purely reactive, responds when other agents ask | `research/` folder |

**Each agent is intelligent about what to read.** No fixed budget or exploration rules. The PM reads entry points and READMEs, not test files. The Coder reads core modules and build config, not docs. Each agent's personality and perspective naturally guides what's relevant.

**Researcher is purely reactive.** It does not explore on its own. Other agents @mention it on demand when they encounter something they need external context for:
- PM finds Stripe integration → `@codebutler.researcher Stripe API best practices?`
- Coder sees unfamiliar framework → `@codebutler.researcher Vite 6 plugin system docs?`
- Reviewer sees custom linter → `@codebutler.researcher eslint-config-airbnb rules?`

The Researcher's knowledge accumulates organically, driven by real questions — not a dump of everything that might be useful.

### Output

Each agent populates their **project map** section in their MD file. The Lead populates `global.md` with shared knowledge (architecture, tech stack, conventions, key decisions). The user reviews and approves all changes before they're committed.

### Re-learn vs Incremental Learning

| Type | When | What happens |
|------|------|-------------|
| **Incremental** | After each thread (default) | Lead updates specific project maps surgically. Small, targeted changes |
| **Re-learn** | On demand or suggested | Full refresh. Agents re-read the codebase, compare with existing knowledge, **compact**: remove outdated info, update what changed, add what's new. Result is cleaner and possibly smaller — not just additive |

Re-learn is knowledge garbage collection. The project maps should reflect the project as it is now, not as it was plus every change that ever happened.

---

## 17. Roadmap System & Unattended Development

### Roadmap File

The roadmap lives in `.codebutler/roadmap.md` — a committed file in the repo. Source of truth for planned work. The PM updates status as threads execute. The user can read and edit it directly at any time.

```markdown
# Roadmap: [project/feature set name]

## 1. Auth system
- Status: done
- Branch: codebutler/auth-system
- Depends on: —
- Acceptance criteria: JWT-based auth, login/register endpoints, middleware

## 2. User profile API
- Status: in_progress
- Branch: codebutler/user-profile
- Depends on: 1
- Acceptance criteria: CRUD endpoints, avatar upload, validation

## 3. Profile UI
- Status: pending
- Depends on: 1, 2
- Acceptance criteria: Profile page, edit form, avatar picker

## 4. Notification system
- Status: pending
- Depends on: —
- Acceptance criteria: Email + push, per-user preferences, queue-based
```

Statuses: `pending`, `in_progress`, `done`, `blocked`.

If the user wants GitHub issues at any point, the Lead can generate them from the roadmap — but the roadmap file remains the source of truth.

### Three Thread Types

#### 1. Add to Roadmap (`roadmap-add`)

User starts a thread to define new features. PM interviews the user (structured discovery), Artist proposes UX if needed, and the result is new items added to `roadmap.md`. No code, no worktree, no PR.

This can happen multiple times — the roadmap grows incrementally. Each thread adds items. The user can also edit the file directly.

#### 2. Implement Roadmap Item (`roadmap-implement`)

User starts a thread and references a roadmap item: "implement item 3" or describes it. PM picks the item from the roadmap, runs the standard `implement` workflow. On completion, PM marks the item as `done` in the roadmap and updates the branch name.

Same as a regular implement thread, but the plan comes from the roadmap item's acceptance criteria instead of a fresh interview.

#### 3. Implement All (`develop`)

User starts a thread and says "start all", "develop everything", or "implement the roadmap". PM orchestrates **unattended execution** of all `pending` items in the roadmap:

1. Reads `roadmap.md`, builds a dependency graph
2. Launches independent items in parallel (respecting `maxConcurrentThreads` from config)
3. Creates a **new Slack thread for each item** (1 thread = 1 branch = 1 PR, non-negotiable)
4. When an item completes (`done`), checks if dependent items are now unblocked → launches them
5. Posts periodic status updates in the **orchestration thread** (the original thread where the user said "start all")

The orchestration thread is the dashboard. The PM posts updates there:
```
Roadmap progress:
✅ 1. Auth system — done (PR #12 merged)
🔄 2. User profile API — in progress
🔄 4. Notification system — in progress
⏳ 3. Profile UI — waiting on 1, 2
```

### Unattended Execution Model

In `develop` mode, the PM has autonomy to approve plans within the scope of the roadmap. **The roadmap IS the approval.** The user approved the acceptance criteria when they approved the roadmap — the PM doesn't need to ask again for each item.

The PM:
- Creates the plan following the roadmap item's acceptance criteria
- Hands off to Coder directly (skips user approval step)
- Only escalates to user when something is genuinely ambiguous, blocked, or outside the roadmap's scope

### User Tagging

When any agent needs user input during unattended execution (or any thread):

1. Read the Slack thread history
2. Extract all Slack user IDs that have participated in the thread (posted messages, not just reactions)
3. Tag them in the message requesting input

```
@user1 @user2 — blocked on item 3: the roadmap says "JWT auth" but the existing
codebase uses session cookies. Should I migrate to JWT or adapt the plan to sessions?
```

This ensures the right people get notified even when they're not actively watching. The thread creator is always included.

### Failure Handling

- If a thread blocks (needs user input, unfixable test failures, etc.), other independent items continue executing
- Items that depend on a blocked item are marked `blocked` in the roadmap with a reason
- PM posts a summary in the orchestration thread: "item 3 blocked — needs user input on auth approach. Items 4, 5 continuing. Item 6 waiting on 3."
- When the user unblocks an item (answers the question), the PM resumes automatically

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
- [x] Discovery workflow — PM interviews → Artist designs → Lead builds roadmap
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
- [x] **`codebutler init` + `codebutler configure`** — explicit init command for first setup (tokens + repo + services). Configure for post-init changes (channel, add/remove agents, tokens)
- [x] **OpenAI key mandatory** — required for image generation (Artist) and voice transcription (Whisper). OpenRouter can't generate images
- [x] **OS services with auto-restart** — LaunchAgent (macOS) / systemd (Linux). 6 services per repo. Survive reboots, restart on crash
- [x] **Multi-repo = same Slack app, different channels** — global tokens shared, per-repo config separate
- [x] **Agent↔model conversation files** — per-agent, per-thread JSON in worktree. Full model transcript (tool calls, reasoning, retries) separate from Slack messages. Agent decides what to post publicly
- [x] **Learn workflow** — automatic onboarding on first run (repo with existing code), re-learn on demand or suggested by Lead. Each agent explores from its perspective, Researcher reactive on demand. Compacts knowledge on re-learn
- [x] **Roadmap file** — `.codebutler/roadmap.md`, committed to git. Source of truth for planned work. Not GitHub issues (can generate them optionally)
- [x] **Three roadmap thread types** — add items (discovery → roadmap), implement one item, implement all (unattended batch)
- [x] **Unattended develop** — PM orchestrates all roadmap items, approves plans within roadmap scope, only escalates when genuinely blocked
- [x] **Researcher open access** — any agent can @mention Researcher, not just PM. Research persisted in `.codebutler/research/`, committed to git, merges with PRs
- [x] **User tagging** — when agents need input, tag all users who participated in the thread
- [x] **Git sync protocol** — every file change is committed + pushed. Agents pull before reading shared state. Non-negotiable for distributed support
- [x] **Distributed agents** — agents can run on different machines. Same repo, same Slack, different services. Git + Slack as coordination. Default is all on one machine
- [x] **PM workflow menu** — when user intent is ambiguous, PM presents available workflows as options. Teaches new users what CodeButler can do
- [x] **Research Index in global.md** — Researcher adds `@` references to persisted findings in global.md. All agents see what research exists via their system prompt
- [x] **MCP support with per-agent access** — `.codebutler/mcp.json` defines MCP servers + which roles can use them. Agent process only launches servers assigned to its role. MCP tools appear alongside native tools in the agent loop. Secrets via env vars, never in config
- [x] **Skills — custom commands** — `.codebutler/skills/*.md` defines project-specific reusable commands. More atomic than workflows, single-agent focused. PM matches trigger phrases during intent classification. Variables captured from user message. Created by team, Lead proposes from retrospective patterns
- [x] **No vector DB for memory** — MD-based memory with Lead curation, git versioning, and full-context loading is sufficient. Agent MDs are small enough to load entirely into context (100% recall, no retrieval errors). Vector DB adds infrastructure, latency, embedding costs, and breaks distributed agents (which coordinate via git + Slack only). Revisit if MDs grow beyond ~50K tokens or cross-project knowledge transfer is needed
- [x] **Event dedup** — each agent process deduplicates Slack events via bounded in-memory `event_id` set (10K entries, 5min TTL). Prevents duplicate tool executions and double-posted messages from Socket Mode retries. No persistent storage needed — restart recovery uses thread history natural dedup
- [x] **Tool risk tiers + approval gates** — tools classified as READ, WRITE_LOCAL, WRITE_VISIBLE, DESTRUCTIVE. Destructive operations require explicit user approval in the thread before execution. Per-role tool restrictions enforced at executor level (not just seeds). Per-repo overrides in `policy.json`
- [x] **Learnings schema + pruning** — every learning follows structured format (when/rule/example/confidence/source). Lead prunes during retros: remove contradictions, archive stale learnings, enforce ~30K token cap per agent MD. Archived learnings excluded from system prompt but preserved in git
- [x] **Message redaction** — regex-based filter on every outbound Slack message. Catches API keys, JWTs, private keys, connection strings, internal IPs. Per-repo custom patterns in `policy.json`. Pure function, microseconds, no LLM. Unredacted text logged locally at Debug level
- [x] **Worktree GC** — PM runs garbage collection on startup + every 6h. Orphan detection: 48h inactivity + not in coder phase + no open PR. Warns first (24h grace), then archives reports and cleans. Agent restart reconciles local worktrees with Slack threads
- [x] **Decision log** — append-only JSONL per thread in worktree. Records significant decisions (workflow selected, model chosen, stuck detected, plan deviated) with input/state/decision/alternatives/evidence. Lead reads during retrospective. Not every tool call — only choice points between alternatives
- [x] **Dynamic model routing** — PM classifies task complexity (simple/medium/complex) during planning and assigns Coder model accordingly. Simple tasks use Sonnet, complex use Opus. Coder can self-escalate to more capable model if stuck detection fires. Two-pass review optimization for Reviewer (cheap first pass, deep second if needed)
- [x] **Reviewer structured protocol** — Reviewer produces invariants list + risk matrix + test plan before commenting on the diff. Prevents "looks good" reviews. Invariants and risk matrix posted in thread for Lead visibility
- [x] **Lead structured retro output** — fixed format: 3 things well + 3 friction points + 1 process change + 1 prompt change + 1 skill tweak + 1 guardrail. Each with severity, impact estimate, and specific file+section to update. Makes user approval granular
- [x] **Block Kit interactive messages** — Slack Block Kit for decision points (plan approval, learning approval, destructive tool confirmation, workflow selection). Buttons replace "reply with 1/2/3". Fallback to plain text if interactions scope unavailable. 🛑 emoji reaction = stop agent immediately
- [x] **Skills validation** — `codebutler validate` checks all skill files: required sections present, valid agent name, no duplicate triggers, no undefined variables in prompts. Runs on init and on file change. Invalid skills skipped with warning, not fatal
- [x] **No thread state machine** — agents drive flow via @mentions, ordering is emergent (PM→Coder→Reviewer→Lead). No centralized state file or locks. Event dedup solves the Slack duplicate problem without adding coordination. Thread phases are informational, not enforced
- [x] **No separate Tester agent** — Coder (Opus) writes tests as part of implementation. Splitting test writing to a cheaper model would produce worse tests. If coverage is insufficient, improve Coder seed or Reviewer checklist
- [x] **No internal Coder-Reviewer loop** — all communication stays in Slack. Adding Go channels between agents breaks the audit trail and the Lead's retrospective input. Slack latency (200-600ms) is acceptable for review loops that happen every 5-15 minutes

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

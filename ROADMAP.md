# CodeButler — Implementation Roadmap

Status legend: `pending` | `in_progress` | `done`

---

## Phase 1: Foundation

The platform that everything else builds on. No agents yet — just the core
runtime, config loading, OpenRouter client, and tool execution.

### M1 — Project Bootstrap `done`

Initialize the Go project and establish the directory structure from
ARCHITECTURE.md.

- [x] `go mod init github.com/leandrotocalini/codebutler`
- [x] Create `cmd/codebutler/main.go` with `--role` flag (cobra or stdlib flag)
- [x] Create `internal/` package layout matching ARCHITECTURE.md:
      `slack/`, `provider/openrouter/`, `provider/openai/`, `tools/`,
      `mcp/`, `skills/`, `multimodel/`, `router/`, `conflicts/`,
      `worktree/`, `config/`, `models/`, `decisions/`, `github/`
- [x] Minimal `main()` that parses `--role` and prints it (proof of life)

**Acceptance:** `go build ./cmd/codebutler && ./codebutler --role pm` prints role.

### M2 — Config System `done`

Load global (`~/.codebutler/config.json`) and per-repo
(`.codebutler/config.json`) configuration.

- [x] Define Go structs for global config (Slack tokens, OpenRouter key, OpenAI key)
- [x] Define Go structs for per-repo config (channel, models, multiModel, limits)
- [x] Config loader: find repo root (walk up for `.codebutler/`), load both files
- [x] Environment variable resolution (`${VAR}` syntax for MCP config)
- [x] Validation: fail fast on missing required fields
- [x] Unit tests with `testdata/` fixtures

**Acceptance:** `config.Load()` returns typed config from fixture files.

### M3 — OpenRouter Client `done`

HTTP client for OpenRouter chat completions with tool-calling support.
This is the only LLM interface — all agents use it.

- [x] Types: `ChatRequest`, `ChatResponse`, `Message`, `ToolCall`, `ToolResult`, `TokenUsage`
- [x] `ChatCompletion(ctx, req) (resp, error)` — single-shot call
- [x] Tool-calling wire format (OpenAI-compatible function calling)
- [x] Response parsing: text response vs tool calls
- [x] Token usage extraction from response metadata
- [x] Error classification (429, 502/503, context_length_exceeded, content_filter, auth, malformed, timeout, unknown) — see ARCHITECTURE.md
- [x] Retry logic: exponential backoff + jitter per error type
- [x] Circuit breaker: per-model, using `sony/gobreaker`
- [x] Unit tests with HTTP test server

**Acceptance:** client can make a chat completion call to OpenRouter, parse a
tool-calling response, retry on 429, and trip the circuit breaker on 3 failures.

### M4 — Tool System `done`

Native tool definitions, sandboxed executor, and risk tier classification.

- [x] Tool interface: `Execute(ctx, ToolCall) (ToolResult, error)`
- [x] Tool registry: name → executor mapping
- [x] Implement native tools: Read, Write, Edit, Bash, Grep, Glob
- [x] Tool risk tiers: READ, WRITE_LOCAL, WRITE_VISIBLE, DESTRUCTIVE
- [x] Bash command classifier (safe list vs dangerous patterns)
- [x] Per-role tool restrictions (PM can't Write, Reviewer can't Edit, etc.)
- [x] Path validation (sandbox to worktree)
- [x] Idempotency: tool-call ID tracking, skip if already executed
- [x] Unit tests per tool + risk classifier

**Acceptance:** tools execute in sandbox, risk tiers enforced, role restrictions
block disallowed tools, idempotent re-execution returns cached results.

---

## Phase 2: Agent Core

The agent loop that drives all six agents. Same code, different config.

### M5 — Agent Loop `done`

The core prompt → LLM → tool-call → execute → repeat loop.

- [x] `AgentRunner` struct with `LLMProvider`, `ToolExecutor`, `MessageSender` interfaces
- [x] `Run(ctx, Task) (Result, error)` — runs the loop until text response or max turns
- [x] System prompt building: seed MD + global.md (+ workflows.md for PM)
- [x] Tool call dispatch: match tool name → execute → append result → next LLM call
- [x] Parallel tool execution when multiple independent tool calls returned
- [x] Turn counter (check before each LLM call, never after)
- [x] Graceful stop: context cancellation propagates through loop
- [x] Unit tests with mock LLM provider

**Acceptance:** agent loop processes tool calls, respects MaxTurns, handles
text responses, parallel tool execution works.

### M6 — Conversation Persistence `done`

Per-agent, per-thread conversation files for crash recovery.

- [x] Conversation file format: JSON array of messages (OpenAI format)
- [x] Save after every model round (crash-safe: write temp + rename)
- [x] Load on resume: rebuild conversation from file
- [x] Thread isolation: path = `.codebutler/branches/<branch>/conversations/<role>.json`
- [x] Unit tests: write, load, resume mid-conversation

**Acceptance:** agent crashes mid-loop, restarts, resumes from last saved round.

### M7 — Agent Loop Safety `done`

MaxTurns enforcement, context compaction, and stuck detection.

- [x] MaxTurns: per-agent caps (PM=15, Coder=100, etc.), configurable
- [x] Context compaction: when approaching context window, summarize old messages
- [x] Stuck detection: same tool+params 3x, same error 3x, no progress 3x
- [x] Escape strategies: inject reflection → force reasoning → reduce tools → escalate
- [x] Progress tracking: hash recent tool calls, detect cycles
- [x] Unit tests for each detection signal + escape strategy

**Acceptance:** stuck detection fires on repeated tool calls, escape strategies
apply in order, agent escalates after all strategies exhausted.

---

## Phase 3: Slack & Communication

Connect agents to Slack. After this phase, a single agent can receive a message
and respond.

### M8 — Slack Client `done`

Socket Mode connection, message send/receive, agent identity.

- [x] Slack Socket Mode client (using `slack-go/slack`)
- [x] Event listener: `message.channels`, `message.groups`
- [x] Message sending: `chat.postMessage` with per-agent display name + icon
- [x] File uploads for code snippets (≥20 lines)
- [x] Emoji reactions: 👀 on processing, ✅ on done
- [x] Event deduplication: bounded in-memory `event_id` set (10K entries, 5min TTL)
- [x] Integration test: connect to Slack, send/receive a message

**Acceptance:** agent connects to Slack via Socket Mode, receives events,
posts with its identity, deduplicates retries.

### M9 — Message Routing & Thread Registry `done`

Per-agent message filtering and goroutine-per-thread dispatch.

- [x] Message filter: PM gets unmentioned + `@codebutler.pm`; others get their @mention only
- [x] `@codebutler.<role>` extraction from message text
- [x] Thread registry: `map[string]*ThreadWorker` — one goroutine per active thread
- [x] Goroutine lifecycle: spawn on first message, die after 60s inactivity, respawn on next
- [x] Panic recovery per goroutine
- [x] Buffered channel per thread worker (message inbox)
- [x] SendMessage tool: post to current thread, auto-prefix `@codebutler.<self>:`
- [x] Message redaction filter (API keys, JWTs, private keys, connection strings, internal IPs)
- [x] Custom redaction patterns from `policy.json`
- [x] Unit tests: routing rules, redaction patterns

**Acceptance:** messages route to correct agent, thread goroutines spawn/die
correctly, redaction catches sensitive patterns.

### M10 — Block Kit & Interactions `pending`

Interactive messages for approval flows.

- [ ] Block Kit message builder (buttons for approve/reject/modify)
- [ ] Interaction event handler (button clicks → resume agent loop)
- [ ] Emoji reaction handler (🛑 = stop agent, 👍 = approve)
- [ ] Fallback to plain text when Block Kit unavailable
- [ ] Integration test with Slack

**Acceptance:** PM can post a plan with Approve/Reject buttons, user clicks,
agent receives the choice and continues.

---

## Phase 4: Git & Worktree

Isolated workspaces for each thread. After this phase, agents can create
branches, commit, push, and create PRs.

### M11 — Worktree Management `pending`

Create, initialize, and remove git worktrees.

- [ ] `Create(branchName)` — `git worktree add`, push branch to remote
- [ ] `Remove(branchName)` — `git worktree remove`, delete remote branch
- [ ] Per-platform init (Go: nothing, Node: `npm ci`, Python: venv, etc.)
- [ ] Branch naming: `codebutler/<slug>` from PM's classification
- [ ] Path: `.codebutler/branches/<branchName>/`
- [ ] Unit tests: create, verify, remove

**Acceptance:** worktree created with correct branch, initialized per platform,
cleanup removes worktree + remote branch.

### M12 — Git & GitHub Tools `pending`

Git and GitHub operations as agent tools.

- [ ] GitCommit tool: stage files, commit (check for already-applied)
- [ ] GitPush tool: push branch (idempotent if remote up to date)
- [ ] GHCreatePR tool: create PR via `gh` CLI (skip if PR exists for branch)
- [ ] Git sync protocol: commit+push after every change, pull before reading shared state
- [ ] `gh pr edit` for PR description updates
- [ ] `gh pr merge --squash` for merge
- [ ] Unit tests with git test repos

**Acceptance:** agent can commit, push, create PR, update description, merge —
all idempotent on retry.

### M13 — Worktree Garbage Collection `pending`

Orphan detection and cleanup.

- [ ] GC trigger: PM startup + every 6 hours
- [ ] Orphan detection: no activity 48h + not in coder phase + no open PR
- [ ] Warn → wait 24h → archive reports → clean
- [ ] Restart recovery: reconcile local worktrees with Slack threads
- [ ] Unit tests with mock Slack thread history

**Acceptance:** orphan worktrees detected, warned, cleaned after grace period.

---

## Phase 5: System Prompts & Skills

Load agent seeds and skills. After this phase, agents can be configured
with their full personalities and custom commands.

### M14 — Seed Loading & Prompt Building `pending`

Read agent seed MDs and construct system prompts.

- [ ] Seed file reader: parse `seeds/<role>.md`
- [ ] System prompt builder: seed + `global.md` + `workflows.md` (PM only)
- [ ] Skill index builder: scan `skills/`, extract name + triggers + description
- [ ] Append skill index to PM's system prompt
- [ ] Hot-reload: detect file changes, rebuild prompt on next LLM call
- [ ] Exclude `## Archived Learnings` from system prompt
- [ ] Unit tests: prompt building with various seed combinations

**Acceptance:** system prompt built from seed + global + workflows + skill index.

### M15 — Skill Parser & Validator `pending`

Parse skill markdown files and validate them.

- [ ] Parse skill file: extract `# name`, description, `## Trigger`, `## Agent`, `## Prompt`
- [ ] Variable extraction: `{param}` from triggers, `{{param}}` from prompt
- [ ] Default values: `{{param | default: "value"}}`
- [ ] Validation: required sections, valid agent name, no duplicate triggers, no undefined variables
- [ ] `codebutler validate` command
- [ ] Unit tests with valid and invalid skill files in `testdata/`

**Acceptance:** skill files parse correctly, validation catches all error types
from SPEC §1.7.

---

## Phase 6: PM + Coder — First Working Flow

Two agents working together. After this phase, a user can describe a feature
in Slack and get a PR.

### M16 — PM Agent `pending`

The orchestrator. Receives user messages, classifies intent, interviews,
explores codebase, proposes plans, delegates.

- [ ] Intent classification: match workflows first, then skills, then ambiguous menu
- [ ] Workflow execution: implement, bugfix, question, refactor
- [ ] User interview loop (clarifying questions until plan is ready)
- [ ] Codebase exploration (Read, Grep, Glob)
- [ ] Plan proposal with file:line references
- [ ] User approval flow (Block Kit buttons)
- [ ] Delegation to Coder via `@codebutler.coder` in SendMessage
- [ ] Dynamic model routing: classify task complexity, assign Coder model
- [ ] Skill execution: resolve variables, route to target agent
- [ ] PM model pool + hot swap (`/pm claude`, `/pm kimi`)
- [ ] Integration test: user message → PM plan → approval

**Acceptance:** PM receives user message, classifies intent, interviews,
proposes plan, user approves, delegates to Coder with plan.

### M17 — Coder Agent `pending`

The builder. Receives task from PM, implements in worktree, creates PR.

- [ ] Receive task from PM (parse plan from @mention message)
- [ ] Implement in worktree (Write, Edit, Bash)
- [ ] Run test suite
- [ ] Ask PM when stuck (SendMessage with @codebutler.pm)
- [ ] Reasoning in thread at decision points
- [ ] Create PR (GitCommit, GitPush, GHCreatePR)
- [ ] Hand off to Reviewer (SendMessage with @codebutler.reviewer)
- [ ] Sandbox enforcement (path validation, command filtering)
- [ ] Integration test: PM plan → Coder implements → PR created

**Acceptance:** Coder receives plan, implements code, runs tests, creates PR,
hands off to Reviewer.

### M18 — End-to-End: User → PM → Coder → PR `pending`

Integration milestone. Full flow from user message to merged PR (without
review or retro).

- [ ] User posts feature request in Slack
- [ ] PM classifies, interviews, explores, proposes plan
- [ ] User approves plan
- [ ] PM delegates to Coder
- [ ] Coder implements in worktree, creates PR
- [ ] Verify: PR exists with correct changes, branch is clean

**Acceptance:** full implement workflow produces a correct PR from a Slack
message, end to end.

---

## Phase 7: Review & Learning

Add Reviewer and Lead. After this phase, the full implement workflow works
end-to-end including code review and retrospective.

### M19 — Reviewer Agent `pending`

Quality gate. Reviews PRs for security, quality, tests, plan compliance.

- [ ] Receive PR notification from Coder
- [ ] Read diff: `git diff main...<branch>`
- [ ] Structured review protocol:
  - Invariants list (what must not break)
  - Risk matrix (security, performance, compatibility, correctness)
  - Test plan (what tests should exist)
- [ ] Structured feedback with `[security]`, `[test]`, `[quality]` tags and file:line refs
- [ ] Review loop: send feedback → Coder fixes → re-review (max 3 rounds)
- [ ] Approve → notify Lead
- [ ] Escalate to Lead on disagreement
- [ ] Two-pass review optimization (cheap first pass, deep second if needed)
- [ ] Integration test: PR diff → structured review → feedback

**Acceptance:** Reviewer produces invariants + risk matrix + test plan,
structured feedback, loops with Coder, approves and notifies Lead.

### M20 — Lead Agent `pending`

Mediator and improvement driver. Runs retrospectives, evolves team.

- [ ] Mediation: read context when agents disagree, decide
- [ ] Retrospective phases:
  - Analysis: read full Slack thread, identify friction
  - Discussion: @mention agents, ask about issues
  - Proposals: structured output (3 well + 3 friction + 1 process + 1 prompt + 1 skill + 1 guardrail)
- [ ] Memory extraction: route learnings to correct agent MDs
- [ ] Learnings schema: when/rule/example/confidence/source
- [ ] Learnings pruning: contradiction removal, stale archival, token cap
- [ ] Global.md updates: architecture, conventions, decisions
- [ ] Workflows.md updates: new steps, new workflows
- [ ] Thread report generation (`.codebutler/reports/<thread-ts>.json`)
- [ ] PR description update via `gh pr edit`
- [ ] Usage report: token/cost breakdown per agent
- [ ] Integration test: mock thread → retrospective → proposals

**Acceptance:** Lead reads thread, produces structured retrospective, proposes
learnings to correct files, generates thread report.

### M21 — Full Implement Workflow E2E `pending`

Complete workflow: user → PM → Coder → Reviewer → Lead → merge.

- [ ] User → PM plans → Coder implements → PR created
- [ ] Reviewer reviews → feedback loop with Coder → approved
- [ ] Lead retrospective → learnings proposed → user approves
- [ ] PR merged, worktree cleaned, branch deleted
- [ ] Thread report saved

**Acceptance:** full implement workflow from Slack message to merged PR with
review and retrospective.

---

## Phase 8: Support Agents

Researcher and Artist. After this phase, all six agents are operational.

### M22 — Researcher Agent `pending`

Web research on demand from any agent.

- [ ] WebSearch tool implementation
- [ ] WebFetch tool implementation
- [ ] Receive @mention from any agent (not just PM)
- [ ] Check existing research in `.codebutler/research/` before searching
- [ ] Synthesize findings in structured format
- [ ] Persist findings to `.codebutler/research/<topic>.md`
- [ ] Update Research Index in `global.md`
- [ ] Integration test: agent asks question → Researcher searches → returns findings

**Acceptance:** any agent can @mention Researcher, get structured findings,
findings persisted and indexed.

### M23 — Artist Agent `pending`

UI/UX designer with image generation.

- [ ] UX reasoning via Claude Sonnet (OpenRouter)
- [ ] Image generation via OpenAI gpt-image-1 (direct API)
- [ ] OpenAI client for image gen/edit (`provider/openai/images.go`)
- [ ] Design proposal format (layout, components, interaction, responsive, notes for Coder)
- [ ] Read existing UI patterns from `artist/assets/`
- [ ] Save generated images to `.codebutler/images/`
- [ ] GenerateImage + EditImage tools
- [ ] Integration test: PM sends feature → Artist returns UX proposal

**Acceptance:** Artist receives feature request, produces UX proposal with
components + responsive behavior, generates images when needed.

---

## Phase 9: Advanced Features

MCP, multi-model, decision log, dynamic routing.

### M24 — MCP Integration `pending`

External tool servers via Model Context Protocol.

- [ ] Config parser: `.codebutler/mcp.json` with per-role filtering
- [ ] MCP client: stdio transport, protocol handshake, `tools/list`, `tools/call`
- [ ] Server lifecycle: spawn child process, discover tools, shutdown (SIGTERM → SIGKILL)
- [ ] Merged tool registry: native tools + MCP tools (native wins on name collision)
- [ ] Error handling: server crash, hang (30s timeout), startup failure
- [ ] Unit tests with mock MCP server

**Acceptance:** agent starts MCP servers for its role, discovers tools, routes
LLM tool calls to correct server, handles failures gracefully.

### M25 — Multi-Model Fan-Out `pending`

Parallel LLM calls to multiple models for brainstorm and other use cases.

- [ ] `MultiModelFanOut` tool: parallel single-shot calls via errgroup
- [ ] Model validation: all from pool, no duplicates, N ≤ maxAgentsPerRound
- [ ] Cost estimation before fan-out
- [ ] Error isolation: one failure doesn't cancel others
- [ ] Result aggregation: structured JSON with per-model responses
- [ ] Circuit breaker integration (per-model)
- [ ] Cost tracking: `FanOutCost` in `ThreadCost`
- [ ] Unit tests with mock multi-model provider

**Acceptance:** fan-out executes N parallel calls, handles partial failures,
tracks cost, respects model pool constraints.

### M26 — Decision Log `pending`

Structured decision recording for debugging and retrospective.

- [ ] `DecisionLogger`: append-only JSONL writer, thread-safe
- [ ] Decision types: workflow_selected, skill_matched, agent_delegated, model_selected,
      tool_chosen, stuck_detected, escalated, plan_deviated, review_issue,
      learning_proposed, compaction_triggered, circuit_breaker
- [ ] Inject logger into agent runner
- [ ] Lead reads decision log during retrospective
- [ ] Unit tests: write, read, concurrent writes

**Acceptance:** decisions logged to JSONL, Lead reads them for analysis.

---

## Phase 10: Workflows & Orchestration

Roadmap system, learn workflow, and unattended development.

### M27 — Roadmap System `pending`

Roadmap file management and roadmap-based workflows.

- [ ] Roadmap file parser (`.codebutler/roadmap.md` markdown format)
- [ ] roadmap-add workflow: PM interviews → creates roadmap items
- [ ] roadmap-implement workflow: PM reads item → runs implement workflow
- [ ] Status tracking: pending → in_progress → done → blocked
- [ ] Dependency resolution: build graph, identify unblocked items
- [ ] Integration test: add items → implement one → status updated

**Acceptance:** roadmap items added via conversation, implemented individually,
status tracked with dependencies.

### M28 — Develop Workflow (Unattended) `pending`

Batch execution of the entire roadmap.

- [ ] PM reads roadmap, builds dependency graph
- [ ] Launch independent items in parallel (respect `maxConcurrentThreads`)
- [ ] Create new Slack thread per item (1 thread = 1 branch = 1 PR)
- [ ] Orchestration thread: periodic status updates
- [ ] On item completion: check if dependents unblocked → launch them
- [ ] Failure handling: mark blocked, continue independent items, tag users
- [ ] User unblock: PM resumes automatically
- [ ] Integration test: roadmap with 3 items → parallel execution

**Acceptance:** PM orchestrates multiple threads, respects dependencies,
handles blocked items, reports progress.

### M29 — Learn Workflow `pending`

Onboarding and knowledge refresh.

- [ ] Auto-trigger on first run (existing codebase detected)
- [ ] Manual trigger: "re-learn" / "refresh knowledge"
- [ ] Phase 1: PM maps project (structure, features, domains)
- [ ] Phase 2: Technical agents in parallel (Coder, Reviewer, Artist) — each reads PM's map, explores from own perspective
- [ ] Phase 3: Lead synthesizes → populates `global.md`
- [ ] Researcher reactive during all phases
- [ ] Re-learn: compare with existing knowledge, compact (remove outdated, update changed, add new)
- [ ] User approves all MD changes
- [ ] Integration test: learn on a sample codebase

**Acceptance:** all agents explore codebase from their perspective, populate
project maps, Lead synthesizes to global.md, user approves.

---

## Phase 11: Setup & Operations

Init wizard, service management, CLI commands.

### M30 — `codebutler init` Wizard `pending`

First-time setup: tokens, repo config, services.

- [ ] Step 1: Global tokens (Slack bot+app tokens, OpenRouter key, OpenAI key)
  - Skip if `~/.codebutler/config.json` exists
  - Guide user through Slack app creation
- [ ] Step 2: Repo setup (seed `.codebutler/`, channel selection, `.gitignore`)
  - Skip if `.codebutler/` exists
  - Copy seeds, create config, create directories
- [ ] Step 3: Service install (select agents, detect OS, install services)
  - macOS: LaunchAgent plists
  - Linux: systemd user units
- [ ] Validation: check all required tokens, verify Slack connection

**Acceptance:** `codebutler init` in a fresh repo creates config, seeds
`.codebutler/`, installs services, starts agents.

### M31 — CLI Commands `pending`

Service management and validation.

- [ ] `codebutler configure` — change channel, add/remove agents, update tokens
- [ ] `codebutler start` — start all agents on this machine
- [ ] `codebutler stop` — stop all agents
- [ ] `codebutler status` — show running agents, active threads
- [ ] `codebutler validate` — check all skill files, config
- [ ] `codebutler --role <role>` — run single agent in foreground (dev mode)

**Acceptance:** all CLI commands work on macOS and Linux.

---

## Phase 12: Hardening & Polish

Production readiness. Cost controls, conflict handling, comprehensive testing.

### M32 — Token Budgets & Cost Controls `pending`

- [ ] Per-thread cost tracking (aggregate from all agents' ThreadCost)
- [ ] Per-thread budget: pause + ask user when exceeded
- [ ] Per-day budget: stop all agents, notify in channel
- [ ] Cost-aware planning: PM includes estimates in plans
- [ ] Cost display in thread reports

**Acceptance:** agents stop at budget limits, user can approve continuation.

### M33 — Conflict Detection & Merge Coordination `pending`

- [ ] File overlap detection between active threads
- [ ] Directory overlap detection
- [ ] Semantic overlap analysis (PM-driven)
- [ ] Merge ordering: PM suggests smallest-first
- [ ] Post-merge notification: other threads rebase
- [ ] Check at thread start + after each Coder response

**Acceptance:** overlapping threads detected, merge order suggested,
post-merge rebase notifications sent.

### M34 — Comprehensive Testing `pending`

- [ ] Unit tests for all packages (target: ≥80% coverage)
- [ ] Integration tests with mock OpenRouter + mock Slack
- [ ] Mock MCP server for MCP tests
- [ ] End-to-end test: full implement workflow with real Slack (manual)
- [ ] Benchmark: agent loop performance, tool execution latency
- [ ] CI pipeline: `go test ./...`, `go vet`, linting

**Acceptance:** all tests pass, CI green, manual E2E verified.

### M35 — Graceful Shutdown & Recovery `pending`

- [ ] SIGTERM → cancel root context → all goroutines wind down → wait → force exit
- [ ] On restart: reconcile worktrees with Slack threads
- [ ] Process unresponded @mentions from thread history
- [ ] Resume conversations from JSON files
- [ ] Service auto-restart on crash (systemd/launchd)

**Acceptance:** agent restarts cleanly, picks up pending work, no data loss.

---

## Dependency Graph

```
M1 ──→ M2 ──→ M3 ──→ M5 ──→ M6 ──→ M7
                │            │
                ↓            ↓
               M4 ──────→ M5
                            │
                            ↓
              M8 ──→ M9 ──→ M10
                      │
                      ↓
             M11 ──→ M12 ──→ M13
                      │
                      ↓
             M14 ──→ M15
                      │
                      ↓
             M16 ──→ M17 ──→ M18
                               │
                               ↓
                      M19 ──→ M20 ──→ M21
                                       │
                                       ↓
                              M22    M23
                               │      │
                               ↓      ↓
                      M24   M25   M26
                               │
                               ↓
                      M27 ──→ M28
                               │
                               ↓
                              M29
                               │
                               ↓
                      M30 ──→ M31
                               │
                               ↓
                M32   M33   M34   M35
```

Core path: M1→M2→M3→M4→M5→M6→M7→M8→M9→M16→M17→M18 (first working flow).

---

## Notes

- Each milestone is designed to be independently testable.
- Milestones within a phase can sometimes be parallelized (e.g., M4 and M3).
- The "Acceptance" field defines the minimum bar for completion.
- Extractable packages (go-openrouter, go-mcp, go-agentloop) are built inside
  `internal/` first and extracted once the interface stabilizes (see
  ARCHITECTURE.md "Extract, Don't Embed").

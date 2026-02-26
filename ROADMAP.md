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

### M10 — Block Kit & Interactions `done`

Interactive messages for approval flows.

- [x] Block Kit message builder (buttons for approve/reject/modify)
- [x] Interaction event handler (button clicks → resume agent loop)
- [x] Emoji reaction handler (🛑 = stop agent, 👍 = approve)
- [x] Fallback to plain text when Block Kit unavailable
- [x] Integration test with Slack

**Acceptance:** PM can post a plan with Approve/Reject buttons, user clicks,
agent receives the choice and continues.

---

## Phase 4: Git & Worktree

Isolated workspaces for each thread. After this phase, agents can create
branches, commit, push, and create PRs.

### M11 — Worktree Management `done`

Create, initialize, and remove git worktrees.

- [x] `Create(branchName)` — `git worktree add`, push branch to remote
- [x] `Remove(branchName)` — `git worktree remove`, delete remote branch
- [x] Per-platform init (Go: nothing, Node: `npm ci`, Python: venv, etc.)
- [x] Branch naming: `codebutler/<slug>` from PM's classification
- [x] Path: `.codebutler/branches/<branchName>/`
- [x] Unit tests: create, verify, remove

**Acceptance:** worktree created with correct branch, initialized per platform,
cleanup removes worktree + remote branch.

### M12 — Git & GitHub Tools `done`

Git and GitHub operations as agent tools.

- [x] GitCommit tool: stage files, commit (check for already-applied)
- [x] GitPush tool: push branch (idempotent if remote up to date)
- [x] GHCreatePR tool: create PR via `gh` CLI (skip if PR exists for branch)
- [x] Git sync protocol: commit+push after every change, pull before reading shared state
- [x] `gh pr edit` for PR description updates
- [x] `gh pr merge --squash` for merge
- [x] Unit tests with git test repos

**Acceptance:** agent can commit, push, create PR, update description, merge —
all idempotent on retry.

### M13 — Worktree Garbage Collection `done`

Orphan detection and cleanup.

- [x] GC trigger: PM startup + every 6 hours
- [x] Orphan detection: no activity 48h + not in coder phase + no open PR
- [x] Warn → wait 24h → archive reports → clean
- [x] Restart recovery: reconcile local worktrees with Slack threads
- [x] Unit tests with mock Slack thread history

**Acceptance:** orphan worktrees detected, warned, cleaned after grace period.

---

## Phase 5: System Prompts & Skills

Load agent seeds and skills. After this phase, agents can be configured
with their full personalities and custom commands.

### M14 — Seed Loading & Prompt Building `done`

Read agent seed MDs and construct system prompts.

- [x] Seed file reader: parse `seeds/<role>.md`
- [x] System prompt builder: seed + `global.md` + `workflows.md` (PM only)
- [x] Skill index builder: scan `skills/`, extract name + triggers + description
- [x] Append skill index to PM's system prompt
- [x] Hot-reload: detect file changes, rebuild prompt on next LLM call
- [x] Exclude `## Archived Learnings` from system prompt
- [x] Unit tests: prompt building with various seed combinations

**Acceptance:** system prompt built from seed + global + workflows + skill index.

### M15 — Skill Parser & Validator `done`

Parse skill markdown files and validate them.

- [x] Parse skill file: extract `# name`, description, `## Trigger`, `## Agent`, `## Prompt`
- [x] Variable extraction: `{param}` from triggers, `{{param}}` from prompt
- [x] Default values: `{{param | default: "value"}}`
- [x] Validation: required sections, valid agent name, no duplicate triggers, no undefined variables
- [x] `codebutler validate` command
- [x] Unit tests with valid and invalid skill files in `testdata/`

**Acceptance:** skill files parse correctly, validation catches all error types
from SPEC §1.7.

---

## Phase 6: PM + Coder — First Working Flow

Two agents working together. After this phase, a user can describe a feature
in Slack and get a PR.

### M16 — PM Agent `done`

The orchestrator. Receives user messages, classifies intent, interviews,
explores codebase, proposes plans, delegates.

- [x] Intent classification: match workflows first, then skills, then ambiguous menu
- [x] Workflow execution: implement, bugfix, question, refactor
- [x] User interview loop (clarifying questions until plan is ready)
- [x] Codebase exploration (Read, Grep, Glob)
- [x] Plan proposal with file:line references
- [x] User approval flow (Block Kit buttons)
- [x] Delegation to Coder via `@codebutler.coder` in SendMessage
- [x] Dynamic model routing: classify task complexity, assign Coder model
- [x] Skill execution: resolve variables, route to target agent
- [x] PM model pool + hot swap (`/pm claude`, `/pm kimi`)
- [x] Integration test: user message → PM plan → approval

**Acceptance:** PM receives user message, classifies intent, interviews,
proposes plan, user approves, delegates to Coder with plan.

### M17 — Coder Agent `done`

The builder. Receives task from PM, implements in worktree, creates PR.

- [x] Receive task from PM (parse plan from @mention message)
- [x] Implement in worktree (Write, Edit, Bash)
- [x] Run test suite
- [x] Ask PM when stuck (SendMessage with @codebutler.pm)
- [x] Reasoning in thread at decision points
- [x] Create PR (GitCommit, GitPush, GHCreatePR)
- [x] Hand off to Reviewer (SendMessage with @codebutler.reviewer)
- [x] Sandbox enforcement (path validation, command filtering)
- [x] Integration test: PM plan → Coder implements → PR created

**Acceptance:** Coder receives plan, implements code, runs tests, creates PR,
hands off to Reviewer.

### M18 — End-to-End: User → PM → Coder → PR `done`

Integration milestone. Full flow from user message to merged PR (without
review or retro).

- [x] User posts feature request in Slack
- [x] PM classifies, interviews, explores, proposes plan
- [x] User approves plan
- [x] PM delegates to Coder
- [x] Coder implements in worktree, creates PR
- [x] Verify: PR exists with correct changes, branch is clean

**Acceptance:** full implement workflow produces a correct PR from a Slack
message, end to end.

---

## Phase 7: Review & Learning

Add Reviewer and Lead. After this phase, the full implement workflow works
end-to-end including code review and retrospective.

### M19 — Reviewer Agent `done`

Quality gate. Reviews PRs for security, quality, tests, plan compliance.

- [x] Receive PR notification from Coder
- [x] Read diff: `git diff main...<branch>`
- [x] Structured review protocol:
  - Invariants list (what must not break)
  - Risk matrix (security, performance, compatibility, correctness)
  - Test plan (what tests should exist)
- [x] Structured feedback with `[security]`, `[test]`, `[quality]` tags and file:line refs
- [x] Review loop: send feedback → Coder fixes → re-review (max 3 rounds)
- [x] Approve → notify Lead
- [x] Escalate to Lead on disagreement
- [x] Two-pass review optimization (cheap first pass, deep second if needed)
- [x] Integration test: PR diff → structured review → feedback

**Acceptance:** Reviewer produces invariants + risk matrix + test plan,
structured feedback, loops with Coder, approves and notifies Lead.

### M20 — Lead Agent `done`

Mediator and improvement driver. Runs retrospectives, evolves team.

- [x] Mediation: read context when agents disagree, decide
- [x] Retrospective phases:
  - Analysis: read full Slack thread, identify friction
  - Discussion: @mention agents, ask about issues
  - Proposals: structured output (3 well + 3 friction + 1 process + 1 prompt + 1 skill + 1 guardrail)
- [x] Memory extraction: route learnings to correct agent MDs
- [x] Learnings schema: when/rule/example/confidence/source
- [x] Learnings pruning: contradiction removal, stale archival, token cap
- [x] Global.md updates: architecture, conventions, decisions
- [x] Workflows.md updates: new steps, new workflows
- [x] Thread report generation (`.codebutler/reports/<thread-ts>.json`)
- [x] PR description update via `gh pr edit`
- [x] Usage report: token/cost breakdown per agent
- [x] Integration test: mock thread → retrospective → proposals

**Acceptance:** Lead reads thread, produces structured retrospective, proposes
learnings to correct files, generates thread report.

### M21 — Full Implement Workflow E2E `done`

Complete workflow: user → PM → Coder → Reviewer → Lead → merge.

- [x] User → PM plans → Coder implements → PR created
- [x] Reviewer reviews → feedback loop with Coder → approved
- [x] Lead retrospective → learnings proposed → user approves
- [x] PR merged, worktree cleaned, branch deleted
- [x] Thread report saved

**Acceptance:** full implement workflow from Slack message to merged PR with
review and retrospective.

---

## Phase 8: Support Agents

Researcher and Artist. After this phase, all six agents are operational.

### M22 — Researcher Agent `done`

Web research on demand from any agent.

- [x] WebSearch tool implementation
- [x] WebFetch tool implementation
- [x] Receive @mention from any agent (not just PM)
- [x] Check existing research in `.codebutler/research/` before searching
- [x] Synthesize findings in structured format
- [x] Persist findings to `.codebutler/research/<topic>.md`
- [x] Update Research Index in `global.md`
- [x] Integration test: agent asks question → Researcher searches → returns findings

**Acceptance:** any agent can @mention Researcher, get structured findings,
findings persisted and indexed.

### M23 — Artist Agent `done`

UI/UX designer with image generation.

- [x] UX reasoning via Claude Sonnet (OpenRouter)
- [x] Image generation via OpenAI gpt-image-1 (direct API)
- [x] OpenAI client for image gen/edit (`provider/openai/images.go`)
- [x] Design proposal format (layout, components, interaction, responsive, notes for Coder)
- [x] Read existing UI patterns from `artist/assets/`
- [x] Save generated images to `.codebutler/images/`
- [x] GenerateImage + EditImage tools
- [x] Integration test: PM sends feature → Artist returns UX proposal

**Acceptance:** Artist receives feature request, produces UX proposal with
components + responsive behavior, generates images when needed.

---

## Phase 9: Advanced Features

MCP, multi-model, decision log, dynamic routing.

### M24 — MCP Integration `done`

External tool servers via Model Context Protocol.

- [x] Config parser: `.codebutler/mcp.json` with per-role filtering
- [x] MCP client: stdio transport, protocol handshake, `tools/list`, `tools/call`
- [x] Server lifecycle: spawn child process, discover tools, shutdown (SIGTERM → SIGKILL)
- [x] Merged tool registry: native tools + MCP tools (native wins on name collision)
- [x] Error handling: server crash, hang (30s timeout), startup failure
- [x] Unit tests with mock MCP server

**Acceptance:** agent starts MCP servers for its role, discovers tools, routes
LLM tool calls to correct server, handles failures gracefully.

### M25 — Multi-Model Fan-Out `done`

Parallel LLM calls to multiple models for brainstorm and other use cases.

- [x] `MultiModelFanOut` tool: parallel single-shot calls via errgroup
- [x] Model validation: all from pool, no duplicates, N ≤ maxAgentsPerRound
- [x] Cost estimation before fan-out
- [x] Error isolation: one failure doesn't cancel others
- [x] Result aggregation: structured JSON with per-model responses
- [x] Circuit breaker integration (per-model)
- [x] Cost tracking: `FanOutCost` in `ThreadCost`
- [x] Unit tests with mock multi-model provider

**Acceptance:** fan-out executes N parallel calls, handles partial failures,
tracks cost, respects model pool constraints.

### M26 — Decision Log `done`

Structured decision recording for debugging and retrospective.

- [x] `DecisionLogger`: append-only JSONL writer, thread-safe
- [x] Decision types: workflow_selected, skill_matched, agent_delegated, model_selected,
      tool_chosen, stuck_detected, escalated, plan_deviated, review_issue,
      learning_proposed, compaction_triggered, circuit_breaker
- [x] Inject logger into agent runner
- [x] Lead reads decision log during retrospective
- [x] Unit tests: write, read, concurrent writes

**Acceptance:** decisions logged to JSONL, Lead reads them for analysis.

---

## Phase 10: Workflows & Orchestration

Roadmap system, learn workflow, and unattended development.

### M27 — Roadmap System `done`

Roadmap file management and roadmap-based workflows.

- [x] Roadmap file parser (`.codebutler/roadmap.md` markdown format)
- [x] roadmap-add workflow: PM interviews → creates roadmap items
- [x] roadmap-implement workflow: PM reads item → runs implement workflow
- [x] Status tracking: pending → in_progress → done → blocked
- [x] Dependency resolution: build graph, identify unblocked items
- [x] Integration test: add items → implement one → status updated

**Acceptance:** roadmap items added via conversation, implemented individually,
status tracked with dependencies.

### M28 — Develop Workflow (Unattended) `done`

Batch execution of the entire roadmap.

- [x] PM reads roadmap, builds dependency graph
- [x] Launch independent items in parallel (respect `maxConcurrentThreads`)
- [x] Create new Slack thread per item (1 thread = 1 branch = 1 PR)
- [x] Orchestration thread: periodic status updates
- [x] On item completion: check if dependents unblocked → launch them
- [x] Failure handling: mark blocked, continue independent items, tag users
- [x] User unblock: PM resumes automatically
- [x] Integration test: roadmap with 3 items → parallel execution

**Acceptance:** PM orchestrates multiple threads, respects dependencies,
handles blocked items, reports progress.

### M29 — Learn Workflow `done`

Onboarding and knowledge refresh.

- [x] Auto-trigger on first run (existing codebase detected)
- [x] Manual trigger: "re-learn" / "refresh knowledge"
- [x] Phase 1: PM maps project (structure, features, domains)
- [x] Phase 2: Technical agents in parallel (Coder, Reviewer, Artist) — each reads PM's map, explores from own perspective
- [x] Phase 3: Lead synthesizes → populates `global.md`
- [x] Researcher reactive during all phases
- [x] Re-learn: compare with existing knowledge, compact (remove outdated, update changed, add new)
- [x] User approves all MD changes
- [x] Integration test: learn on a sample codebase

**Acceptance:** all agents explore codebase from their perspective, populate
project maps, Lead synthesizes to global.md, user approves.

---

## Phase 11: Setup & Operations

Init wizard, service management, CLI commands.

### M30 — `codebutler init` Wizard `done`

First-time setup: tokens, repo config, services.

- [x] Step 1: Global tokens (Slack bot+app tokens, OpenRouter key, OpenAI key)
  - Skip if `~/.codebutler/config.json` exists
  - Guide user through Slack app creation
- [x] Step 2: Repo setup (seed `.codebutler/`, channel selection, `.gitignore`)
  - Skip if `.codebutler/` exists
  - Copy seeds, create config, create directories
- [x] Step 3: Service install (select agents, detect OS, install services)
  - macOS: LaunchAgent plists
  - Linux: systemd user units
- [x] Validation: check all required tokens, verify Slack connection

**Acceptance:** `codebutler init` in a fresh repo creates config, seeds
`.codebutler/`, installs services, starts agents.

### M31 — CLI Commands `done`

Service management and validation.

- [x] `codebutler configure` — change channel, add/remove agents, update tokens
- [x] `codebutler start` — start all agents on this machine
- [x] `codebutler stop` — stop all agents
- [x] `codebutler status` — show running agents, active threads
- [x] `codebutler validate` — check all skill files, config
- [x] `codebutler --role <role>` — run single agent in foreground (dev mode)

**Acceptance:** all CLI commands work on macOS and Linux.

---

## Phase 12: Hardening & Polish

Production readiness. Cost controls, conflict handling, comprehensive testing.

### M32 — Token Budgets & Cost Controls `done`

- [x] Per-thread cost tracking (aggregate from all agents' ThreadCost)
- [x] Per-thread budget: pause + ask user when exceeded
- [x] Per-day budget: stop all agents, notify in channel
- [x] Cost-aware planning: PM includes estimates in plans
- [x] Cost display in thread reports

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

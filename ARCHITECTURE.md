# Architecture

Code-level implementation details. Not part of any agent's system prompt.

## Message Routing

When a new Slack message arrives, the daemon filters it **before** calling any model:

```
for each agent:
    if agent.shouldProcess(message):
        agent.process(message)
```

Filter rules (string match, no model involved):
- **PM**: process if message contains `@codebutler.pm` OR message contains NO `@codebutler.*` mention
- **All other agents**: process only if message contains `@codebutler.<their-role>`
- A single message can match multiple agents (e.g., `@codebutler.coder @codebutler.reviewer` routes to both)

If a message doesn't match an agent's filter → no model call, zero tokens.

## Conversation Persistence

Each agent manages its own conversation with the model. The daemon does NOT maintain one shared conversation — each agent has its own.

**Conversation files live in the thread's worktree:**

```
.codebutler/branches/<branchName>/conversations/
  pm.json
  coder.json
  reviewer.json
  lead.json
  artist.json
  researcher.json
```

Each file stores the **complete agent↔model transcript**: system prompt, user messages, assistant responses, tool calls, tool results, retries. This is NOT the same as what appears in Slack — most tool calls and intermediate reasoning stay private to the agent.

### Two layers of state

| Layer | What it holds | Who sees it | Lifetime |
|-------|--------------|-------------|----------|
| **Slack thread** | Curated agent outputs, user messages, inter-agent @mentions | Everyone (user + all agents) | Permanent (Slack retention) |
| **Conversation file** | Full model transcript (tool calls, results, reasoning, retries) | Only the owning agent (+ Lead for deep analysis) | Worktree lifetime |

The agent decides what to post to Slack. The model may return 20 tool-call rounds, but only the final "PR ready" or "here's the plan" goes to the thread.

### Processing a message

When an agent processes a new message:
1. Load `conversations/<role>.json` from the worktree (create if first message)
2. Append the new user/agent message to the conversation
3. Call the model with the full conversation
4. Model responds — could be a tool call or a text response
5. If tool call: execute tool, append result, go to 3 (agent loop)
6. If text response: append to conversation, decide whether to post to Slack
7. Save conversation file after each model round (crash-safe)

### Agent restart

Conversation files are the source of truth for model context. If an agent restarts mid-task, it loads the conversation file and resumes from the last saved round — no context lost, no need to replay from Slack.

### Thread lifecycle

When a thread ends (Lead completes retrospective), the worktree is deleted — conversation files go with it. If deep analysis is needed later, the Lead can archive them before cleanup.

## Agent System Prompts

Each agent's system prompt is built from:
1. `seeds/<role>.md` — agent-specific identity, personality, tools, rules
2. `seeds/global.md` — shared project knowledge (architecture, conventions, decisions)
3. `seeds/workflows.md` — available workflows (PM only, or all agents if relevant)
4. Skill index from `skills/` — PM only: skill names, triggers, descriptions (for intent classification)

The seed files are read once at startup. If the Lead updates them (after user approval), the changes take effect on the next model call (the updated file is loaded into the conversation).

## Skills

Custom commands defined as markdown files in `.codebutler/skills/`. Parsed at startup, re-read when files change.

### Skill Index (PM System Prompt)

At startup, the PM process scans `skills/` and builds a skill index appended to its system prompt:

```
Available skills:
- deploy: Deploy the project to an environment. Triggers: deploy, deploy to {environment}
- db-migrate: Run database migrations. Triggers: migrate, run migrations, db migrate
- changelog: Generate changelog entry. Triggers: changelog, what changed, release notes
```

The PM uses this index during intent classification — it's just text in the prompt, the model matches naturally.

### Skill Execution

```
1. PM classifies user message:
   a. Match workflows first (implement, bugfix, etc.)
   b. If no workflow match → check skill triggers
   c. If no skill match → ambiguous → present options
2. PM reads the matched skill file (skills/<name>.md)
3. PM extracts variables from user message using trigger pattern
4. PM resolves {{variables}} in the skill's prompt
5. PM @mentions the skill's target agent with the resolved prompt
6. Target agent executes the prompt (standard agent loop)
7. If code changes → Reviewer + Lead (standard flow)
8. If no code changes → agent reports result, done
```

### Trigger Matching

Trigger matching is done by the LLM (PM), not by regex. The skill index in the system prompt gives the PM enough context to match user intent to skills. The `{param}` syntax in triggers is a hint to the PM about what to extract — the PM resolves it naturally from the user's message.

### Package

```
internal/
  skills/
    loader.go       # Scan skills/, parse markdown, build index
    parser.go       # Parse skill file: name, triggers, agent, prompt, variables
```

## Per-Process Event Loop

Every agent process runs the same loop:

```
1. Connect to Slack via Socket Mode
2. Receive event from Slack
3. Filter: is this message directed at me? (@mention or user message for PM)
4. Extract thread_ts
5. Route to existing thread goroutine, or spawn new one
6. Thread goroutine: read thread history from Slack → run agent loop → post response
```

Each process has its own thread registry (`map[string]*ThreadWorker`). The PM process has goroutines for every active thread. The Coder process has goroutines only for threads where it's been @mentioned.

Thread goroutines are ephemeral — die after 60s of inactivity (~2KB stack). On next @mention, a new goroutine reads the thread from Slack to rebuild context. Panic recovery per goroutine.

## MCP Server Management

Each agent process manages its own MCP server child processes. No shared MCP state between agents.

### Startup

```
1. Read .codebutler/mcp.json
2. Filter servers by current agent's role
3. For each matching server:
   a. Resolve ${VAR} in env from os.Environ()
   b. Spawn child process (command + args, stdio transport)
   c. Initialize MCP connection (protocol handshake)
   d. Call tools/list → get available tools with schemas
   e. Add tools to agent's tool registry alongside native tools
4. If a server fails to start → log warning, continue without it
```

### Tool Routing

The agent's tool registry holds both native and MCP tools. When the LLM returns a tool call:

```
1. Look up tool name in registry
2. If native tool → execute locally (existing path)
3. If MCP tool → route to the owning MCP server process:
   a. Send tools/call via stdio
   b. Wait for response (with timeout)
   c. Return result to LLM
4. If unknown tool → return error to LLM
```

Tool names must be unique across all sources. If an MCP server exposes a tool with the same name as a native tool, the native tool wins and a warning is logged.

### Shutdown

When the agent process exits (graceful or crash):
- Send SIGTERM to all MCP child processes
- Wait up to 5s for each to exit
- SIGKILL any that remain

### Error Handling

| Failure | Recovery |
|---------|----------|
| MCP server crashes mid-session | Log error, remove its tools from registry, continue without them. Post warning in thread if a tool call was in flight |
| MCP server hangs on tool call | Timeout (30s default, configurable), return error to LLM, LLM retries or uses alternative |
| MCP server fails to start | Log warning, agent starts without those tools. Not fatal |
| Invalid mcp.json | Agent starts with zero MCP tools. Log error |

### Package

```
internal/
  mcp/
    manager.go      # Lifecycle: start servers, stop, restart
    client.go       # MCP protocol client (stdio transport, tools/list, tools/call)
    registry.go     # Merged tool registry (native + MCP)
    config.go       # Parse mcp.json, filter by role, resolve env vars
```

## Agent Interface

```go
type Agent interface {
    Run(ctx context.Context, task Task) (Result, error)
    Resume(ctx context.Context, id string) (Result, error)
    SendMessage(ctx context.Context, msg Message) error
    Name() string
}
```

Same `AgentRunner` struct parameterized by config. Shared OpenRouter client. Standard loop: load prompt → LLM call → tool use → execute → append → repeat.

## Go Guidelines

Design principles: pure functions, dependency injection via interfaces, goroutines everywhere, structured logging. The goal is code that's trivially testable without mocks when possible, and with simple interface mocks when not.

### Pure Functions First

Default to pure functions — data in, data out, no side effects. Push I/O and state to the edges; keep the core logic functional.

```go
// Good: pure function, trivially testable
func ClassifyIntent(message string, workflows []Workflow, skills []Skill) Intent {
    // deterministic: same input → same output
}

// Good: pure transformation
func BuildSystemPrompt(seed string, global string, projectMap string) string {
    return seed + "\n\n" + global + "\n\n" + projectMap
}

// Good: pure parsing
func ParseSkillFile(content []byte) (Skill, error) {
    // parses markdown, returns structured data
}
```

Side effects (Slack calls, LLM calls, file I/O, git operations) live in thin adapter functions at the boundary. The core logic never imports `net/http` or `os` — it receives data and returns data.

```go
// Boundary: thin adapter that does I/O
func (s *SlackClient) PostMessage(channel, text string) error { ... }

// Core: pure decision logic
func FormatAgentResponse(result AgentResult, maxLines int) string { ... }
```

### Dependency Injection via Interfaces

No DI framework. Go interfaces are the mechanism. Define small interfaces where you need external behavior, accept them as constructor/function parameters.

```go
// Small, focused interfaces — one concern each
type LLMProvider interface {
    ChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type MessageSender interface {
    SendMessage(ctx context.Context, channel string, thread string, text string) error
}

type ToolExecutor interface {
    Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// Constructor receives dependencies — no globals, no init()
func NewAgentRunner(
    provider LLMProvider,
    sender MessageSender,
    executor ToolExecutor,
    config AgentConfig,
) *AgentRunner {
    return &AgentRunner{
        provider: provider,
        sender:   sender,
        executor: executor,
        config:   config,
    }
}
```

Rules:
- **Interfaces are defined by the consumer**, not the implementer. If `AgentRunner` needs to send messages, it defines `MessageSender` in its own package — it doesn't import a `slack.Interface`
- **No interface pollution.** If a function only needs one method, the interface has one method. Don't create `SlackClient` with 15 methods when you need `SendMessage`
- **Accept interfaces, return structs.** Constructors return concrete types; parameters accept interfaces
- **No `init()` functions.** All setup is explicit in `main()` or constructors. No hidden global state

### Testing Without Pain

Pure functions test with zero setup. Functions with dependencies test with simple struct mocks — no mock framework needed.

```go
// Testing a pure function: no setup, no mocks
func TestClassifyIntent(t *testing.T) {
    workflows := []Workflow{{Name: "implement", Triggers: []string{"build", "create"}}}
    skills := []Skill{{Name: "deploy", Triggers: []string{"deploy to"}}}

    got := ClassifyIntent("deploy to production", workflows, skills)

    assert.Equal(t, IntentSkill, got.Type)
    assert.Equal(t, "deploy", got.Skill.Name)
}

// Testing with dependencies: simple struct that satisfies the interface
type mockProvider struct {
    response ChatResponse
    err      error
}

func (m *mockProvider) ChatCompletion(_ context.Context, _ ChatRequest) (ChatResponse, error) {
    return m.response, m.err
}

func TestAgentLoop_ToolCall(t *testing.T) {
    provider := &mockProvider{response: ChatResponse{ToolCalls: [...]}}
    executor := &mockExecutor{result: ToolResult{Output: "file contents"}}
    runner := NewAgentRunner(provider, &discardSender{}, executor, defaultConfig)

    result, err := runner.Run(context.Background(), task)

    assert.NoError(t, err)
    assert.Equal(t, 1, executor.callCount)
}
```

Guidelines:
- **Table-driven tests** for pure functions with multiple cases
- **One mock per interface** — the mock is a struct with fields for the return values, defined in the test file. No `gomock`, no `testify/mock`
- **Test behavior, not implementation.** Assert on outputs and observable effects, not on internal method call order
- **`testdata/` directories** for fixture files (skill markdown, config JSON, conversation files)

### Goroutines — Maximize Concurrency

Goroutines are cheap. Use them aggressively for anything that can run in parallel. The architecture already demands this: multiple threads, multiple agents, parallel tool execution.

```go
// Per-thread goroutines with panic recovery
func (p *Process) handleThread(threadTS string, msg Message) {
    p.mu.Lock()
    worker, exists := p.threads[threadTS]
    if !exists {
        worker = NewThreadWorker(threadTS, p.agent)
        p.threads[threadTS] = worker
        go func() {
            defer func() {
                if r := recover(); r != nil {
                    p.log.Error("thread panicked", "thread", threadTS, "panic", r)
                }
            }()
            worker.Run()
        }()
    }
    p.mu.Unlock()
    worker.inbox <- msg
}
```

Patterns:
- **One goroutine per active thread.** Cheap (~2KB stack), dies on inactivity, respawns on next message
- **`context.Context` everywhere.** Every goroutine receives a context. Cancellation propagates top-down: process → thread → agent loop → tool execution
- **Channels for communication between goroutines.** No shared mutable state. Thread workers receive messages through a buffered channel
- **`errgroup` for parallel fan-out.** When multiple independent operations can run concurrently (parallel research requests, multi-file reads, learn workflow where all agents explore simultaneously)

```go
// Fan-out: parallel agent exploration during learn workflow
func (pm *PM) RunLearnWorkflow(ctx context.Context, agents []Agent) error {
    g, ctx := errgroup.WithContext(ctx)
    for _, agent := range agents {
        g.Go(func() error {
            return agent.Explore(ctx)
        })
    }
    return g.Wait()
}

// Fan-out: parallel tool execution when tools are independent
func (e *Executor) RunParallel(ctx context.Context, calls []ToolCall) []ToolResult {
    results := make([]ToolResult, len(calls))
    var wg sync.WaitGroup
    for i, call := range calls {
        wg.Add(1)
        go func() {
            defer wg.Done()
            results[i] = e.Execute(ctx, call)
        }()
    }
    wg.Wait()
    return results
}
```

- **`select` with context for timeouts.** Never block forever — LLM calls, MCP tool calls, Slack sends all have deadlines via context
- **Graceful shutdown.** `SIGTERM` → cancel root context → all goroutines wind down → wait with timeout → force exit

### Structured Logging

Every process logs with structured fields so you can follow exactly what's happening — which agent, which thread, which tool, how long it took.

```go
// Use log/slog (stdlib). No external logging library needed.
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))

// Every log line carries agent + thread context
log := logger.With("agent", role, "thread", threadTS)
log.Info("processing message", "from", msg.User, "mentions", msg.Mentions)
log.Info("llm call", "model", config.Model, "tokens_in", req.TokenCount)
log.Info("tool executed", "tool", call.Name, "duration", elapsed, "success", err == nil)
log.Warn("mcp server slow", "server", name, "duration", elapsed)
log.Error("tool failed", "tool", call.Name, "err", err)
```

Rules:
- **`log/slog` from stdlib.** No logrus, no zap, no zerolog. slog is good enough and has zero dependencies
- **Structured fields, not string interpolation.** `log.Info("msg", "key", val)` not `log.Info(fmt.Sprintf(...))`
- **Agent and thread in every log line.** Use `logger.With()` to attach context once, then pass the child logger down
- **Log at boundaries:** Slack message received, LLM call start/end, tool execution start/end, agent loop iteration, goroutine lifecycle (spawn/die). Not inside pure functions
- **Timing on I/O operations.** Every LLM call, tool execution, and Slack API call logs its duration
- **Log levels:** `Info` for normal flow (message received, tool executed, PR created). `Warn` for recoverable issues (MCP server slow, retry). `Error` for failures (LLM call failed, tool crashed). `Debug` for verbose tracing (full request/response bodies — off by default)

## Project Structure

```
cmd/codebutler/main.go          # Entrypoint, --role flag, setup wizard
internal/
  slack/                         # Slack client, handler, channels, snippets
  github/github.go               # PR detection, merge, description updates via gh
  models/interfaces.go           # Agent interface + types
  provider/
    openrouter/                  # Client, chat completions with tool-calling
    openai/images.go             # Image gen/edit (Artist)
  tools/                         # Definition, executor (sandboxed), loop (provider-agnostic)
  mcp/                           # MCP server lifecycle, client, merged tool registry
  skills/                        # Skill loader, parser, index builder
  router/router.go               # Message classifier (per-agent filter)
  conflicts/                     # Tracker, notify
  worktree/                      # Worktree create/init/remove (per-platform)
  config/                        # Global + per-repo config loading
```

## Testing Strategy

| Package | What to Test |
|---------|-------------|
| `internal/slack/snippets.go` | Code block extraction, routing |
| `internal/tools/executor.go` | Execution, sandboxing, limits |
| `internal/tools/loop.go` | Tool-calling loop, truncation |
| `internal/conflicts/tracker.go` | Overlap detection, merge ordering |
| `internal/github/github.go` | PR detection, merge polling |
| `internal/provider/openrouter/` | API client, error handling |
| `internal/worktree/` | Create, init, remove, isolation |
| `internal/mcp/` | Config parsing, role filtering, env resolution, tool routing, server lifecycle |
| `internal/skills/` | Skill file parsing, index building, variable extraction |

Integration: mock OpenRouter, mock MCP servers (stdio). E2E: real Slack (manual).

## Error Recovery

Each process is independent — one crash doesn't affect others.

| Failure | Recovery |
|---------|----------|
| Agent process crashes | Service restarts it. Reads active threads from Slack, processes unresponded @mentions |
| Slack disconnect | Auto-reconnect per process (SDK handles) |
| LLM call hangs | context.WithTimeout per goroutine → kill, reply error in thread |
| LLM call fails | Error reply in thread, session preserved for retry |
| Agent not running | @mention sits in thread. When agent starts, it reads thread and processes |
| Machine reboot | All 6 services restart, each reads active threads from Slack |

## Agent Loop Safety

The agent loop (prompt → LLM call → tool use → repeat) can run away. Three mechanisms prevent this.

### MaxTurns

Every agent has a hard cap on loop iterations per activation. When reached, the agent posts a summary of what it accomplished and what's left, then stops. The user (or orchestrating agent) can re-invoke it to continue.

| Agent | MaxTurns | Rationale |
|-------|----------|-----------|
| PM | 15 | Planning + exploration — shouldn't need more |
| Coder | 100 | Long implementations with many tool calls |
| Reviewer | 20 | Read diff + write feedback |
| Researcher | 10 | Web search + synthesis |
| Artist | 15 | UX reasoning + image gen |
| Lead | 30 | Reads thread + discusses with multiple agents |

Configurable per-repo in `config.json`. The loop checks `turn >= maxTurns` before every LLM call — never after, so it can't overshoot.

### Context Compaction

Conversations grow. When the token count approaches the model's context window (tracked from OpenRouter's response metadata), the agent compacts:

1. Keep the system prompt intact (never compacted)
2. Keep the last N tool calls + results verbatim (recent context matters most)
3. Summarize everything in between into a structured progress block:
   ```
   ## Progress so far
   - Read 12 files to understand auth module structure
   - Wrote JWT middleware in internal/auth/middleware.go
   - Tests passing (8/8)
   - Remaining: wire middleware into router, update config
   ```
4. Replace the old messages with the summary + recent messages
5. Continue the loop with a smaller, focused context

The summary is generated by the same model in a single-shot call with the instruction "summarize your progress so far for yourself — what you did, what you learned, what's left." This is appended as a user message, not as system prompt.

### Stuck Detection

If the agent makes 3 consecutive LLM calls that produce no observable progress (no new tool calls, no new Slack messages, same response pattern), it breaks the loop and posts to the thread: "I appear to be stuck. Here's what I was trying to do: [context]. Can you help?"

## LLM Error Classification

Not all LLM errors are the same. The OpenRouter client classifies each error and applies the right strategy:

| Error | Detection | Action |
|-------|-----------|--------|
| Rate limit | HTTP 429, `Retry-After` header | Wait for `Retry-After` duration + jitter, retry. Max 5 retries |
| Provider overloaded | HTTP 502, 503 | Exponential backoff (1s, 2s, 4s, 8s, 16s) + jitter. Max 5 retries |
| Context too long | HTTP 400 + `context_length_exceeded` | Compact conversation (see above), retry once |
| Content filtered | HTTP 400 + `content_filter` | Log, post to thread ("can't process this — content policy"), do not retry |
| Auth error | HTTP 401, 403 | Fail immediately, log error. Agent posts "configuration error — check API keys" |
| Malformed response | JSON parse failure on tool calls | Retry with error feedback in the next prompt: "Your previous response failed to parse: {error}. Please correct." Max 3 retries |
| Timeout | No response within deadline | Retry once. If second attempt also times out, check circuit breaker |
| Unknown | Anything else | Log full response, post error to thread, do not retry |

**Jitter is mandatory on retries.** Six agent processes hitting the same OpenRouter account will thundering-herd without it. Jitter formula: `delay * (0.5 + rand.Float64())`.

## Circuit Breaker

Wraps the OpenRouter client. Prevents burning through retries and money when the provider is down.

```
States:
  Closed (normal)  →  3 consecutive failures  →  Open (fast-fail)
  Open             →  30s timer expires        →  Half-Open (probe)
  Half-Open        →  1 success               →  Closed
  Half-Open        →  1 failure               →  Open (reset timer)
```

When the circuit is **open**, every LLM call returns immediately with an error — no HTTP request made. The agent posts to the thread: "LLM provider is temporarily unavailable, retrying in 30s." After 30s, the next call is a probe: if it succeeds, the circuit closes and the agent resumes normally.

**Per-model circuit breakers.** If the Coder's Opus model is down but Sonnet is fine, only the Coder's circuit opens. Other agents using Sonnet continue working.

**Fallback models.** When the circuit opens, the agent can optionally try a fallback model (configured per-agent in `config.json`). The PM already has a model pool; other agents can define a `fallbackModel` field.

Use `github.com/sony/gobreaker` — battle-tested, simple API, fits the interface pattern.

## Idempotent Tool Execution

When an agent crashes and restarts, it resumes from the last saved conversation round. This means the last tool call might execute twice. Side-effecting tools must handle this:

| Tool | Idempotency strategy |
|------|---------------------|
| **Read, Grep, Glob** | Pure reads — naturally idempotent |
| **Write** | Write to temp file + atomic rename. Re-execution produces the same file |
| **Edit** | Check if the edit was already applied (old_string not found = already done). Skip silently |
| **Bash** | Not idempotent by nature. For known-safe commands (test runners, linters), re-execution is fine. For mutations (package install, db migration), the agent should check state first |
| **GitCommit** | Check if a commit with the same content already exists on the branch. If so, skip |
| **GitPush** | Idempotent if the commit exists — push is a no-op when remote is up to date |
| **SendMessage** | Use the tool-call ID as an idempotency key. Before posting, check if a message with that key was already sent in the thread. Skip if found |
| **GHCreatePR** | Check if a PR already exists for the branch. If so, skip creation, return existing PR URL |

**Implementation:** each tool's `Execute` method checks for prior execution before performing the action. The tool-call ID (from the LLM response) is passed to every tool and stored with the result in the conversation file. On replay, the executor checks: "did I already execute tool-call-{id}?" → if yes, return the cached result.

## Cross-Agent Tracing

Six independent processes. One user request flows through PM → Coder → Reviewer → Lead. To debug the full flow, you need traces that span all agents.

### Trace Propagation via Slack

Slack messages are the wire protocol between agents. Embed trace context in every agent-to-agent message as a hidden metadata block:

```
@codebutler.coder implement: [plan details]
<!-- trace:abc123/span:def456 -->
```

The receiving agent extracts the trace/span IDs and creates a child span linked to the sender's trace. This produces end-to-end traces across the entire thread lifecycle:

```
Trace: thread-{threadTS}
  └─ PM: plan (3.2s)
       ├─ llm-call (2.1s, 1.2k tokens)
       ├─ tool:Glob (0.05s)
       └─ tool:Read x4 (0.3s)
  └─ Coder: implement (45s)
       ├─ llm-call (8.2s, 15k tokens)
       ├─ tool:Write x6 (0.1s)
       ├─ tool:Bash/test (12s)
       ├─ llm-call (5.1s, 8k tokens)  ← fix failing test
       └─ tool:GitCommit (0.3s)
  └─ Reviewer: review (6s)
  └─ Lead: retrospective (4s)
```

### What to Trace

Every span carries structured attributes:

| Span type | Key attributes |
|-----------|---------------|
| `agent-activation` | agent, thread, trigger_message_id |
| `llm-call` | model, input_tokens, output_tokens, cost, cache_hit |
| `tool-call` | tool_name, duration, success, args_hash |
| `slack-post` | channel, thread, message_length |
| `agent-loop` | turns_used, max_turns, compaction_triggered |

Use OpenTelemetry (`go.opentelemetry.io/otel`) with a local exporter (OTLP to a local collector or simple file export). No external tracing infrastructure required by default — a JSON file per thread with all spans is enough for debugging. Teams can plug in Jaeger, Datadog, etc. via the standard OTLP exporter.

## Token Budget & Cost Tracking

Every LLM call returns `usage.prompt_tokens`, `usage.completion_tokens`, and the actual model used (which may differ from requested if OpenRouter fell back). Track these and enforce limits.

### Tracking

Each agent maintains a running cost tally per thread:

```go
type ThreadCost struct {
    ThreadID       string
    Agent          string
    InputTokens    int64
    OutputTokens   int64
    CachedTokens   int64
    LLMCalls       int
    ToolCalls      int
    EstimatedCost  float64  // USD, calculated from model pricing
}
```

Updated after every LLM call. Persisted in the conversation file (survives restarts). The Lead reads all agents' costs during retrospective for the usage report.

### Budgets

Two levels of enforcement:

| Level | Config field | What happens when exceeded |
|-------|-------------|---------------------------|
| **Per-thread** | `limits.maxCostPerThread` | Agent stops, posts to thread: "Budget reached ($X spent). Approve to continue?" User replies → agent resumes with a fresh budget allocation |
| **Per-day** | `limits.maxCostPerDay` | All agents stop for the day. PM posts to channel: "Daily budget reached ($X). Resumes tomorrow or increase the limit" |

### Cost-Aware Decisions

The PM uses cost awareness in planning:
- Large tasks get a cost estimate in the plan ("estimated: ~$2.00 for Coder, ~$0.15 for Reviewer")
- The user sees the estimate before approving
- During `develop` (unattended), the PM monitors cumulative cost and pauses if the total exceeds the per-thread budget × number of active threads

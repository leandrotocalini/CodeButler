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

### Event Deduplication

Slack Socket Mode can deliver the same event more than once (network retries, reconnects, server-side replays). Each agent process maintains an in-memory set of recently processed `event_id`s (bounded, last 10,000 entries, ~5 minutes TTL). Before processing any event:

1. Extract `event_id` from the Slack envelope
2. Check the dedup set — if present, drop silently
3. If new, add to set, then proceed with routing

This is a simple `map[string]time.Time` with periodic eviction of expired entries. No persistence needed — on restart the agent reads unprocessed @mentions from the Slack thread history, which provides its own natural dedup (the agent checks the conversation file to see which messages it already processed).

**What this prevents:** duplicate tool executions, double-posted Slack messages, redundant LLM calls — all of which burn tokens and confuse the thread.

```
internal/
  slack/
    dedup.go        # Bounded set with TTL, thread-safe (sync.Map or mutex)
```

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
| **Slack thread** | Curated agent outputs, decision-point reasoning, user messages, inter-agent @mentions | Everyone (user + all agents) | Permanent (Slack retention) |
| **Conversation file** | Full model transcript (tool calls, results, retries) | Only the owning agent | Worktree lifetime |

The agent decides what to post to Slack. Tool calls and intermediate results stay private. **But reasoning at decision points goes to Slack** — the PM posts why it explores certain code and how findings shape the plan, the Coder posts implementation approach and deviations from the plan, the Lead posts observed patterns and evidence behind proposals. This is because the Slack thread is the Lead's source of truth for retrospectives. If reasoning stays only in the conversation file, the Lead can't learn from it and the team doesn't improve.

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

### Extract, Don't Embed

When building a tool or library to solve a problem, default to making it a standalone open-source package that CodeButler imports — not an internal module buried inside the project.

**The test:** if someone who doesn't use CodeButler could benefit from this package, it should be its own repo with its own go.mod.

Candidates for extraction:

| Package | What it does | Why it's generic |
|---------|-------------|-----------------|
| `gobreaker` wrapper / LLM circuit breaker | Circuit breaker tuned for LLM APIs (per-model, with fallback) | Any Go app calling LLMs needs this |
| Agent loop | prompt → LLM → tool calls → execute → repeat | The core loop is model-agnostic and provider-agnostic |
| OpenRouter client | Typed Go client for OpenRouter (chat completions, tool calling, streaming, model fallbacks) | No good Go client exists today |
| MCP client | MCP protocol over stdio (handshake, tools/list, tools/call) | Anyone building MCP-enabled Go tools needs this |
| Tool executor | Sandboxed tool execution (file ops, bash, git) with idempotency | Reusable for any agent framework |
| Conversation store | Append-only JSON conversation files with compaction | Generic for any LLM app that needs persistence |
| Skill parser | Parse markdown skill files (triggers, variables, prompts) | Useful for any agent system with custom commands |

**How it works in practice:**

```
github.com/leandrotocalini/go-openrouter    # standalone package
github.com/leandrotocalini/go-mcp           # standalone package
github.com/leandrotocalini/go-agentloop     # standalone package
github.com/leandrotocalini/codebutler       # imports the above
```

Rules:
- **No CodeButler-specific types in extracted packages.** The agent loop package doesn't know about Slack threads or agent MDs — it works with generic interfaces (`LLMProvider`, `ToolExecutor`, `ConversationStore`)
- **Own repo, own go.mod, own tests, own README.** Not a Go workspace monorepo — truly independent packages that can be versioned and imported separately
- **Extract when the abstraction is clear**, not preemptively. Build it inside `internal/` first, prove the interface works, then extract when it stabilizes. Premature extraction is worse than no extraction
- **CodeButler is the first consumer, not the only one.** Design the API as if you're publishing it for strangers. If the API only makes sense in the context of CodeButler, it's not ready to extract

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
  multimodel/                    # Multi-model fan-out, result collection, cost estimation
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
| `internal/multimodel/` | Fan-out execution, result aggregation, cost estimation, error isolation |

Integration: mock OpenRouter, mock MCP servers (stdio). E2E: real Slack (manual).

## Multi-Model Fan-Out

The `MultiModelFanOut` tool makes parallel single-shot LLM calls to different models via OpenRouter, each with a custom system prompt. Available to **all agents** — not just the PM. The brainstorm workflow is the primary consumer, but any agent can use it when multiple model perspectives would improve their output.

### Per-Agent Use Cases

| Agent | When to use | What it does |
|-------|------------|--------------|
| **PM** | `brainstorm` workflow | Creates dynamic Thinker personas, fans out a question, synthesizes ideas |
| **Reviewer** | Complex or security-critical PRs | Fans out the same diff to multiple models, each reviewing from a different angle (security, performance, logic, test coverage). Synthesizes into a single review with higher coverage than any one model |
| **Coder** | Stuck on implementation or choosing between approaches | Fans out "how would you implement X?" to multiple models, compares approaches, picks the best one. Like pair programming with 3 different senior devs |
| **Lead** | Retrospective analysis | Fans out the same thread summary to multiple models, each analyzing from a different angle. Surfaces patterns one model alone might miss |
| **Researcher** | Broad research questions | Fans out the same question to multiple models with different knowledge cutoffs and training data emphasis. Gets more comprehensive results |

**When NOT to use it:** routine operations, simple tasks, when the agent's primary model is sufficient. Multi-model fan-out costs N× a single call — agents should use it judiciously.

### MultiModelFanOut Tool

```go
// Tool definition (appears in every agent's tool list)
{
    Name: "MultiModelFanOut",
    Description: "Run parallel single-shot LLM calls to multiple models, each with a custom system prompt. Each call uses a different model — no duplicates. Returns all responses for the calling agent to synthesize.",
    Parameters: {
        "agents": [
            {
                "name": "string — display name (e.g., 'Security Reviewer', 'DeFi Expert')",
                "systemPrompt": "string — custom system prompt for this sub-agent",
                "model": "string — OpenRouter model ID from multiModel.models pool"
            }
        ],
        "userPrompt": "string — the question/context all sub-agents receive"
    }
}
```

### Execution

```
1. Agent calls MultiModelFanOut with N sub-agent configs + shared user prompt
2. Tool validates: all models are in multiModel.models pool, N <= maxAgents, no duplicate models (each sub-agent MUST use a different model — model diversity is the whole point)
3. Tool estimates cost (model pricing × estimated prompt tokens) — if exceeds maxCostPerRound, returns warning to the calling agent (agent decides whether to proceed or ask user)
4. Tool launches N goroutines via errgroup, each making a ChatCompletion call to OpenRouter:
   - Messages: [system: agent.systemPrompt, user: userPrompt]
   - No tools enabled (single-shot, no agent loop)
   - Timeout: 120s per call (reasoning models can be slow)
5. Each goroutine captures: response text, token usage, duration, model used, errors
6. errgroup.Wait() — all calls complete (or timeout)
7. Tool returns aggregated results to the calling agent as structured JSON
8. Agent processes results in its next turn (synthesis, posting to thread)
```

### Error Isolation

Each sub-agent is independent. If one model fails (429, timeout, content filter), the others continue.

```go
// Inside each goroutine — errors don't propagate
resp, err := provider.ChatCompletion(ctx, req)
if err != nil {
    results[i] = FanOutResult{
        Name:  a.Name,
        Model: a.Model,
        Error: err.Error(),
    }
    return nil // don't cancel errgroup
}
```

The calling agent receives all results including failures. It's responsible for noting which sub-agents failed ("3 of 4 models responded — Gemini timed out").

### Circuit Breaker Integration

Fan-out calls go through the same per-model circuit breakers as regular agent calls. If a model's circuit is open, the sub-agent returns immediately with an error — no wasted time waiting for a known-bad model. The calling agent can adapt or proceed with fewer results.

Fan-out failures do NOT open circuits for the agents' primary models. They're separate models in the pool.

### Cost Tracking

Each sub-agent's token usage is tracked and added to the thread's `ThreadCost`:

```go
type FanOutCost struct {
    Name         string
    Model        string
    InputTokens  int64
    OutputTokens int64
    EstimatedUSD float64
    Duration     time.Duration
}
```

The `ThreadCost.FanOutRounds` field accumulates across rounds for any agent that uses fan-out. The Lead includes these costs in the thread report — useful for understanding whether multi-model consultation pays for itself.

### Package

```
internal/
  multimodel/
    fanout.go       # MultiModelFanOut tool: parallel execution, result collection
    cost.go         # Cost estimation from model pricing tables
    types.go        # FanOutConfig, FanOutResult, FanOutCost
```

The multimodel package depends on `LLMProvider` interface (same as all agents) — no direct OpenRouter dependency. Testable with mock providers.

```go
func TestFanOut_PartialFailure(t *testing.T) {
    provider := &mockMultiModelProvider{
        responses: map[string]ChatResponse{
            "anthropic/claude-opus-4-6": {Content: "idea A"},
            "google/gemini-2.5-pro":     {Content: "idea B"},
        },
        errors: map[string]error{
            "openai/o3": fmt.Errorf("rate limited"),
        },
    }

    results := multimodel.FanOut(ctx, provider, agents, "what should we build?")

    assert.Equal(t, 3, len(results))
    assert.Empty(t, results[0].Error) // Claude succeeded
    assert.Empty(t, results[1].Error) // Gemini succeeded
    assert.Contains(t, results[2].Error, "rate limited") // o3 failed gracefully
}
```

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

The agent loop can enter unproductive cycles where the model keeps trying the same thing. Detection and escape happen at two levels: **code-level** (the runtime detects and intervenes) and **behavioral** (the agent's seed instructs it to self-monitor via reasoning-in-thread).

#### Detection Signals

The runtime tracks these signals across consecutive LLM calls:

| Signal | Detection | Threshold |
|--------|-----------|-----------|
| **Same tool + same params** | Hash tool name + args, compare to last N calls | 3 identical calls |
| **Same error repeated** | Compare error strings from tool results | 3 identical errors |
| **No progress** | No new tool calls, no new Slack messages, same response pattern | 3 consecutive calls |
| **Iteration without change** | Agent says "trying again" / "let me retry" but the next action is identical | 2 consecutive |

The runtime maintains a rolling window of the last 5 tool calls per agent activation. Detection runs before every LLM call — not after, so it can't overshoot.

#### Escape Strategies

When a loop is detected, the runtime applies escalating strategies:

1. **Inject reflection prompt** — append a system message to the conversation: "You appear to be in a loop. You've tried [X] three times with the same result. Stop and reflect: what have you tried so far, why didn't it work, and what fundamentally different approach could you take?" This costs one turn but often breaks the cycle
2. **Force a reasoning step** — if the reflection doesn't break the loop, inject: "List every approach you've tried and why each failed. Then propose an approach you haven't tried yet. If you can't think of one, say so." This prevents "trying again" without novelty
3. **Reduce available tools** — temporarily remove the tool that's being called in the loop from the agent's tool registry. The model is forced to find an alternative path. Restore tools after the agent makes progress
4. **Escalate** — if strategies 1–3 don't break the loop within 2 additional turns, the agent posts to the thread: "I'm stuck. Here's what I tried: [summary]. I need help." PM escalates to the user. Coder escalates to PM. Lead escalates to the user

Strategies are applied in order. Each strategy gets 2 turns to produce progress before escalating to the next. Total cost: max 6 extra turns before the user is asked for help.

#### What counts as "progress"

Progress means the agent's state has observably changed:
- A **new** tool call (different tool or different args)
- A Slack message posted
- A file written or edited
- A test that was failing now passes (or a new test that fails differently)

"Same tool, different file" counts as progress. "Same tool, same file, same args" does not.

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

## Worktree Garbage Collection

Worktrees accumulate. Normal cleanup happens when the user closes a thread (Lead deletes worktree + remote branch). But if the PM crashes before cleanup, a thread is abandoned, or the Lead's close sequence fails, orphan worktrees remain — consuming disk space and leaving stale branches on the remote.

### GC Trigger

The PM runs GC on startup and then periodically (every 6 hours, configurable). No separate daemon — the PM is always running and already manages worktree creation.

### Detection

A worktree is considered orphaned when **all** of the following are true:

1. No agent has posted in the thread for > 48 hours (configurable `gc.inactivityTimeout`)
2. The thread phase is not `coder` (don't GC while code is being written)
3. No open PR exists for the branch

The PM checks these conditions by:
- Reading Slack thread history (last message timestamp)
- Checking PR status via `gh pr list --head <branch>`
- Reading the thread's conversation files for last activity

### GC Actions

When an orphan is detected:

1. **Warn first** — PM posts in the thread: "This thread has been inactive for 48h. Reply to keep it open, or I'll clean up in 24h." Tag all thread participants
2. **Wait 24h** — if someone replies, reset the inactivity timer. If not, proceed
3. **Archive** — copy thread report + decision log to `.codebutler/reports/` on main (if not already there)
4. **Clean** — remove local worktree (`git worktree remove`), delete remote branch (`git push origin --delete`), close PR if still open

### Naming Convention

Worktree branches use deterministic names: `codebutler/<slug>` where `<slug>` is derived from the thread's first user message (PM generates it). This makes it easy to identify what a worktree is for without cross-referencing thread IDs.

### Recovery on Restart

When any agent process starts:
1. List all local worktrees (`git worktree list`)
2. For each worktree, check if the corresponding Slack thread still exists and has unprocessed @mentions
3. If the thread is gone (deleted, archived) → clean up the worktree immediately
4. If the thread exists but no pending work → leave it (GC will handle it if inactive)

### Package

```
internal/
  worktree/
    gc.go           # Orphan detection, warn/wait/clean cycle
    recovery.go     # Startup recovery: reconcile worktrees with Slack threads
```

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

## Thread Reports

After every retrospective, the Lead generates a structured report file. Reports accumulate over time and form the dataset for analyzing agent behavior across threads and projects.

### Report File

`.codebutler/reports/<thread-ts>.json` — one file per completed thread, committed to git. Survives worktree cleanup (Lead copies to main before delete).

### Structure

```json
{
  "thread_id": "1709312345.123456",
  "project": "my-app",
  "workflow": "implement",
  "timestamp": "2026-02-25T14:30:00Z",
  "duration_minutes": 45,
  "outcome": "pr_merged",
  "agents": {
    "pm": {
      "turns_used": 8,
      "max_turns": 15,
      "reasoning_messages": 4,
      "exploration_files_read": 6,
      "delegations": 1,
      "loops_detected": 0,
      "escalations": 0
    },
    "coder": {
      "turns_used": 34,
      "max_turns": 100,
      "reasoning_messages": 3,
      "plan_deviations": 1,
      "test_cycles": 2,
      "tool_failures": 0,
      "loops_detected": 0,
      "escalations": 0
    },
    "reviewer": {
      "turns_used": 5,
      "max_turns": 20,
      "review_rounds": 1,
      "issues_found": {"security": 1, "test": 1, "quality": 1}
    },
    "lead": {
      "turns_used": 12,
      "max_turns": 30,
      "learnings_proposed": 2,
      "learnings_approved": 2
    }
  },
  "cost": {
    "total_usd": 2.45,
    "by_agent": {"pm": 0.01, "coder": 2.20, "reviewer": 0.15, "lead": 0.09}
  },
  "patterns": [
    {
      "type": "exploration_gap",
      "agent": "pm",
      "description": "PM didn't discover test helper at testutil/, causing 4 extra Coder turns"
    },
    {
      "type": "plan_deviation",
      "agent": "coder",
      "description": "router.go was refactored since plan was made — stale file:line reference"
    }
  ]
}
```

### Two Data Sources

| Source | What it provides | Accuracy |
|--------|-----------------|----------|
| **Runtime metrics** | `turns_used`, `max_turns`, `tool_failures`, `loops_detected`, `escalations`, `cost`, `duration` | Exact — code-generated from `ThreadCost` and loop detector counters |
| **Lead analysis** | `reasoning_messages`, `plan_deviations`, `exploration_files_read`, `patterns`, `outcome` | Interpreted — the Lead reads the Slack thread and classifies |

Runtime metrics are injected into the report template automatically before the Lead starts its retrospective. The Lead fills in the qualitative fields during analysis. This separation means the ground-truth numbers are always accurate even if the Lead's interpretation is off.

### What Each Metric Tells You About CodeButler

| Metric | High value means | CodeButler improvement |
|--------|-----------------|----------------------|
| `turns_used / max_turns` | Agent hits budget frequently | Increase budget or optimize agent efficiency |
| `loops_detected` | Stuck detection fires often | Improve seed instructions or escape strategies |
| `reasoning_messages` (low) | Agent not following reasoning-in-thread | Strengthen seed instructions, check model compliance |
| `plan_deviations` | PM plans don't match reality | PM needs deeper exploration or stale-reference detection |
| `review_rounds` (high) | Coder output needs many fixes | Add Reviewer patterns to Coder seed or PM plan |
| `issues_found` by type | Same issue type repeats across threads | Add preventive check to PM exploration or Coder rules |
| `cost.by_agent` | One agent dominates cost | Optimize model choice or reduce unnecessary turns |
| Same `pattern.type` across reports | Systemic issue | Fix in CodeButler code, not just seeds |

### Aggregate Analysis

The `/behavior-report` skill reads all report files and produces cross-thread insights. See `seeds/skills/behavior-report.md`.

## Decision Log

Thread reports are post-hoc — they summarize what happened after the thread closes. The decision log captures **why** things happened in real-time. Every significant decision an agent makes is recorded as a structured entry, enabling debugging ("why did the PM pick this workflow?"), retrospective analysis by the Lead, and future heuristic training.

### Log File

`.codebutler/branches/<branchName>/decisions.jsonl` — one JSON line per decision, append-only. Lives in the worktree alongside conversation files. The Lead reads it during retrospective for evidence-based analysis.

### Entry Structure

```json
{
  "ts": "2026-02-25T14:30:12Z",
  "agent": "pm",
  "type": "workflow_selected",
  "input": "user said: add dark mode toggle",
  "state": {"thread_phase": "pm", "turns_used": 2},
  "decision": "implement",
  "alternatives": ["bugfix", "refactor"],
  "evidence": "user said 'add', no existing dark mode code found",
  "outcome": null
}
```

### Decision Types

| Type | Agent | What it captures |
|------|-------|-----------------|
| `workflow_selected` | PM | Why this workflow over others |
| `skill_matched` | PM | Why this skill, what variables were extracted |
| `agent_delegated` | PM | Why this agent was @mentioned with this task |
| `model_selected` | Any | Why this model was chosen (see Dynamic Model Routing) |
| `tool_chosen` | Any | Why this tool for this step (when alternatives exist) |
| `stuck_detected` | Any | What signal triggered stuck detection, which escape strategy |
| `escalated` | Any | Why the agent escalated (to Lead, to PM, to user) |
| `plan_deviated` | Coder | Why the implementation diverged from the PM's plan |
| `review_issue` | Reviewer | What was found, severity, evidence |
| `learning_proposed` | Lead | What learning, why, which agent, what evidence from thread |
| `compaction_triggered` | Any | Token count before/after, what was summarized |
| `circuit_breaker` | Any | State change (closed→open, open→half-open, etc.), model, failure count |

### Implementation

Each agent has a `DecisionLogger` that writes to the JSONL file. The logger is injected into the agent runner — same pattern as other dependencies. Writing is synchronous (append to file) and cheap.

```go
type Decision struct {
    Timestamp    time.Time      `json:"ts"`
    Agent        string         `json:"agent"`
    Type         string         `json:"type"`
    Input        string         `json:"input"`
    State        map[string]any `json:"state"`
    Decision     string         `json:"decision"`
    Alternatives []string       `json:"alternatives,omitempty"`
    Evidence     string         `json:"evidence"`
    Outcome      *string        `json:"outcome"` // filled later if known
}
```

**Not every tool call is a decision.** A decision is a point where the agent chose between alternatives. Reading a file is not a decision. Choosing which file to read first (when multiple candidates exist) is. The agent's seed instructs it when to log decisions — same as reasoning-in-thread, but structured and machine-readable.

### Package

```
internal/
  decisions/
    logger.go       # Append-only JSONL writer, thread-safe
    types.go        # Decision struct + decision type constants
```

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

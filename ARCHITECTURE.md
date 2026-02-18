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

Per-thread, per-agent conversation files:

```
.codebutler/conversations/
  <thread-id>/
    pm.json
    coder.json
    reviewer.json
    lead.json
    artist.json
    researcher.json
```

Each file stores the full message history (system prompt + user/assistant turns).

### Processing a message

When an agent processes a new message:
1. Load `.codebutler/conversations/<thread-id>/<role>.json` (create if first message)
2. Append the new user message to the conversation
3. Call the model with the full conversation
4. Append the assistant response
5. Save back to the file

### Daemon restart

Conversation files are the source of truth. If the daemon restarts, agents resume from where they left off — no context is lost.

### Thread lifecycle

When a thread ends (Lead completes retrospective), conversation files can be archived or deleted.

## Agent System Prompts

Each agent's system prompt is built from:
1. `seeds/<role>.md` — agent-specific identity, personality, tools, rules
2. `seeds/global.md` — shared project knowledge (architecture, conventions, decisions)
3. `seeds/workflows.md` — available workflows (PM only, or all agents if relevant)

The seed files are read once at startup. If the Lead updates them (after user approval), the changes take effect on the next model call (the updated file is loaded into the conversation).

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

Integration: mock OpenRouter. E2E: real Slack (manual).

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

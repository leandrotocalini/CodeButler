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

# CodeButler Journal

> Documenting architectural decisions, pivots, and learnings along the way.
> This will become the basis for a blog post.

---

## Chapter 1: The File-Based Protocol (v0)

**Problem:** Connect WhatsApp messages to Claude Code so a developer can send tasks from their phone.

**First approach:** JSON files as a message bus.

```
WhatsApp → Agent writes incoming.json → Claude reads it
Claude writes outgoing.json → Agent reads it → WhatsApp
```

Four files in `/tmp/codebutler/`:
- `incoming.json` — WhatsApp message for Claude
- `outgoing.json` — Claude's response for WhatsApp
- `question.json` — Claude asks user a question
- `answer.json` — User's answer

**How it worked:** The Go agent polled every 1 second for `outgoing.json` and `question.json`. Claude was expected to poll for `incoming.json`.

**The problem:** Claude had to be running AND actively watching for files. If Claude wasn't running, messages went nowhere. And polling every second on both sides was wasteful. The protocol was ad-hoc — we invented it, documented it in CLAUDE.md, and hoped Claude would follow it. Essentially, Claude had to be told "go check if there's a message" — it was passive.

---

## Chapter 2: MCP — Making It Native (v1)

**Insight:** Claude Code has native MCP (Model Context Protocol) support. Instead of inventing a file protocol, why not make WhatsApp a tool Claude can use?

**Approach:** The Go agent becomes an MCP Server exposing tools:

```
Claude Code ←→ MCP (stdio) ←→ Go Agent ←→ WhatsApp
```

Tools exposed:
- `send_message` — Send text to WhatsApp
- `ask_question` — Ask user with options, wait for answer
- `get_pending` — Get pending WhatsApp messages (with long-polling)
- `get_status` — Check connection status

**What improved:**
- No more file polling — direct function calls
- Claude discovers tools automatically via `.mcp.json`
- Typed errors instead of corrupt files
- Bidirectional communication in one protocol

**Configuration:** Just a `.mcp.json` in the project root:
```json
{
  "mcpServers": {
    "codebutler": {
      "command": "./codebutler",
      "args": ["--mcp"]
    }
  }
}
```

**The remaining problem:** Claude still had to be the initiator. It had to actively call `get_pending()` in a loop to receive messages. The CLAUDE.md had to instruct: "After completing each task, always call `get_pending()` to wait for the next WhatsApp message."

This felt like the file protocol with extra steps — Claude was still polling, just through a fancier pipe. The agent was essentially a passive viewer. It couldn't push work to Claude.

---

## Chapter 3: Agent Invokes Claude Directly (v2)

**Key realization:** The agent should be the orchestrator, not Claude. When a WhatsApp message arrives, the agent should INVOKE Claude, not wait for Claude to ask.

**Approach:** The Go agent spawns `claude -p "task"` as a subprocess.

```
WhatsApp message → Agent → exec("claude -p task") → output → WhatsApp
```

**What changed:**
- Agent is the driver — receives message, spawns Claude, sends result
- Claude gets full codebase access (reads, edits, runs commands)
- No polling on either side — event-driven from WhatsApp message to response
- Added `ClaudeConfig` for customization (command, workDir, maxTurns, timeout)
- Tasks serialized with a mutex — one Claude instance at a time

**What we kept:**
- MCP server still available as `--mcp` flag for reverse use case (Claude-initiated)
- Web UI for setup (QR scan, configuration)
- WhatsApp client unchanged (whatsmeow)

**What we dropped:**
- File-based protocol monitors (`monitorOutgoing`, `monitorQuestions`)
- Dependency on `protocol` package for the main flow
- The need for CLAUDE.md to explain polling behavior

**New problem:** Claude runs as a black box. User sends a message, waits potentially minutes, gets a wall of text back. No progress updates. No way to answer questions mid-task. No conversation continuity.

---

## Chapter 4: Streaming + Conversation Continuity (v3)

**Problem:** With `claude -p` (print mode), the interaction is one-shot:
1. User sends task
2. ... silence for 2 minutes ...
3. Giant response appears

The user has no idea what's happening. And if Claude needs clarification, it just makes its best guess instead of asking.

**Approach:** Use `--output-format stream-json` to get real-time events from Claude, and `--resume` to continue conversations.

```
Agent spawns: claude -p "task" --output-format stream-json
Agent reads stdout line by line:
  → content_block_start (tool_use) → "🔧 Reading file..." → WhatsApp
  → content_block_start (tool_use) → "✏️ Editing file..." → WhatsApp
  → result event → final summary → WhatsApp + save session_id
```

For conversation continuity:
```
User: "add JWT auth"
  → Claude processes, sends updates, returns result + session_id
  → Agent stores session_id in chatSessions map
User: "use jose instead of jsonwebtoken"
  → Agent detects existing session for this chat
  → Runs: claude --resume SESSION_ID -p "use jose instead"
  → Claude continues with full context from previous conversation
```

**Implementation details:**
- `streamEvent` structs parse each JSON line from stdout
- `content_block_start` with `tool_use` type triggers WhatsApp updates
- Updates throttled to every 30s to avoid spamming
- Tool names mapped to human-readable labels (`Read` → "Reading file...", `Bash` → "Running command...")
- `session_id` extracted from `result` event and stored per chat JID in a `sync.Map`
- Next message from same chat automatically uses `--resume`

**What this enables:**
- Real-time progress in WhatsApp (user sees what Claude is doing)
- Conversation continuity (follow-up messages resume context)
- Natural iteration: user sends correction → Claude continues where it left off
- No polling, no files, no MCP required — just streaming stdout + session tracking

---

## Architecture Evolution Summary

```
v0: Files    WhatsApp → JSON files ←→ Claude (both polling)
v1: MCP      WhatsApp → Agent ←MCP→ Claude (Claude polling via tools)
v2: CLI      WhatsApp → Agent → exec(claude) → WhatsApp (agent drives)
v3: Stream   WhatsApp → Agent → exec(claude --stream) ←→ WhatsApp (real-time)
```

Each version solved the previous version's key pain point:
- v0→v1: From ad-hoc files to a standard protocol
- v1→v2: From Claude-must-be-running-and-polling to agent-invokes-on-demand
- v2→v3: From black-box execution to observable, interactive sessions

---

## Key Learnings

1. **MCP is great for tool discovery, not for push notifications.** MCP makes tools discoverable and callable, but the client (Claude) has to initiate. It's a request-response protocol, not an event-driven one.

2. **The simplest integration with Claude Code is `claude -p`.** No SDKs, no complex protocols. Just spawn the CLI, pass the prompt, capture output. It has full access to everything Claude Code can do.

3. **`--output-format stream-json` unlocks observability.** Instead of waiting for the whole response, you can track tool uses and progress in real time. Combined with `--resume`, you get a conversational experience.

4. **Serialization matters.** Running multiple Claude instances concurrently would conflict on file edits. A simple mutex ensures tasks run one at a time, which matches the natural flow of a chat conversation.

5. **WhatsApp has a ~65K char limit** but practically, messages over 4K are unreadable on a phone. Truncation and summarization are important UX considerations.

---

## Tech Stack

- **Go** — Agent binary (WhatsApp client, web UI, task orchestration)
- **whatsmeow** — WhatsApp Web protocol library for Go
- **Claude Code CLI** — `claude -p` for task execution
- **MCP** — Optional, for Claude-initiated WhatsApp access
- **SQLite** — WhatsApp session persistence
- **OpenAI Whisper** — Voice message transcription (optional)

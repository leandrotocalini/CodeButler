# Global — Shared Project Knowledge

This file is read by ALL agents. It contains project-wide knowledge that every agent needs.

## Architecture

### Agent Daemon

The system runs **one daemon process** that manages **multiple agents**. Each agent is an independent loop with its own model conversation. The daemon does NOT run one big model call for all messages — each agent filters and processes only its own messages.

### Message Routing (code-level)

When a new Slack message arrives, the daemon checks it against each agent's filter **before** calling any model:

```
for each agent:
    if agent.shouldProcess(message):
        agent.process(message)
```

Filter rules (implemented in code, not in the model):
- **PM**: process if message contains `@codebutler.pm` OR message contains NO `@codebutler.*` mention
- **All other agents**: process if message contains `@codebutler.<their-role>` (exact match)
- A single message can match multiple agents (e.g., `@codebutler.coder @codebutler.reviewer` routes to both)

If a message doesn't match an agent's filter, that agent does nothing — no model call, no token cost.

### Conversation Persistence (code-level)

Each agent maintains its own conversation file in the worktree:

```
.codebutler/conversations/
  pm.json
  coder.json
  reviewer.json
  lead.json
  artist.json
  researcher.json
```

Each file stores the full message history (system prompt + user/assistant turns) for that agent's model conversation. When an agent processes a new message:
1. Load `<role>.json` (or create empty if first message)
2. Append the new user message
3. Call the model with the full conversation
4. Append the assistant response
5. Save back to `<role>.json`

This means agents have persistent memory across messages without re-prompting from scratch. The conversation file is the source of truth — if the daemon restarts, agents resume from where they left off.

### Thread Isolation

Each Slack thread gets its own set of conversation files. The directory structure is:

```
.codebutler/conversations/
  <thread-id>/
    pm.json
    coder.json
    ...
```

When a thread ends (Lead completes retrospective), the conversation files can be archived or deleted.

## Tech Stack

(Will be populated. Examples: "Go 1.22", "React 18", "shadcn/ui + Tailwind", "PostgreSQL 16")

## Conventions

(Will be populated. Examples: "snake_case for DB columns", "camelCase for Go variables", "all errors must be wrapped with fmt.Errorf")

## Key Decisions

(Will be populated. Examples: "We use JWT for auth, not sessions", "All API responses use the standard envelope format")

## Deployment

(Will be populated. Examples: "Deploy via GitHub Actions to AWS ECS", "Staging auto-deploys on push to main")

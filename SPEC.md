# Daemon Spec

How the daemon processes messages and manages agents. This is a code-level spec — not part of any agent's system prompt.

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

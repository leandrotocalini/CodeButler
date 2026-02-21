# CodeButler

Multi-agent AI dev team accessible from Slack. One Go binary, six agents,
each with its own personality, tools, and persistent memory. You describe
what you want in a Slack thread — the agents plan, build, review, and learn.

**Status**: Defining specs and agent seeds. Code not yet written.

## Design Documents

- `SPEC.md` — Product spec (what the system does, agents, flows, config, memory)
- `ARCHITECTURE.md` — Implementation details (message routing, event loop, project structure, testing)
- `seeds/` — Agent seed MDs (identity, personality, routing rules, tools, workflows)

## Agent Seeds

```
seeds/
  pm.md          # PM: entry point, planner, orchestrator
  coder.md       # Coder: writes code, runs tests, creates PRs
  reviewer.md    # Reviewer: reviews PRs (quality, security, tests)
  researcher.md  # Researcher: web research on demand
  artist.md      # Artist: UI/UX design + image generation
  lead.md        # Lead: mediator, retrospectives, team improvement
  global.md      # Shared project knowledge (all agents read this)
  workflows.md   # Available workflows (PM uses these to classify intent)
  skills/        # Seeded skills (copied to .codebutler/skills/ on init)
    explain.md   # Explain code (PM)
    test.md      # Write tests (Coder)
    changelog.md # Generate changelog (PM)
    hotfix.md    # Quick bug fix (PM → Coder)
    docs.md      # Generate docs (Coder)
    security-scan.md  # Security audit (Reviewer)
    self-document.md  # Document work in JOURNEY.md (Lead)
    status.md    # Project status report (PM)
```

## Key Concepts

- **Message routing**: each agent only processes messages with its `@codebutler.<role>` mention. PM also gets messages with no mention. Filtering happens in code before any model call.
- **Conversation persistence**: per-thread, per-agent JSON files in `.codebutler/conversations/<thread-id>/`.
- **Same binary, different roles**: `codebutler --role pm`, `codebutler --role coder`, etc.

## Development

All code and comments are in English. PRs only to `main` (direct push disabled).

## Documentation

- `JOURNEY.md` — Detailed log of architecture decisions, features, and reasoning.
  Use `/self-document` to add new entries after implementing features.

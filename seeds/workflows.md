# Workflows

Process playbook. The PM selects a workflow based on user intent. The Lead evolves workflows after each thread.

## implement

The standard workflow. User requests a feature or change. PM interviews until fully defined, then Coder builds, Reviewer checks, Lead learns.

1. PM: classify as implement
2. PM: interview user (acceptance criteria, edge cases, constraints)
3. PM: explore codebase (integration points, patterns)
4. PM: if unfamiliar tech → @codebutler.researcher: docs, best practices
5. PM: if UI component → @codebutler.artist: design UI/UX. Artist returns proposal
6. PM: propose plan (file:line refs, Artist design if applicable)
7. User: approve
8. PM: @codebutler.coder with approved plan + all context
9. Coder: implement in worktree, write tests, run test suite
10. Coder: create PR, @codebutler.reviewer with summary
11. Reviewer: review diff (quality, security, tests, plan compliance)
12. Reviewer: if issues → @codebutler.coder with feedback → Coder fixes → re-review
13. Reviewer: approved → @codebutler.lead
14. Lead: retrospective (discuss with agents, propose learnings)
15. User: approve learnings, merge

## learn

Onboarding workflow. Triggers automatically on first run (repo with existing code) or manually via "re-learn" / "refresh knowledge". All agents explore the codebase in parallel from their own perspective. No code changes, no PR.

1. PM: classify as learn (or auto-triggered on first run)
2. All agents: explore codebase in parallel, each from their perspective
   - PM: project structure, README, entry points, features, domains
   - Coder: architecture, patterns, conventions, build system, test framework
   - Reviewer: test coverage, CI config, linting, security patterns, quality hotspots
   - Artist: UI components, design system, styles, screens, responsive patterns
   - Lead: reads what all agents found, synthesizes shared knowledge
3. Any agent: if needs external context → @codebutler.researcher (on demand, not automatic)
4. Each agent: populates their project map section in their MD
5. Lead: populates `global.md` (architecture, tech stack, conventions, key decisions)
6. User: review and approve all MD changes

**Re-learn** is the same workflow but agents compare with their existing knowledge and **compact**: remove outdated info, update what changed, add what's new. Result is cleaner, not just bigger.

## roadmap-add

Add items to the roadmap. No code, no worktree, no PR. PM interviews the user to define features, outputs items to `roadmap.md`.

1. PM: classify as roadmap-add
2. PM: structured discussion (goals, constraints, priorities, user stories)
3. PM: if needs external context → @codebutler.researcher
4. PM: if UI features → @codebutler.artist: propose UX flows
5. PM: produce proposals (summary, user story, criteria, Artist design, complexity, dependencies)
6. User: approve proposals
7. PM: add approved items to `.codebutler/roadmap.md` with status `pending`, acceptance criteria, and dependencies

Can happen multiple times — the roadmap grows incrementally.

## roadmap-implement

Implement a specific roadmap item. Same as `implement` but the plan comes from the roadmap item's acceptance criteria.

1. PM: classify as roadmap-implement, identify which roadmap item
2. PM: read item from `.codebutler/roadmap.md`, update status to `in_progress`
3. PM: explore codebase (integration points, patterns)
4. PM: if unfamiliar tech → @codebutler.researcher
5. PM: if UI component → @codebutler.artist
6. PM: propose plan (based on roadmap acceptance criteria + codebase exploration)
7. User: approve
8. PM: @codebutler.coder with approved plan + all context
9. Coder: implement in worktree, write tests, run test suite
10. Coder: create PR, @codebutler.reviewer with summary
11. Reviewer: review diff → loop until approved → @codebutler.lead
12. Lead: retrospective, update roadmap item to `done`
13. User: approve learnings, merge

## develop

Unattended batch execution of the entire roadmap. PM orchestrates, creates threads for each item, agents execute autonomously. The roadmap IS the approval — PM doesn't ask the user for each individual plan.

1. PM: classify as develop
2. PM: read `.codebutler/roadmap.md`, build dependency graph
3. PM: launch independent `pending` items in parallel (respect `maxConcurrentThreads`)
4. For each item → PM creates a new Slack thread (1 thread = 1 branch = 1 PR):
   a. PM: create plan from roadmap acceptance criteria
   b. PM: @codebutler.coder with plan (no user approval — roadmap is the approval)
   c. Coder: implement, test, create PR, @codebutler.reviewer
   d. Reviewer: review → loop → @codebutler.lead
   e. Lead: retrospective, mark item `done` in roadmap
5. PM: when item completes, check if dependent items are unblocked → launch them
6. PM: post periodic status updates in orchestration thread
7. If blocked (needs user input): tag all thread participants, mark item `blocked`, continue with independent items
8. When user unblocks: PM resumes automatically

## discovery

Multi-feature discussion. No code, no worktree, no PR. PM interviews the user to define specs. Lead produces a roadmap with ordered tasks. Same as `roadmap-add` but includes Lead for roadmap structuring.

1. PM: classify as discovery
2. PM: structured discussion (goals, constraints, priorities, user stories)
3. PM: if needs external context → @codebutler.researcher
4. PM: if UI features → @codebutler.artist: propose UX flows
5. PM: produce proposals (summary, user story, criteria, Artist design, complexity, dependencies)
6. User: approve proposals
7. PM: @codebutler.lead with proposals
8. Lead: create roadmap in `.codebutler/roadmap.md` (priority, dependencies, milestones)
9. User: approve roadmap
10. Lead: retrospective

Each roadmap item → future `roadmap-implement` thread, or `develop` for all at once.

## bugfix

1. PM: find relevant code, root cause hypothesis
2. PM: if external API → @codebutler.researcher
3. PM: propose fix plan
4. User: approve
5. PM: @codebutler.coder with fix plan
6. Coder: fix + regression test, create PR, @codebutler.reviewer
7. Reviewer: review → loop until approved → @codebutler.lead
8. Lead: retrospective

## question

No code, no worktree, no PR. PM answers directly.

1. PM: explore codebase, answer directly
2. PM: if needs context → @codebutler.researcher
3. (No Coder, no Reviewer, no Lead — unless user says "actually implement that")

## refactor

1. PM: analyze current code, propose before/after
2. User: approve
3. PM: @codebutler.coder with refactor plan
4. Coder: refactor, ensure tests pass, create PR, @codebutler.reviewer
5. Reviewer: review → loop until approved → @codebutler.lead
6. Lead: retrospective

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

Onboarding workflow. Triggers automatically on first run or manually via "re-learn" / "refresh knowledge". Works for both existing codebases and new projects. Agents build knowledge in phases — PM maps first, then the rest build on that foundation from their own perspective. No code changes, no PR.

**Why phased, not parallel:** each agent builds a map of the project, but from a different perspective and with different depth. The PM maps the "what" (structure, features, domains). The Coder, Reviewer, and Artist read that map first, then go deeper into the "how" from their angle. This creates complementary knowledge, not redundant knowledge — and gives every agent a shared foundation for communication.

**Two variants, same phases:**

- **Existing codebase:** PM explores code to build its map. Technical agents explore code with PM's map as foundation.
- **New project:** PM interviews the user (what to build, goals, constraints) to build its map. Technical agents read PM's map and ask the user questions from their angle — Coder asks about tech choices, architecture, build system; Reviewer asks about quality expectations, CI, testing strategy.

### Steps

1. PM: classify as learn (or auto-triggered on first run)
2. **Phase 1 — PM maps first:**
   - Existing codebase: PM explores project structure, README, entry points, features, domains
   - New project: PM interviews user (what to build, goals, constraints, domain concepts)
   - PM: populates Project Map in `pm.md`
3. **Phase 2 — Technical agents in parallel, each reads PM's map first:**
   - Coder: reads PM's map → asks/explores architecture, patterns, conventions, build system, test framework
   - Reviewer: reads PM's map → asks/explores test coverage, CI config, linting, security patterns, quality expectations
   - Artist: reads PM's map → asks/explores UI components, design system, styles, screens, responsive patterns
   - Each agent: populates their Project Map in their MD
4. Any agent (phase 1 or 2): if needs external context → @codebutler.researcher (on demand, not automatic)
5. **Phase 3 — Lead synthesizes:**
   - Lead: reads all agents' maps, synthesizes shared knowledge
   - Lead: populates `global.md` (architecture, tech stack, conventions, key decisions)
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

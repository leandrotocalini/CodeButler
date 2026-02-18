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

## discovery

Multi-feature discussion. No code, no worktree, no PR. PM interviews the user to define specs. Lead produces a roadmap with ordered tasks.

1. PM: classify as discovery
2. PM: structured discussion (goals, constraints, priorities, user stories)
3. PM: if needs external context → @codebutler.researcher
4. PM: if UI features → @codebutler.artist: propose UX flows
5. PM: produce proposals (summary, user story, criteria, Artist design, complexity, dependencies)
6. User: approve proposals
7. PM: @codebutler.lead with proposals
8. Lead: create roadmap (priority, dependencies, milestones)
9. User: approve roadmap
10. Lead: create GitHub issues or commit roadmap
11. Lead: retrospective

Each roadmap item → future implement thread. Start: manually, "start next", or "start all".

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

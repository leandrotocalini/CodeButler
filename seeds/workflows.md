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
- **New project:** PM interviews the user (what to build, goals, constraints) to build its map. If unfamiliar tech or domain → PM asks Researcher before finishing Phase 1 so the map includes research context. Technical agents read PM's map (with research included) and ask the user questions from their angle — Coder asks about tech choices, architecture, build system; Reviewer asks about quality expectations, CI, testing strategy.

### Steps

1. PM: classify as learn (or auto-triggered on first run)
2. **Phase 1 — PM maps first:**
   - Existing codebase: PM explores project structure, README, entry points, features, domains
   - New project: PM interviews user (what to build, goals, constraints, domain concepts)
   - PM: if unfamiliar tech or domain → @codebutler.researcher before finishing phase 1
   - PM: populates Project Map in `pm.md` (includes research references if applicable)
3. **Phase 2 — Technical agents in parallel, each reads PM's map first:**
   - Coder: reads PM's map (+ research) → asks/explores architecture, patterns, conventions, build system, test framework
   - Reviewer: reads PM's map (+ research) → asks/explores test coverage, CI config, linting, security patterns, quality expectations
   - Artist: reads PM's map (+ research) → asks/explores UI components, design system, styles, screens, responsive patterns
   - Each agent: if needs more external context → @codebutler.researcher (on demand)
   - Each agent: populates their Project Map in their MD
4. **Phase 3 — Lead synthesizes:**
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

## brainstorm

Multi-model diverge-converge workflow. User describes a topic, problem, or feature set. PM dynamically creates specialized Thinker agents — each with a custom system prompt and a different LLM model — fans them out in parallel, collects responses, and synthesizes the best ideas. No code, no worktree, no PR — pure ideation. Output can feed into `discovery`, `roadmap-add`, or `implement`.

**Why multiple models?** Different models have different training data, reasoning styles, and blind spots. Asking the same question to Claude, Gemini, GPT, and DeepSeek produces complementary ideas — things one model suggests that others miss entirely. The PM's job is to merge the best of each.

**Dynamic Thinkers, not static agents.** The PM creates Thinker agents on the fly — they're not predefined roles like Coder or Reviewer. For each brainstorm, the PM:
1. Analyzes the topic and decides what **perspectives** would be most valuable
2. Crafts a unique **system prompt** for each Thinker (persona, focus area, thinking style)
3. Assigns a **different model** to each Thinker from the configured pool
4. Runs them all in parallel as single-shot LLM calls (no tools, no agent loop)

This is the key difference from static agents: the PM is the creative director. It decides "for this crypto trading bot question, I need a quantitative analyst, a risk manager, a market microstructure expert, and a systems architect" — and creates those personas dynamically. For a different topic, it would create entirely different Thinkers.

**One model per Thinker, no duplicates.** Each Thinker in a round MUST use a different LLM model. Running the same model twice with different prompts is redundant — the value comes from how different models reason differently about the same problem. The number of Thinkers per round is capped by the number of models in the `brainstorm.models` pool.

**Thinkers are lightweight.** They're goroutines inside the PM process, not separate OS processes. No Slack presence, no conversation persistence, no tool use. They receive a system prompt + user prompt, return a response, and die. The PM owns the entire lifecycle.

### Steps

1. PM: classify as brainstorm
2. PM: interview user — what's the topic? What constraints exist? What kind of ideas are you looking for? (architecture, features, approaches, optimizations, etc.)
3. PM: explore codebase if relevant (existing architecture, patterns, constraints)
4. PM: if needs external context → @codebutler.researcher (e.g., "what are current best practices for X?")
5. PM: **design the panel** — based on the topic, decide:
   - How many Thinkers (2–6, based on topic complexity)
   - What perspective each Thinker should have (PM writes a custom system prompt for each)
   - Which model each Thinker uses (assigned from `brainstorm.models` pool — **each Thinker MUST use a different model**, no duplicates in a single round)
6. PM: post the panel design in the thread ("Spinning up 4 Thinkers: Pragmatic Engineer on Gemini, Security Specialist on Claude, DeFi Protocol Expert on DeepSeek, Systems Architect on GPT")
7. PM: call `BrainstormFanOut` tool with the context block + per-Thinker system prompts + model assignments. Tool executes all calls in parallel via OpenRouter
8. PM: receive all Thinker responses. Post each in the thread (attributed to model + perspective) so the user can see raw outputs
9. PM: synthesize — identify:
   - **Common themes** (ideas that multiple Thinkers converge on → high confidence)
   - **Unique insights** (ideas only one Thinker proposed → worth exploring)
   - **Conflicts** (where Thinkers disagree → needs user judgment)
   - **Ranked recommendations** (PM's merged list, ordered by impact and feasibility)
10. PM: present synthesis to user with clear attribution ("The DeFi Expert (DeepSeek) suggested X, both the Architect (GPT) and Security Specialist (Claude) recommended Y, the Pragmatic Engineer (Gemini) uniquely proposed Z")
11. User: approve, refine, or ask for another round with different focus
12. PM: if user wants to act on ideas → transition to `discovery` (multiple features), `roadmap-add` (add to backlog), or `implement` (single feature)

### PM System Prompt Design

The PM's ability to design good Thinker prompts is what makes this workflow powerful. Guidelines for the PM:

- **Make perspectives genuinely different.** "Senior engineer" and "experienced developer" are the same thing. "Quantitative analyst focused on backtesting edge cases" and "market microstructure expert focused on order book dynamics" are different
- **Match perspectives to the topic.** A UX brainstorm needs a Cognitive Psychologist, an Accessibility Expert, a Visual Designer. A backend architecture brainstorm needs a Distributed Systems Engineer, a DBA, a DevOps specialist
- **Include at least one contrarian.** One Thinker should be told to challenge assumptions and find holes ("You are a skeptic. Your job is to find what could go wrong, what's being overlooked, and what assumptions are dangerous")
- **Include domain context in each prompt.** Don't just say "you're a security expert" — say "you're a security expert reviewing a crypto trading bot that connects to 3 exchanges via API keys and executes trades automatically. Focus on API key management, replay attacks, and fund safety"
- **Vary the model-perspective assignment.** Don't always put the hardest perspective on the best model. Mix it up — sometimes a cheaper model with the right framing produces the most creative insights

### Variants

**Focused brainstorm:** user provides a specific technical question ("what's the best approach for real-time sync between mobile and server?"). PM skips the interview, designs a focused panel, goes straight to fan-out.

**Iterative brainstorm:** after the first round, the user says "go deeper on idea #3". PM takes that idea + the original context, designs a new panel focused on that specific angle (possibly different perspectives than round 1), and runs another round. Each round narrows the focus.

**Codebase-aware brainstorm:** PM includes detailed codebase context (architecture, tech stack, existing patterns) in every Thinker's prompt so they propose ideas that are feasible within the current system. Without codebase context, ideas tend to be more greenfield.

**Team-augmented brainstorm:** PM also @mentions real CodeButler agents (Coder, Reviewer, Artist) asking for their perspective on the topic — from their role's angle. Combined with Thinker responses, this gives both multi-model diversity AND multi-role diversity. The Lead can later analyze which combination produced the best ideas.

### Lead Visibility & Learning

The brainstorm workflow produces valuable meta-data for the Lead to learn from. The PM MUST post enough in the Slack thread for the Lead to analyze the quality of the brainstorm process — not just the final synthesis.

**What the PM posts in the thread (visible to Lead):**

1. **Panel design** — which Thinkers were created, what perspective each has, and which model each uses. This lets the Lead assess whether the PM chose good perspectives for the topic
2. **Per-Thinker system prompts** — posted as a collapsed/summarized block. The Lead needs to see HOW the PM framed each Thinker to evaluate prompt quality. Were prompts genuinely different? Were they specific enough?
3. **Per-Thinker responses** — key excerpts from each Thinker's output, attributed to model + perspective. The Lead needs to see which model produced the most useful ideas and whether the perspective framing helped or hurt
4. **Synthesis reasoning** — the PM's analysis of why it ranked ideas the way it did, what common themes it found, and what it discarded

**What goes in the thread report (`.codebutler/reports/`):**

```json
{
  "brainstorm": {
    "rounds": 1,
    "thinkers": [
      {
        "name": "Quantitative Analyst",
        "model": "deepseek/deepseek-r1",
        "prompt_length_tokens": 450,
        "response_length_tokens": 1200,
        "unique_ideas": 3,
        "ideas_selected_in_synthesis": 2
      }
    ],
    "total_ideas_generated": 14,
    "ideas_selected": 6,
    "ideas_common_across_models": 4,
    "cost_usd": 0.45,
    "user_satisfaction": "approved_all"
  }
}
```

**What the Lead learns over time:**

- Which model combinations produce the best results for different topic types
- Whether the PM's perspective selection is effective (do specialized prompts beat generic ones?)
- Whether certain models consistently produce more unique/selected ideas
- Whether brainstorm rounds lead to better plans than single-model planning
- Whether the PM's synthesis captures the best ideas or misses things the user catches

The Lead can propose improvements: "PM's brainstorm panels for backend architecture consistently miss a database perspective — add to PM seed as a recommendation" or "DeepSeek-R1 produced the most unique ideas in 4 of 5 brainstorms — consider it the default first pick for reasoning-heavy topics."

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

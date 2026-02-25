Implement all pending milestones from the CodeButler roadmap, one by one,
respecting the dependency graph.

1. Read `ROADMAP.md` — build a full picture of all milestones and their statuses
2. Read the "Dependency Graph" section and build the dependency order
3. Count how many milestones are `pending` or `in_progress` — announce the plan:
   - List the milestones to implement in dependency order
   - Note which ones can be parallelized (within the same phase)
4. **Loop** — repeat until no `pending` milestones with satisfied dependencies remain:
   a. Identify the next milestone with status `pending` whose dependencies are all `done`
      - If a milestone depends on another that isn't `done`, skip it
      - If no milestone is unblocked, stop and report what's blocked and why
   b. Announce which milestone you're starting
   c. Read the milestone's tasks and acceptance criteria carefully
   d. Read `SPEC.md` and `ARCHITECTURE.md` for the detailed specs relevant to this milestone
   e. Update `ROADMAP.md`: change the milestone status from `pending` to `in_progress`
   f. Implement everything listed in the milestone:
      - Follow the project structure from `ARCHITECTURE.md`
      - Follow Go guidelines: pure functions, dependency injection via interfaces, structured logging with `log/slog`, goroutines + channels, `errgroup` for fan-out
      - Write unit tests for every package (table-driven, one mock per interface, `testdata/` for fixtures)
      - No external test frameworks beyond stdlib (`testing` package + `testify` if already in go.mod)
   g. Run `go test ./...` — fix until all pass
   h. Run `go vet ./...` — fix any issues
   i. When all tasks are complete and tests pass:
      - Update `ROADMAP.md`: change status to `done`, check all `[x]` items
      - Commit with message: `M<number>: <milestone title>`
   j. Use `/self-document` to add a JOURNEY.md entry about what was built
   k. **Continue to the next milestone** — go back to step 4a
5. When all milestones are done (or all remaining are blocked), print a final summary:
   - Total milestones completed in this session
   - Any milestones still blocked (with reasons)
   - Current state of the roadmap

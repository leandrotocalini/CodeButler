Implement the next pending milestone from the CodeButler roadmap.

1. Read `ROADMAP.md` — find all milestones and their statuses
2. Identify the first milestone with status `pending` whose dependencies are all `done`
   - Dependencies follow the graph in the "Dependency Graph" section
   - If a milestone depends on another that isn't `done`, skip it and check the next one
3. Announce which milestone you're implementing and why it's next
4. Read the milestone's tasks and acceptance criteria carefully
5. Read `SPEC.md` and `ARCHITECTURE.md` for the detailed specs relevant to this milestone
6. Update `ROADMAP.md`: change the milestone status from `pending` to `in_progress`
7. Implement everything listed in the milestone:
   - Follow the project structure from `ARCHITECTURE.md`
   - Follow Go guidelines: pure functions, dependency injection via interfaces, structured logging with `log/slog`, goroutines + channels, `errgroup` for fan-out
   - Write unit tests for every package (table-driven, one mock per interface, `testdata/` for fixtures)
   - No external test frameworks beyond stdlib (`testing` package + `testify` if already in go.mod)
8. Run `go test ./...` — fix until all pass
9. Run `go vet ./...` — fix any issues
10. When all tasks are complete and tests pass:
    - Update `ROADMAP.md`: change status to `done`, check all `[x]` items
    - Commit with message: `M<number>: <milestone title>`
11. Use `/self-document` to add a JOURNEY.md entry about what was built

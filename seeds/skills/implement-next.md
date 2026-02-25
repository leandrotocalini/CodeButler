# implement-next

Pick up the next pending milestone from the roadmap and implement it.

## Trigger
implement next, next milestone, continue roadmap, implement-next

## Agent
coder

## Prompt
Implement the next pending milestone from the CodeButler roadmap.

1. Read `ROADMAP.md` and find the first milestone with status `pending`
2. Check its dependencies — verify all prerequisite milestones are `done`. If not, pick the first `pending` milestone whose dependencies are all satisfied
3. Read the milestone's description, tasks, and acceptance criteria carefully
4. Read `SPEC.md` and `ARCHITECTURE.md` for detailed specs relevant to this milestone
5. Update `ROADMAP.md`: change the milestone status from `pending` to `in_progress`
6. Implement everything listed in the milestone:
   - Follow the project structure defined in `ARCHITECTURE.md`
   - Follow Go guidelines from `ARCHITECTURE.md` (pure functions, dependency injection, interfaces, structured logging, goroutines)
   - Write unit tests for every package (table-driven tests, one mock per interface, `testdata/` for fixtures)
   - Keep code in `internal/` packages as specified
7. Run tests: `go test ./...` — fix until all pass
8. Run vet: `go vet ./...` — fix any issues
9. Once all tasks are complete and tests pass, update `ROADMAP.md`:
   - Change milestone status from `in_progress` to `done`
   - Check all checkbox items `[x]`
10. Commit all changes with a descriptive message referencing the milestone number
11. Create a PR with the milestone summary

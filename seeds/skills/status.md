# status

Show the current state of the project.

## Trigger
status, project status, what's going on, where are we

## Agent
pm

## Prompt
Report the current project status.

1. Read `.codebutler/roadmap.md` if it exists — summarize progress (done/in_progress/pending/blocked items)
2. Check for active branches: `git branch -r --list 'origin/codebutler/*'` — list any in-progress work
3. Check for open PRs: `gh pr list --state open` — list with title, branch, and status
4. Read recent git history: `git log --oneline -10` — summarize recent activity
5. Post a structured status report in the thread:
   - **Roadmap** — progress summary (X of Y items done) with list
   - **Active work** — branches and PRs in progress
   - **Recent activity** — what happened in the last few commits
   - **Blocked** — anything that needs attention

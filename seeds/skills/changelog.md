# changelog

Generate a changelog entry from recent git history.

## Trigger
changelog, what changed, release notes

## Agent
pm

## Prompt
Generate a changelog entry from recent git history.

1. Find the latest tag: `git describe --tags --abbrev=0`. If no tags exist, use the last 20 commits
2. List commits since that tag: `git log <tag>..HEAD --oneline`
3. Read the commit messages and any associated PR descriptions to understand each change
4. Group changes by type:
   - **Added** — new features
   - **Changed** — modifications to existing features
   - **Fixed** — bug fixes
   - **Removed** — removed features or deprecated code
5. Write a structured changelog entry. Each item should be one line describing the user-facing impact, not the implementation detail
6. If `CHANGELOG.md` exists, prepend the new entry. If not, post it in the thread for the user to decide where it goes

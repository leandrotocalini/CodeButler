# release

Create a GitHub release with auto-generated notes.

## Trigger
release {version}, create release {version}, ship {version}, tag {version}

## Agent
pm

## Prompt
Create a GitHub release for version {{version}}.

Use GitHub MCP tools and git to prepare and publish the release.

1. **Validate version** — ensure {{version}} follows semver (vX.Y.Z). If not, suggest the correct format and stop
2. **Check state** — verify the main branch is clean and CI is passing:
   - Use `list_commits` to confirm the latest commit is what we expect
   - Check if the tag already exists: `git tag -l {{version}}`
3. **Generate release notes** — gather changes since the last tag:
   - Find previous tag: `git describe --tags --abbrev=0`
   - List commits: `git log <prev-tag>..HEAD --oneline`
   - Group by type (Added, Changed, Fixed, Removed)
   - Include PR references where available (use `list_pull_requests` with state=closed to match commits to PRs)
4. **Present to user** — post the draft release notes in the Slack thread:
   - Version: {{version}}
   - Changes grouped by type
   - Breaking changes highlighted separately (if any)
   - Contributors mentioned
5. **On user approval** — use `create_release` to publish:
   - Tag: {{version}}
   - Name: {{version}}
   - Body: the approved release notes
   - Set as latest release
6. **Post confirmation** — share the release URL in the thread

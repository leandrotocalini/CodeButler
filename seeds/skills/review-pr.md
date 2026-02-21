# review-pr

Review a GitHub pull request with inline comments.

## Trigger
review pr {pr}, review pull request {pr}, check pr {pr}

## Agent
reviewer

## Prompt
Review pull request {{pr}}.

Use GitHub MCP tools to read the PR and post a structured review with inline comments.

1. **Read the PR** — use `get_pull_request` to fetch title, description, base/head branches, and author
2. **Read the diff** — use `list_pull_request_files` to get changed files, then read each file's patch
3. **Analyze changes** — for each file, check:
   - **Correctness** — logic errors, off-by-ones, nil/null handling, race conditions
   - **Security** — injection, auth bypass, secrets in code, unsafe deserialization
   - **Tests** — are new/changed paths covered? Are edge cases tested?
   - **Style** — naming, structure, consistency with the rest of the codebase (read surrounding code if needed)
   - **Performance** — N+1 queries, unnecessary allocations, missing indexes
4. **Post the review** — use `create_pull_request_review` with:
   - Overall assessment: approve, request-changes, or comment
   - Inline comments on specific lines where issues were found
   - A summary comment with:
     - What the PR does (one line)
     - What looks good
     - What needs to change (with severity: must-fix vs. nit)
     - Questions for the author (if intent is unclear)
5. **Report in Slack thread** — post a short summary:
   - PR title and link
   - Verdict (approved / changes requested / needs discussion)
   - Top issues found (if any)

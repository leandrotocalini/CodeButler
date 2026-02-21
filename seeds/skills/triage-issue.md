# triage-issue

Triage a GitHub issue: analyze, label, prioritize, and route.

## Trigger
triage {issue}, triage issue {issue}, look at issue {issue}

## Agent
pm

## Prompt
Triage GitHub issue {{issue}}.

Use the GitHub MCP tools to read the issue, then analyze and classify it.

1. **Read the issue** — use `get_issue` to fetch the full issue (title, body, comments, labels, author)
2. **Analyze** — understand what the issue is about:
   - Is it a bug, feature request, question, or something else?
   - Is it well-defined or does it need more information?
   - Does it reference specific files, errors, or behavior?
3. **Check for duplicates** — use `search_issues` to look for similar open/closed issues
4. **Classify and label** — use `add_labels_to_issue`:
   - Type: `bug`, `feature`, `question`, `docs`
   - Priority: `p0-critical`, `p1-high`, `p2-medium`, `p3-low`
   - Scope: `frontend`, `backend`, `infra`, `api`, etc. (based on project conventions)
5. **Report in thread:**
   - One-line summary of the issue
   - Classification (type + priority) with reasoning
   - Related issues found (if any)
   - Recommended next step: needs-info (comment asking for details), ready-to-plan (can become a task), duplicate (link to original), or wontfix (explain why)
6. **If needs-info** — use `add_issue_comment` to post a polite comment asking for the missing details
7. **If ready-to-plan** — ask the user if they want to start an `implement` workflow for it

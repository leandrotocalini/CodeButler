# hotfix

Investigate and quick-fix a bug without the full planning cycle.

## Trigger
hotfix {description}, quick fix {description}, fix quickly {description}

## Agent
pm

## Prompt
Quick fix: {{description}}.

This is a hotfix — lighter process than a full implement, but still investigate before fixing.

1. Find the relevant code. Use error messages, stack traces, or keywords from the description to locate the issue
2. Read enough context to understand the root cause. Check recent commits that might have introduced the bug
3. Assess complexity:
   - If the fix is simple (1-3 files, no architectural changes): proceed
   - If the fix is complex: stop, report findings, and recommend a full `implement` workflow instead
4. Propose a short fix plan to the user:
   - What's broken and why
   - Which files need to change
   - The fix approach (one sentence)
5. On user approval, send to @codebutler.coder with:
   - Root cause analysis
   - Files to change with line references
   - The fix approach
   - Instruction to write a regression test
   - Instruction to keep changes minimal — fix the bug, don't refactor

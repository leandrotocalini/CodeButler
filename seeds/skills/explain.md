# explain

Explain how a part of the codebase works.

## Trigger
explain {target}, how does {target} work, what does {target} do

## Agent
pm

## Prompt
Explain how {{target}} works in this codebase.

1. Find the relevant files — use Glob and Grep to locate code related to {{target}}
2. Read the key files. Focus on the public API, main logic flow, and how it connects to the rest of the system
3. Post a structured explanation in the thread:
   - **What it does** — one paragraph summary
   - **Key files** — list with file:line references
   - **How it works** — step-by-step flow (entry point → logic → output)
   - **Dependencies** — what it imports/uses, what uses it
   - **Gotchas** — non-obvious behavior, edge cases, known issues
4. Keep it concise. This is for a developer who needs to understand the code, not a tutorial

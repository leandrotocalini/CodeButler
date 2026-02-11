Document recent work in JOURNEY.md. Follow these steps:

1. Read JOURNEY.md to see existing entries and writing style
2. Read recent commits with `git log --oneline -20` and diff the current branch vs main
3. Read the modified files to understand the changes in detail
4. Add a new entry at the end of JOURNEY.md

## Writing style

Write like a technical blog post — someone telling a story about building something.
The audience is a developer who's curious about the project but hasn't seen the code.

**Lead with the problem.** Start each section with the situation or friction that
motivated the change. What was annoying? What broke? What was missing? Make the
reader feel the problem before showing the solution.

**Focus on the "why".** The most interesting part is never *what* you built — it's
*why* you built it that way and not another way. Explain the reasoning, the trade-offs
you considered, and the alternatives you discarded (and why you discarded them).

**Keep implementation details light.** Don't list every file and function changed.
Mention specific code only when it illustrates a design decision or a clever trick.
A reader should understand the architecture without reading the source.

**Use concrete examples.** Instead of abstract descriptions, show what actually
happens: "When the user sends a message and walks away for 20 minutes..." is better
than "The system handles idle timeouts."

## Format

- `## YYYY-MM-DD — Descriptive title` (the title should hint at the insight, not just the feature)
- Subsections with `###` if multiple topics
- Code snippets only when they clarify a design choice, not to document the API
- English, conversational but technical tone

Do NOT delete previous entries. Only append at the end.

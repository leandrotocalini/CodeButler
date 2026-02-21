# docs

Generate or update documentation for a part of the codebase.

## Trigger
document {target}, docs for {target}, write docs for {target}

## Agent
coder

## Prompt
Generate or update documentation for {{target}}.

1. Read the source code for {{target}}. Understand the public API, configuration, and usage patterns
2. Check if documentation already exists (README, doc comments, wiki). If so, read it to understand what's outdated or missing
3. Write documentation that covers:
   - **Overview** — what it does, when to use it
   - **Usage** — code examples showing the most common operations
   - **Configuration** — available options with defaults
   - **API reference** — public functions/methods with parameters and return values (only if not already covered by doc comments)
4. Place the docs where they belong:
   - Package-level: doc comments in the source code
   - Module/feature-level: README.md in the relevant directory
   - Project-level: update the main README.md
5. Create a PR with the documentation changes

# Sources

Your code repositories go here. CodeButler can work across multiple repos.

## Adding a Repository

```bash
cd Sources/
git clone https://github.com/yourorg/your-repo
```

## CLAUDE.md

Each repository should have a `CLAUDE.md` file that tells Claude about the project:

```markdown
# Project Name

## What it does
Brief description.

## Tech Stack
- Language: Go/TypeScript/Python
- Framework: Express/Gin/Django
- Database: PostgreSQL/MongoDB

## Key Commands
- Build: `make build`
- Test: `make test`
- Run: `make dev`
```

This helps Claude understand your codebase faster.

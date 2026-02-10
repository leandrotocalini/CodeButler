# Repo Package

Discovers repositories in the Sources directory.

## Usage

```go
repos, err := repo.ScanRepositories("./Sources")
r, err := repo.GetRepository("./Sources", "my-project")
```

Finds directories with `.git` folder. Detects presence of `CLAUDE.md`.

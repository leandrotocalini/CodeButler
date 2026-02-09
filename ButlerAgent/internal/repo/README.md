# Repository Package

This package handles repository management for CodeButler, scanning and detecting projects in the Sources directory.

## Philosophy

**Language-agnostic**: CodeButler doesn't care about project type (Go, Node, Python, etc.). Claude Code reads the `CLAUDE.md` file to understand how to work with any project.

## Files

### scanner.go
- **ScanRepositories()**: Scans Sources directory for git repositories
- **GetRepository()**: Gets a specific repository by name
- **isGitRepo()**: Checks if a directory is a git repository
- **fileExists()**: Checks if a file exists

### scanner_test.go
- Comprehensive tests for all scanning functions

## Features

- Scans `Sources/` directory for repositories
- Only includes directories with `.git` folder
- **Detects CLAUDE.md** presence (required for Claude Code integration)
- Simple and fast - no language detection needed

## CLAUDE.md Detection

**CRITICAL**: CodeButler only works with repositories that have a `CLAUDE.md` file. This file contains instructions for Claude Code and makes the project AI-ready.

```go
type Repository struct {
    Name         string
    Path         string
    HasClaudeMd  bool      // ✅ if CLAUDE.md exists
    ClaudeMdPath string    // Full path to CLAUDE.md
}
```

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/repo"
)

func main() {
    // Scan all repositories
    repos, err := repo.ScanRepositories("./Sources")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found %d repositories:\n", len(repos))
    for _, r := range repos {
        claudeStatus := "❌"
        if r.HasClaudeMd {
            claudeStatus = "✅"
        }
        fmt.Printf("  - %s %s at %s\n", r.Name, claudeStatus, r.Path)
    }

    // Filter only Claude-ready repos
    claudeRepos := []repo.Repository{}
    for _, r := range repos {
        if r.HasClaudeMd {
            claudeRepos = append(claudeRepos, r)
        }
    }
    fmt.Printf("\nClaude-ready: %d/%d\n", len(claudeRepos), len(repos))

    // Get specific repository
    myRepo, err := repo.GetRepository("./Sources", "my-project")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Repository: %s\n", myRepo.Name)
    if myRepo.HasClaudeMd {
        fmt.Printf("CLAUDE.md found at: %s\n", myRepo.ClaudeMdPath)
    }
}
```

## Repository Struct

```go
type Repository struct {
    Name string      // Directory name
    Path string      // Full path to repository
    Type RepoType    // Detected project type
}
```

## Detection Logic

1. **Scan directory**: Read all entries in Sources/
2. **Filter directories**: Skip files, only check directories
3. **Check git**: Must have `.git/` subdirectory
4. **Detect type**: Check for marker files in priority order
5. **Return list**: All valid repositories found

## Error Handling

Functions return errors for:
- Sources path doesn't exist
- Permission denied reading directory
- Repository not found (GetRepository)

## Testing

Run tests:
```bash
go test ./internal/repo/... -v
```

Tests cover:
- ✅ Scanning multiple repositories
- ✅ **CLAUDE.md detection**
- ✅ Git repository validation
- ✅ Getting specific repository
- ✅ Non-existent paths
- ✅ Non-git directories (ignored)

## Why Language-Agnostic?

1. **Claude Code handles it**: The Claude Code CLI reads `CLAUDE.md` to understand any project
2. **Simpler**: No need to maintain language detection logic
3. **Flexible**: Works with any language, even mixed-language projects
4. **Future-proof**: New languages work automatically

The only thing that matters: **Does the repo have CLAUDE.md?** ✅/❌

## Future Enhancements

Possible improvements:
- Git branch detection
- Git remote URL extraction
- Last commit info
- Repository size
- Language version detection (Go 1.21, Node 18, etc.)
- Dependency analysis
- Build status

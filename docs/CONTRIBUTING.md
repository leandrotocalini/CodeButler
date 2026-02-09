# Contributing to CodeButler

Thank you for your interest in contributing to CodeButler! This document provides guidelines for contributing to the project.

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Node.js (for Claude Code SDK)
- Git
- WhatsApp account (for testing)

### Setup Steps

```bash
# Clone the repository
git clone git@github.com:leandrotocalini/CodeButler.git
cd CodeButler

# Install dependencies
go mod download

# Build
go build -o butler main.go

# Run tests
go test ./internal/...
```

## Project Structure

```
CodeButler/
├── main.go                    # Entry point and first-time setup
├── internal/
│   ├── whatsapp/              # WhatsApp client and handlers
│   ├── access/                # Access control logic
│   ├── audio/                 # Whisper API integration
│   ├── repo/                  # Repository management
│   ├── claude/                # Claude Code SDK executor
│   └── config/                # Configuration system
└── docs/                      # Documentation
```

## Code Style

### Go Code

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` to format code
- Use `golint` to check for issues
- Write tests for new functionality
- Keep functions small and focused

### Comments

Use comments sparingly. Only comment complex code. Code should be self-documenting through:
- Clear variable names
- Descriptive function names
- Logical structure

```go
// Good: No comment needed
func getUserByID(id string) (*User, error) {
    return db.FindUser(id)
}

// Good: Complex logic explained
func calculateSessionExpiry(lastActive time.Time) time.Time {
    // Sessions expire after 30 minutes of inactivity
    // or 24 hours absolute, whichever comes first
    thirtyMinLater := lastActive.Add(30 * time.Minute)
    twentyFourHoursLater := time.Now().Add(24 * time.Hour)

    if thirtyMinLater.Before(twentyFourHoursLater) {
        return thirtyMinLater
    }
    return twentyFourHoursLater
}
```

### Error Handling

- Always handle errors explicitly
- Wrap errors with context using `fmt.Errorf`
- Don't panic in library code

```go
// Good
func loadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }
    // ...
}

// Bad
func loadConfig(path string) *Config {
    data, _ := os.ReadFile(path) // Ignoring error
    // ...
}
```

## Testing

### Unit Tests

- Write unit tests for all public functions
- Use table-driven tests for multiple cases
- Mock external dependencies

```go
func TestIsAllowed(t *testing.T) {
    tests := []struct {
        name     string
        sender   string
        config   *Config
        expected bool
    }{
        {
            name:     "personal number allowed",
            sender:   "1234567890@s.whatsapp.net",
            config:   &Config{PersonalNumber: "1234567890@s.whatsapp.net"},
            expected: true,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := IsAllowed(tt.sender, tt.config)
            if result != tt.expected {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

### Integration Tests

- Tag integration tests with `//go:build integration`
- Use test fixtures when needed
- Clean up resources after tests

## Pull Request Process

1. **Create a branch** from `main`
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**
   - Write code
   - Add tests
   - Update documentation

3. **Run tests**
   ```bash
   go test ./...
   ```

4. **Format code**
   ```bash
   go fmt ./...
   ```

5. **Commit with clear message**
   ```bash
   git commit -m "Add feature: brief description

   - Detailed change 1
   - Detailed change 2

   Closes #123"
   ```

6. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

7. **Fill out PR template**
   - Describe what the PR does
   - Link related issues
   - Add screenshots if UI changes
   - Request review

## Commit Messages

Follow conventional commits format:

```
<type>: <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, etc)
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance tasks

Examples:
```
feat: add audio transcription support

- Integrate OpenAI Whisper API
- Download voice messages from WhatsApp
- Transcribe to text before processing

Closes #42
```

```
fix: handle WhatsApp session timeout

WhatsApp sessions expire after 20 seconds if QR not scanned.
Added timeout handling and retry logic.

Fixes #56
```

## Areas for Contribution

### High Priority

- [ ] WhatsApp integration (Phase 1)
- [ ] Configuration system (Phase 2)
- [ ] Repository management (Phase 5)
- [ ] Claude Code executor (Phase 6)

### Medium Priority

- [ ] Audio transcription (Phase 4)
- [ ] Dev control commands (Phase 8)
- [ ] Bulk operations (Phase 8)
- [ ] System monitoring (Phase 8)

### Nice to Have

- [ ] Web dashboard
- [ ] Multiple language support
- [ ] Custom workflows
- [ ] Plugin system

See [IMPLEMENTATION_ROADMAP.md](IMPLEMENTATION_ROADMAP.md) for detailed tasks.

## Getting Help

- **Questions**: Open a [Discussion](https://github.com/leandrotocalini/CodeButler/discussions)
- **Bugs**: Open an [Issue](https://github.com/leandrotocalini/CodeButler/issues)
- **Feature Requests**: Open an [Issue](https://github.com/leandrotocalini/CodeButler/issues) with `enhancement` label

## Code of Conduct

### Our Standards

- Be respectful and inclusive
- Welcome newcomers
- Focus on constructive feedback
- Accept criticism gracefully

### Unacceptable Behavior

- Harassment or discrimination
- Trolling or insulting comments
- Personal attacks
- Publishing private information

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

**Thank you for contributing to CodeButler!** 🎉

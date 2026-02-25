package tools

import (
	"strings"
)

// safeCommands are commands that are always WRITE_LOCAL (test runners, linters, build tools).
var safeCommands = []string{
	"go test", "go vet", "go build", "go fmt", "go mod",
	"npm test", "npm run test", "npm run lint", "npm run build", "npm ci",
	"npx eslint", "npx prettier", "npx jest", "npx tsc",
	"yarn test", "yarn lint", "yarn build",
	"pnpm test", "pnpm lint", "pnpm build",
	"make test", "make build", "make lint", "make check",
	"pytest", "python -m pytest", "mypy", "ruff", "black --check", "flake8",
	"eslint", "prettier", "tsc",
	"cargo test", "cargo build", "cargo clippy", "cargo fmt",
	"bundle exec rspec", "bundle exec rubocop",
	"grep", "find", "ls", "cat", "head", "tail", "wc", "sort", "uniq",
	"echo", "pwd", "which", "env", "date", "whoami",
	"diff", "sha256sum", "md5sum", "file", "stat",
}

// dangerousPatterns are substrings that indicate DESTRUCTIVE commands.
var dangerousPatterns = []string{
	"rm -rf", "rm -r", "rmdir",
	"DROP ", "DELETE FROM", "TRUNCATE ",
	"deploy", "npm publish", "yarn publish",
	"docker", "kubectl",
	"sudo ", "chmod ", "chown ",
	"| sh", "| bash", "|sh", "|bash",
	"pip install", "npm install -g", "gem install",
	"apt-get", "apt ", "yum ", "brew install",
	"systemctl", "service ",
	"shutdown", "reboot",
	"mkfs", "fdisk", "dd ",
	"> /dev/", ">> /dev/",
	"eval ", "`", "$(",
}

// ClassifyBashCommand determines the risk tier of a bash command string.
// Returns Destructive for dangerous patterns, WriteLocal for safe/unknown commands.
func ClassifyBashCommand(command string) RiskTier {
	cmd := strings.TrimSpace(command)
	lower := strings.ToLower(cmd)

	// Check dangerous patterns first (higher priority)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return Destructive
		}
	}

	// Check safe commands
	for _, safe := range safeCommands {
		if strings.HasPrefix(lower, strings.ToLower(safe)) {
			return WriteLocal
		}
	}

	// Unknown commands default to WRITE_LOCAL (conservative but not blocking)
	return WriteLocal
}

// ClassifyToolRisk determines the risk tier for a named tool with optional arguments.
// For Bash tools, it analyzes the command string. For others, returns the tool's default tier.
func ClassifyToolRisk(toolName string, args map[string]interface{}) RiskTier {
	switch toolName {
	case "Read", "Grep", "Glob":
		return Read
	case "Write", "Edit":
		return WriteLocal
	case "GitCommit", "GitPush", "GHCreatePR", "SendMessage":
		return WriteVisible
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			return ClassifyBashCommand(cmd)
		}
		return WriteLocal
	default:
		return WriteLocal
	}
}

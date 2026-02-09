package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/claude"
	"github.com/leandrotocalini/CodeButler/internal/commands"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/repo"
	"github.com/leandrotocalini/CodeButler/internal/session"
)

type Bot struct {
	cfg          *config.Config
	sessionMgr   *session.Manager
	executor     *claude.Executor
	repos        []repo.Repository
	sendMessage  func(chatID, text string) error
}

func NewBot(cfg *config.Config, sendMessage func(chatID, text string) error) *Bot {
	return &Bot{
		cfg:         cfg,
		sessionMgr:  session.NewManager(),
		executor:    claude.NewExecutor(),
		sendMessage: sendMessage,
	}
}

func (b *Bot) LoadRepositories() error {
	repos, err := repo.ScanRepositories(b.cfg.Sources.RootPath)
	if err != nil {
		return fmt.Errorf("failed to scan repositories: %w", err)
	}
	b.repos = repos
	return nil
}

func (b *Bot) HandleCommand(chatID, messageText string) string {
	cmd := commands.Parse(messageText)
	if cmd == nil {
		return ""
	}

	if err := commands.ValidateCommand(cmd); err != nil {
		return fmt.Sprintf("❌ %s", err.Error())
	}

	switch cmd.Type {
	case commands.CommandHelp:
		return commands.GetHelpText()

	case commands.CommandRepos:
		return b.handleRepos()

	case commands.CommandUse:
		return b.handleUse(chatID, cmd.GetArg(0))

	case commands.CommandStatus:
		return b.handleStatus(chatID)

	case commands.CommandRun:
		return b.handleRun(chatID, cmd.GetArgsString())

	case commands.CommandClear:
		return b.handleClear(chatID)

	default:
		return "❌ Unknown command. Type '@codebutler help' for available commands."
	}
}

func (b *Bot) handleRepos() string {
	if len(b.repos) == 0 {
		return "📂 No repositories found in Sources/\n\n💡 Clone some repositories to get started."
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("📂 Found %d repositor(y/ies):\n\n", len(b.repos)))

	claudeReady := 0
	for i, r := range b.repos {
		claudeStatus := "❌"
		if r.HasClaudeMd {
			claudeStatus = "✅"
			claudeReady++
		}
		result.WriteString(fmt.Sprintf("%d. *%s* %s\n", i+1, r.Name, claudeStatus))
	}

	result.WriteString(fmt.Sprintf("\n✅ Claude-ready: %d/%d", claudeReady, len(b.repos)))
	result.WriteString("\n\n💡 Use: @codebutler use <repo-name>")

	return result.String()
}

func (b *Bot) handleUse(chatID, repoName string) string {
	for _, r := range b.repos {
		if r.Name == repoName {
			if !r.HasClaudeMd {
				return fmt.Sprintf("❌ Repository '%s' doesn't have CLAUDE.md\n\n💡 Add a CLAUDE.md file to use this repo with Claude Code", repoName)
			}

			b.sessionMgr.SetActiveRepo(chatID, r.Name, r.Path)
			return fmt.Sprintf("✅ Now using: *%s*\n\n💡 Run commands with: @codebutler run <prompt>", r.Name)
		}
	}

	return fmt.Sprintf("❌ Repository '%s' not found\n\n💡 List repos with: @codebutler repos", repoName)
}

func (b *Bot) handleStatus(chatID string) string {
	repoName, repoPath, err := b.sessionMgr.GetActiveRepo(chatID)
	if err != nil {
		return "📍 No active repository\n\n💡 Select one with: @codebutler use <repo-name>"
	}

	return fmt.Sprintf("📍 Active: *%s*\n📂 Path: %s", repoName, repoPath)
}

func (b *Bot) handleRun(chatID, prompt string) string {
	repoName, repoPath, err := b.sessionMgr.GetActiveRepo(chatID)
	if err != nil {
		return "❌ No active repository\n\n💡 Select one first: @codebutler use <repo-name>"
	}

	if !b.executor.CheckInstalled() {
		return "❌ Claude Code CLI not installed\n\n💡 Install from: https://docs.anthropic.com/en/docs/claude-code"
	}

	// Start execution in background
	go b.executeInBackground(chatID, repoName, repoPath, prompt)

	return fmt.Sprintf("🤖 Executing in *%s*...\n\n```\n%s\n```\n\n⏳ This may take a few minutes...", repoName, prompt)
}

func (b *Bot) executeInBackground(chatID, repoName, repoPath, prompt string) {
	start := time.Now()

	// Execute Claude Code
	result, err := b.executor.ExecutePrompt(repoPath, prompt)

	duration := time.Since(start)

	// Build response message
	var response strings.Builder
	response.WriteString(fmt.Sprintf("✅ Execution completed in *%s*\n", repoName))
	response.WriteString(fmt.Sprintf("⏱️  Duration: %.1fs\n\n", duration.Seconds()))

	if err != nil {
		response.WriteString(fmt.Sprintf("❌ Error: %v\n\n", err))
		if result != nil && result.Stderr != "" {
			response.WriteString("```\n")
			response.WriteString(truncate(result.Stderr, 3000))
			response.WriteString("\n```")
		}
	} else {
		if result.Stdout != "" {
			response.WriteString("📤 Output:\n```\n")
			response.WriteString(truncate(result.Stdout, 3000))
			response.WriteString("\n```")
		}

		if result.Stderr != "" {
			response.WriteString("\n\n⚠️  Warnings:\n```\n")
			response.WriteString(truncate(result.Stderr, 1000))
			response.WriteString("\n```")
		}

		if result.Stdout == "" && result.Stderr == "" {
			response.WriteString("✅ Command completed successfully (no output)")
		}
	}

	// Send result back to chat
	if b.sendMessage != nil {
		if err := b.sendMessage(chatID, response.String()); err != nil {
			// Log error but don't crash
			fmt.Printf("Failed to send result: %v\n", err)
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

func (b *Bot) handleClear(chatID string) string {
	b.sessionMgr.ClearSession(chatID)
	return "✅ Session cleared\n\n💡 Select a repo with: @codebutler use <repo-name>"
}

func (b *Bot) GetActiveRepo(chatID string) (string, string, error) {
	return b.sessionMgr.GetActiveRepo(chatID)
}

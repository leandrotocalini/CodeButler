package commands

import (
	"fmt"
	"strings"
)

type CommandType string

const (
	CommandHelp   CommandType = "help"
	CommandRepos  CommandType = "repos"
	CommandUse    CommandType = "use"
	CommandRun    CommandType = "run"
	CommandStatus CommandType = "status"
	CommandClear  CommandType = "clear"
	CommandUnknown CommandType = "unknown"
)

type Command struct {
	Type CommandType
	Args []string
	Raw  string
}

func Parse(text string) *Command {
	text = strings.TrimSpace(text)

	// Support legacy @codebutler prefix (optional, case-insensitive)
	text = strings.TrimPrefix(text, "@codebutler")
	text = strings.TrimPrefix(text, "@CodeButler")
	text = strings.TrimPrefix(text, "@CODEBUTLER")
	text = strings.TrimSpace(text)

	if text == "" {
		return nil
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}

	// Clean command name: lowercase and remove trailing punctuation
	cmdName := strings.ToLower(parts[0])
	cmdName = strings.TrimRight(cmdName, ".,!?;:")
	args := parts[1:]

	cmd := &Command{
		Args: args,
		Raw:  text,
	}

	switch cmdName {
	case "help", "h":
		cmd.Type = CommandHelp
	case "repos", "list", "ls":
		cmd.Type = CommandRepos
	case "use", "select", "cd":
		cmd.Type = CommandUse
	case "run", "exec", "do":
		cmd.Type = CommandRun
	case "status", "current", "pwd":
		cmd.Type = CommandStatus
	case "clear", "reset":
		cmd.Type = CommandClear
	default:
		cmd.Type = CommandUnknown
	}

	return cmd
}

func (c *Command) GetArg(index int) string {
	if index < 0 || index >= len(c.Args) {
		return ""
	}
	return c.Args[index]
}

func (c *Command) GetArgsString() string {
	return strings.Join(c.Args, " ")
}

func (c *Command) HasArgs() bool {
	return len(c.Args) > 0
}

func GetHelpText() string {
	return `🤖 CodeButler Commands:

@codebutler help
   Show this help message

@codebutler repos
   List all available repositories

@codebutler use <repo-name>
   Select a repository to work with
   Example: @codebutler use aurum

@codebutler status
   Show current active repository

@codebutler run <prompt>
   Execute a Claude Code command in the active repo
   Example: @codebutler run add a new function to handle user login

@codebutler clear
   Clear active repository selection

💡 Tips:
- You must select a repo with 'use' before running commands
- Only repos with CLAUDE.md can be used
- Commands run in the background and may take time`
}

func ValidateCommand(cmd *Command) error {
	switch cmd.Type {
	case CommandUse:
		if !cmd.HasArgs() {
			return fmt.Errorf("'use' command requires a repository name\nUsage: use <repo-name>")
		}
	case CommandRun:
		if !cmd.HasArgs() {
			return fmt.Errorf("'run' command requires a prompt\nUsage: run <prompt>")
		}
	case CommandUnknown:
		// Extract the actual text user typed (first word)
		parts := strings.Fields(cmd.Raw)
		unknownCmd := "?"
		if len(parts) > 0 {
			unknownCmd = parts[0]
		}
		return fmt.Errorf("unknown command: '%s'\nType 'help' for available commands", unknownCmd)
	}
	return nil
}

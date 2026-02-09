package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
)

type Intent struct {
	Command    string   `json:"command"`    // One of: repos, use, status, run, clear, help, unknown
	Arguments  []string `json:"arguments"`  // Arguments for the command
	Confidence string   `json:"confidence"` // high, medium, low
}

type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []ClaudeMessage `json:"messages"`
}

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeResponse struct {
	Content []ClaudeContent `json:"content"`
}

type ClaudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseIntent uses Claude API to understand user intent and map to commands
func ParseIntent(userMessage string) (*Command, error) {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		// Fallback to direct parsing if no API key
		return Parse(userMessage), nil
	}

	systemPrompt := `You are a command intent parser for CodeButler.

Available commands:
- repos: List all repositories
- use <repo-name>: Select a repository
- status: Show active repository
- run <prompt>: Execute Claude Code command
- clear: Clear selection
- help: Show help
- unknown: Not a command

Return ONLY JSON:
{"command":"repos|use|status|run|clear|help|unknown","arguments":["arg1"],"confidence":"high|medium|low"}

Examples:
"repos" -> {"command":"repos","arguments":[],"confidence":"high"}
"Repos." -> {"command":"repos","arguments":[],"confidence":"high"}
"list repos" -> {"command":"repos","arguments":[],"confidence":"high"}
"muéstrame repositorios" -> {"command":"repos","arguments":[],"confidence":"high"}
"use aurum" -> {"command":"use","arguments":["aurum"],"confidence":"high"}
"selecciona aurum" -> {"command":"use","arguments":["aurum"],"confidence":"high"}
"run add tests" -> {"command":"run","arguments":["add","tests"],"confidence":"high"}
"agrega tests" -> {"command":"run","arguments":["agrega","tests"],"confidence":"medium"}
"hola" -> {"command":"unknown","arguments":[],"confidence":"high"}`

	reqBody := ClaudeRequest{
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 200,
		System:    systemPrompt,
		Messages: []ClaudeMessage{
			{Role: "user", Content: userMessage},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return Parse(userMessage), nil
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return Parse(userMessage), nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Parse(userMessage), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Fallback on error
		return Parse(userMessage), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Parse(userMessage), nil
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return Parse(userMessage), nil
	}

	if len(claudeResp.Content) == 0 {
		return Parse(userMessage), nil
	}

	// Parse intent JSON from Claude's response
	var intent Intent
	if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &intent); err != nil {
		return Parse(userMessage), nil
	}

	// Map intent to Command
	cmd := &Command{
		Args: intent.Arguments,
		Raw:  userMessage,
	}

	switch intent.Command {
	case "repos":
		cmd.Type = CommandRepos
	case "use":
		cmd.Type = CommandUse
	case "status":
		cmd.Type = CommandStatus
	case "run":
		cmd.Type = CommandRun
	case "clear":
		cmd.Type = CommandClear
	case "help":
		cmd.Type = CommandHelp
	default:
		cmd.Type = CommandUnknown
	}

	return cmd, nil
}

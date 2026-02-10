package config

type Config struct {
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	OpenAI   OpenAIConfig   `json:"openai"`
	Sources  SourcesConfig  `json:"sources"`
	Claude   ClaudeConfig   `json:"claude"`
}

type WhatsAppConfig struct {
	SessionPath    string `json:"sessionPath"`
	PersonalNumber string `json:"personalNumber"`
	GroupJID       string `json:"groupJID"`
	GroupName      string `json:"groupName"`
	BotPrefix      string `json:"botPrefix"` // Prefix for bot messages (e.g., "[BOT]")
}

type OpenAIConfig struct {
	APIKey string `json:"apiKey"`
}

type SourcesConfig struct {
	RootPath string `json:"rootPath"`
}

type ClaudeConfig struct {
	Command  string `json:"command"`  // Path to claude CLI (default: "claude")
	WorkDir  string `json:"workDir"`  // Working directory for Claude tasks
	MaxTurns int    `json:"maxTurns"` // Max agentic turns per task (default: 10)
}

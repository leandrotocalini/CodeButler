package config

// RepoConfig lives in <repo>/.codebutler/config.json
// Everything is per-repo: WhatsApp session, group, Claude settings.
type RepoConfig struct {
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Claude   ClaudeConfig   `json:"claude"`
	OpenAI   OpenAIConfig   `json:"openai,omitempty"`
}

type WhatsAppConfig struct {
	GroupJID  string `json:"groupJID"`
	GroupName string `json:"groupName"`
	BotPrefix string `json:"botPrefix"`
}

type ClaudeConfig struct {
	MaxTurns       int    `json:"maxTurns"`                 // default 10
	Timeout        int    `json:"timeout"`                  // minutes, default 30
	PermissionMode string `json:"permissionMode,omitempty"` // default "bypassPermissions"
}

type OpenAIConfig struct {
	APIKey string `json:"apiKey"` // for Whisper transcription
}

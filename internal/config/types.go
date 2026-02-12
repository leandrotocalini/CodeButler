package config

// RepoConfig lives in <repo>/.codebutler/config.json
// Everything is per-repo: WhatsApp session, group, Claude settings.
type RepoConfig struct {
	WhatsApp  WhatsAppConfig  `json:"whatsapp"`
	Slack     SlackRepoConfig `json:"slack,omitempty"`
	Claude    ClaudeConfig    `json:"claude"`
	OpenAI    OpenAIConfig    `json:"openai,omitempty"`
	Moonshot  MoonshotConfig  `json:"moonshot,omitempty"`
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

type MoonshotConfig struct {
	APIKey string `json:"apiKey"` // for Kimi draft mode
}

// SlackRepoConfig is the per-repo Slack config (channel ID only).
// Tokens live in ~/.codebutler/slack.json (global, shared across repos).
type SlackRepoConfig struct {
	ChannelID string `json:"channelID"` // Slack channel for this repo
}

// SlackGlobalConfig lives in ~/.codebutler/slack.json.
// One Slack app serves all repos; each repo just picks a channel.
type SlackGlobalConfig struct {
	BotToken string `json:"botToken"` // xoxb- Bot User OAuth Token
	AppToken string `json:"appToken"` // xapp- App-Level Token (Socket Mode)
}

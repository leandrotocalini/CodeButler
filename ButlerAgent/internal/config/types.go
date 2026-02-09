package config

type Config struct {
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	OpenAI   OpenAIConfig   `json:"openai"`
	Sources  SourcesConfig  `json:"sources"`
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

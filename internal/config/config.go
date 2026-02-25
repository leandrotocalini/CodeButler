// Package config handles loading and validation of global and per-repo
// configuration files for CodeButler.
package config

// GlobalConfig holds secrets loaded from ~/.codebutler/config.json.
// This file is never committed to git.
type GlobalConfig struct {
	Slack      GlobalSlack      `json:"slack"`
	OpenRouter GlobalOpenRouter `json:"openrouter"`
	OpenAI     GlobalOpenAI     `json:"openai"`
}

type GlobalSlack struct {
	BotToken string `json:"botToken"`
	AppToken string `json:"appToken"`
}

type GlobalOpenRouter struct {
	APIKey string `json:"apiKey"`
}

type GlobalOpenAI struct {
	APIKey string `json:"apiKey"`
}

// RepoConfig holds per-repo settings loaded from <repo>/.codebutler/config.json.
// This file is committed to git.
type RepoConfig struct {
	Slack      RepoSlack      `json:"slack"`
	Models     ModelsConfig   `json:"models"`
	MultiModel MultiModel     `json:"multiModel"`
	Limits     LimitsConfig   `json:"limits"`
}

type RepoSlack struct {
	ChannelID   string `json:"channelID"`
	ChannelName string `json:"channelName"`
}

// ModelsConfig maps each agent role to its model configuration.
type ModelsConfig struct {
	PM         *PMModelConfig      `json:"pm,omitempty"`
	Coder      *AgentModelConfig   `json:"coder,omitempty"`
	Reviewer   *AgentModelConfig   `json:"reviewer,omitempty"`
	Researcher *AgentModelConfig   `json:"researcher,omitempty"`
	Lead       *AgentModelConfig   `json:"lead,omitempty"`
	Artist     *ArtistModelConfig  `json:"artist,omitempty"`
}

// PMModelConfig supports a default model and a hot-swap pool.
type PMModelConfig struct {
	Default string            `json:"default"`
	Pool    map[string]string `json:"pool,omitempty"`
}

// AgentModelConfig holds a single model for a standard agent.
type AgentModelConfig struct {
	Model         string `json:"model"`
	FallbackModel string `json:"fallbackModel,omitempty"`
}

// ArtistModelConfig holds separate models for UX reasoning and image generation.
type ArtistModelConfig struct {
	UXModel    string `json:"uxModel"`
	ImageModel string `json:"imageModel"`
}

// MultiModel configures the pool of models for MultiModelFanOut.
type MultiModel struct {
	Models           []string `json:"models,omitempty"`
	MaxAgentsPerRound int      `json:"maxAgentsPerRound,omitempty"`
	MaxCostPerRound  float64  `json:"maxCostPerRound,omitempty"`
}

// LimitsConfig controls concurrency and rate limits.
type LimitsConfig struct {
	MaxConcurrentThreads int `json:"maxConcurrentThreads,omitempty"`
	MaxCallsPerHour      int `json:"maxCallsPerHour,omitempty"`
}

// Config is the fully merged configuration from global + per-repo sources.
type Config struct {
	Global GlobalConfig
	Repo   RepoConfig
}

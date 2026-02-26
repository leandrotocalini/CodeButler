package multimodel

import "time"

// ThinkerConfig defines a single sub-agent for a fan-out round.
type ThinkerConfig struct {
	Name         string `json:"name"`         // display name (e.g., "Security Reviewer")
	SystemPrompt string `json:"systemPrompt"` // custom system prompt
	Model        string `json:"model"`        // OpenRouter model ID
}

// ThinkerResult holds the response from a single sub-agent.
type ThinkerResult struct {
	Name     string        `json:"name"`
	Model    string        `json:"model"`
	Response string        `json:"response"`
	Error    string        `json:"error,omitempty"`
	Tokens   TokenUsage    `json:"tokens"`
	Duration time.Duration `json:"duration"`
}

// TokenUsage tracks token consumption for a single call.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// FanOutCost tracks the cost of a single fan-out round.
type FanOutCost struct {
	Thinkers     []ThinkerCost `json:"thinkers"`
	TotalUSD     float64       `json:"total_usd"`
	TotalTokens  int           `json:"total_tokens"`
	TotalDuration time.Duration `json:"total_duration"`
}

// ThinkerCost tracks the cost of a single thinker.
type ThinkerCost struct {
	Name         string        `json:"name"`
	Model        string        `json:"model"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	EstimatedUSD float64       `json:"estimated_usd"`
	Duration     time.Duration `json:"duration"`
}

// FanOutConfig configures a fan-out round.
type FanOutConfig struct {
	ModelPool        []string // allowed models
	MaxAgentsPerRound int     // max sub-agents per round
	MaxCostPerRound  float64 // soft cost limit in USD
}

// FanOutRequest is the input to a fan-out execution.
type FanOutRequest struct {
	Thinkers   []ThinkerConfig `json:"agents"`
	UserPrompt string          `json:"userPrompt"`
}

// FanOutResponse is the aggregated output of a fan-out round.
type FanOutResponse struct {
	Results   []ThinkerResult `json:"results"`
	Cost      FanOutCost      `json:"cost"`
	Succeeded int             `json:"succeeded"`
	Failed    int             `json:"failed"`
}

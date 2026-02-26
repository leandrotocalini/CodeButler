package multimodel

// modelPricing maps model IDs to per-million-token prices (input, output).
// These are approximate rates — used for estimation, not billing.
var modelPricing = map[string][2]float64{
	// Anthropic
	"anthropic/claude-opus-4-6":             {15.0, 75.0},
	"anthropic/claude-sonnet-4-5-20250929":  {3.0, 15.0},
	"anthropic/claude-sonnet-4-20250514":    {3.0, 15.0},
	// OpenAI
	"openai/o3":                {10.0, 40.0},
	"openai/gpt-4o":            {2.5, 10.0},
	"openai/gpt-4o-mini":       {0.15, 0.6},
	// Google
	"google/gemini-2.5-pro":    {1.25, 10.0},
	"google/gemini-2.0-flash":  {0.1, 0.4},
	// DeepSeek
	"deepseek/deepseek-r1":     {0.55, 2.19},
	"deepseek/deepseek-chat":   {0.14, 0.28},
	// Moonshot
	"moonshotai/kimi-k2":       {0.6, 2.0},
}

// defaultInputPrice is used when a model isn't in the pricing table.
const defaultInputPrice = 3.0
const defaultOutputPrice = 15.0

// EstimateCost estimates the cost of a fan-out round before execution.
// Uses estimated prompt tokens (based on prompt length) and assumed output.
func EstimateCost(thinkers []ThinkerConfig, userPrompt string) float64 {
	var total float64
	for _, t := range thinkers {
		total += EstimateThinkerCost(t, userPrompt)
	}
	return total
}

// EstimateThinkerCost estimates the cost of a single thinker call.
func EstimateThinkerCost(t ThinkerConfig, userPrompt string) float64 {
	// Rough token estimate: 1 token ≈ 4 chars
	inputTokens := (len(t.SystemPrompt) + len(userPrompt)) / 4
	outputTokens := 1000 // assume ~1K output tokens per response

	inputPrice, outputPrice := modelPrice(t.Model)

	cost := float64(inputTokens)/1_000_000*inputPrice +
		float64(outputTokens)/1_000_000*outputPrice

	return cost
}

// CalculateThinkerCost calculates actual cost from real token usage.
func CalculateThinkerCost(model string, usage TokenUsage) float64 {
	inputPrice, outputPrice := modelPrice(model)

	return float64(usage.PromptTokens)/1_000_000*inputPrice +
		float64(usage.CompletionTokens)/1_000_000*outputPrice
}

// modelPrice returns the input and output price per million tokens.
func modelPrice(model string) (float64, float64) {
	if prices, ok := modelPricing[model]; ok {
		return prices[0], prices[1]
	}
	return defaultInputPrice, defaultOutputPrice
}

// CalculateFanOutCost aggregates costs from a completed fan-out round.
func CalculateFanOutCost(results []ThinkerResult) FanOutCost {
	cost := FanOutCost{
		Thinkers: make([]ThinkerCost, len(results)),
	}

	for i, r := range results {
		usd := CalculateThinkerCost(r.Model, r.Tokens)
		cost.Thinkers[i] = ThinkerCost{
			Name:         r.Name,
			Model:        r.Model,
			InputTokens:  r.Tokens.PromptTokens,
			OutputTokens: r.Tokens.CompletionTokens,
			EstimatedUSD: usd,
			Duration:     r.Duration,
		}
		cost.TotalUSD += usd
		cost.TotalTokens += r.Tokens.TotalTokens
		if r.Duration > cost.TotalDuration {
			cost.TotalDuration = r.Duration // wall clock = max duration
		}
	}

	return cost
}

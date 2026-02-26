package multimodel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

// ChatRequest is a minimal chat completion request.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []ChatMsg `json:"messages"`
}

// ChatMsg is a single message in a chat request.
type ChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the result of a chat completion.
type ChatResponse struct {
	Content  string     `json:"content"`
	Usage    TokenUsage `json:"usage"`
	Duration time.Duration
}

// LLMProvider makes chat completion calls. Satisfied by the OpenRouter client.
type LLMProvider interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// FanOut executes parallel single-shot LLM calls to multiple models.
// Each thinker gets a custom system prompt + the shared user prompt.
// Errors in individual calls don't cancel others.
func FanOut(ctx context.Context, provider LLMProvider, req FanOutRequest, logger *slog.Logger) *FanOutResponse {
	if logger == nil {
		logger = slog.Default()
	}

	results := make([]ThinkerResult, len(req.Thinkers))
	g, gctx := errgroup.WithContext(ctx)

	for i, t := range req.Thinkers {
		g.Go(func() error {
			start := time.Now()

			chatReq := ChatRequest{
				Model: t.Model,
				Messages: []ChatMsg{
					{Role: "system", Content: t.SystemPrompt},
					{Role: "user", Content: req.UserPrompt},
				},
			}

			resp, err := provider.ChatCompletion(gctx, chatReq)
			duration := time.Since(start)

			if err != nil {
				logger.Warn("thinker call failed",
					"thinker", t.Name,
					"model", t.Model,
					"error", err,
					"duration", duration,
				)
				results[i] = ThinkerResult{
					Name:     t.Name,
					Model:    t.Model,
					Error:    err.Error(),
					Duration: duration,
				}
				return nil // don't cancel errgroup
			}

			results[i] = ThinkerResult{
				Name:     t.Name,
				Model:    t.Model,
				Response: resp.Content,
				Tokens:   resp.Usage,
				Duration: duration,
			}

			logger.Info("thinker completed",
				"thinker", t.Name,
				"model", t.Model,
				"tokens", resp.Usage.TotalTokens,
				"duration", duration,
			)

			return nil
		})
	}

	g.Wait()

	// Aggregate results
	var succeeded, failed int
	for _, r := range results {
		if r.Error != "" {
			failed++
		} else {
			succeeded++
		}
	}

	cost := CalculateFanOutCost(results)

	return &FanOutResponse{
		Results:   results,
		Cost:      cost,
		Succeeded: succeeded,
		Failed:    failed,
	}
}

// Validate checks that the fan-out request is valid against the config.
func Validate(req FanOutRequest, config FanOutConfig) error {
	if len(req.Thinkers) == 0 {
		return fmt.Errorf("no thinkers specified")
	}

	if config.MaxAgentsPerRound > 0 && len(req.Thinkers) > config.MaxAgentsPerRound {
		return fmt.Errorf("too many thinkers: %d exceeds max %d", len(req.Thinkers), config.MaxAgentsPerRound)
	}

	// Check for duplicate models
	seen := make(map[string]bool)
	for _, t := range req.Thinkers {
		if seen[t.Model] {
			return fmt.Errorf("duplicate model %q — each thinker must use a different model", t.Model)
		}
		seen[t.Model] = true
	}

	// Check models are in pool (if pool is specified)
	if len(config.ModelPool) > 0 {
		poolSet := make(map[string]bool)
		for _, m := range config.ModelPool {
			poolSet[m] = true
		}
		for _, t := range req.Thinkers {
			if !poolSet[t.Model] {
				return fmt.Errorf("model %q not in allowed pool", t.Model)
			}
		}
	}

	// Check names are non-empty
	for _, t := range req.Thinkers {
		if t.Name == "" {
			return fmt.Errorf("thinker name cannot be empty")
		}
		if t.SystemPrompt == "" {
			return fmt.Errorf("thinker %q has empty system prompt", t.Name)
		}
		if t.Model == "" {
			return fmt.Errorf("thinker %q has empty model", t.Name)
		}
	}

	if req.UserPrompt == "" {
		return fmt.Errorf("user prompt cannot be empty")
	}

	return nil
}

// CheckCostLimit checks if the estimated cost exceeds the configured limit.
// Returns the estimated cost and whether it exceeds the limit.
func CheckCostLimit(req FanOutRequest, config FanOutConfig) (float64, bool) {
	estimated := EstimateCost(req.Thinkers, req.UserPrompt)
	if config.MaxCostPerRound <= 0 {
		return estimated, false
	}
	return estimated, estimated > config.MaxCostPerRound
}

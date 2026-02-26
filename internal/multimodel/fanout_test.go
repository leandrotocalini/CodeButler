package multimodel

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockProvider implements LLMProvider for testing.
type mockProvider struct {
	responses map[string]*ChatResponse
	errors    map[string]error
	delay     time.Duration
}

func (m *mockProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if err, ok := m.errors[req.Model]; ok {
		return nil, err
	}
	if resp, ok := m.responses[req.Model]; ok {
		return resp, nil
	}
	return &ChatResponse{Content: "default response from " + req.Model}, nil
}

func TestFanOut_AllSucceed(t *testing.T) {
	provider := &mockProvider{
		responses: map[string]*ChatResponse{
			"model-a": {Content: "idea A", Usage: TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}},
			"model-b": {Content: "idea B", Usage: TokenUsage{PromptTokens: 100, CompletionTokens: 60, TotalTokens: 160}},
			"model-c": {Content: "idea C", Usage: TokenUsage{PromptTokens: 100, CompletionTokens: 70, TotalTokens: 170}},
		},
	}

	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "Analyst", SystemPrompt: "You are an analyst.", Model: "model-a"},
			{Name: "Designer", SystemPrompt: "You are a designer.", Model: "model-b"},
			{Name: "Engineer", SystemPrompt: "You are an engineer.", Model: "model-c"},
		},
		UserPrompt: "What should we build?",
	}

	resp := FanOut(context.Background(), provider, req, nil)

	if resp.Succeeded != 3 {
		t.Errorf("expected 3 succeeded, got %d", resp.Succeeded)
	}
	if resp.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", resp.Failed)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}

	for i, r := range resp.Results {
		if r.Error != "" {
			t.Errorf("result %d has error: %s", i, r.Error)
		}
		if r.Response == "" {
			t.Errorf("result %d has empty response", i)
		}
	}
}

func TestFanOut_PartialFailure(t *testing.T) {
	provider := &mockProvider{
		responses: map[string]*ChatResponse{
			"model-a": {Content: "idea A", Usage: TokenUsage{TotalTokens: 100}},
			"model-b": {Content: "idea B", Usage: TokenUsage{TotalTokens: 100}},
		},
		errors: map[string]error{
			"model-c": fmt.Errorf("rate limited"),
		},
	}

	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "Analyst", SystemPrompt: "prompt", Model: "model-a"},
			{Name: "Designer", SystemPrompt: "prompt", Model: "model-b"},
			{Name: "Engineer", SystemPrompt: "prompt", Model: "model-c"},
		},
		UserPrompt: "What should we build?",
	}

	resp := FanOut(context.Background(), provider, req, nil)

	if resp.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", resp.Succeeded)
	}
	if resp.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", resp.Failed)
	}

	// Find the failed result
	var failedResult *ThinkerResult
	for i := range resp.Results {
		if resp.Results[i].Error != "" {
			failedResult = &resp.Results[i]
			break
		}
	}
	if failedResult == nil {
		t.Fatal("expected one failed result")
	}
	if failedResult.Model != "model-c" {
		t.Errorf("expected model-c to fail, got %s", failedResult.Model)
	}
}

func TestFanOut_AllFail(t *testing.T) {
	provider := &mockProvider{
		errors: map[string]error{
			"model-a": fmt.Errorf("error a"),
			"model-b": fmt.Errorf("error b"),
		},
	}

	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "p", Model: "model-a"},
			{Name: "B", SystemPrompt: "p", Model: "model-b"},
		},
		UserPrompt: "test",
	}

	resp := FanOut(context.Background(), provider, req, nil)

	if resp.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", resp.Succeeded)
	}
	if resp.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", resp.Failed)
	}
}

func TestFanOut_CostTracking(t *testing.T) {
	provider := &mockProvider{
		responses: map[string]*ChatResponse{
			"anthropic/claude-opus-4-6": {
				Content: "deep analysis",
				Usage:   TokenUsage{PromptTokens: 500, CompletionTokens: 1000, TotalTokens: 1500},
			},
			"openai/gpt-4o": {
				Content: "quick take",
				Usage:   TokenUsage{PromptTokens: 500, CompletionTokens: 800, TotalTokens: 1300},
			},
		},
	}

	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "Deep", SystemPrompt: "analyze deeply", Model: "anthropic/claude-opus-4-6"},
			{Name: "Quick", SystemPrompt: "quick analysis", Model: "openai/gpt-4o"},
		},
		UserPrompt: "What's the best approach?",
	}

	resp := FanOut(context.Background(), provider, req, nil)

	if resp.Cost.TotalTokens != 2800 {
		t.Errorf("expected 2800 total tokens, got %d", resp.Cost.TotalTokens)
	}
	if resp.Cost.TotalUSD <= 0 {
		t.Error("expected non-zero total cost")
	}
	if len(resp.Cost.Thinkers) != 2 {
		t.Errorf("expected 2 thinker costs, got %d", len(resp.Cost.Thinkers))
	}
}

func TestValidate_Valid(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "prompt", Model: "model-a"},
			{Name: "B", SystemPrompt: "prompt", Model: "model-b"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{
		ModelPool:        []string{"model-a", "model-b", "model-c"},
		MaxAgentsPerRound: 6,
		MaxCostPerRound:  1.0,
	}

	if err := Validate(req, config); err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidate_DuplicateModels(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "prompt", Model: "model-a"},
			{Name: "B", SystemPrompt: "prompt", Model: "model-a"}, // duplicate!
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{ModelPool: []string{"model-a"}}

	err := Validate(req, config)
	if err == nil {
		t.Error("expected error for duplicate models")
	}
}

func TestValidate_TooManyThinkers(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "p", Model: "m-a"},
			{Name: "B", SystemPrompt: "p", Model: "m-b"},
			{Name: "C", SystemPrompt: "p", Model: "m-c"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{MaxAgentsPerRound: 2}

	err := Validate(req, config)
	if err == nil {
		t.Error("expected error for too many thinkers")
	}
}

func TestValidate_ModelNotInPool(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "p", Model: "model-x"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{ModelPool: []string{"model-a", "model-b"}}

	err := Validate(req, config)
	if err == nil {
		t.Error("expected error for model not in pool")
	}
}

func TestValidate_EmptyThinkers(t *testing.T) {
	req := FanOutRequest{
		Thinkers:   []ThinkerConfig{},
		UserPrompt: "test",
	}

	err := Validate(req, FanOutConfig{})
	if err == nil {
		t.Error("expected error for empty thinkers")
	}
}

func TestValidate_EmptyPrompt(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "p", Model: "m"},
		},
		UserPrompt: "",
	}

	err := Validate(req, FanOutConfig{})
	if err == nil {
		t.Error("expected error for empty prompt")
	}
}

func TestValidate_EmptyName(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "", SystemPrompt: "p", Model: "m"},
		},
		UserPrompt: "test",
	}

	err := Validate(req, FanOutConfig{})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidate_NoPoolRestriction(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "p", Model: "any-model"},
		},
		UserPrompt: "test",
	}

	// Empty pool = no restriction
	config := FanOutConfig{}

	if err := Validate(req, config); err != nil {
		t.Errorf("expected valid with no pool restriction: %v", err)
	}
}

func TestEstimateCost(t *testing.T) {
	thinkers := []ThinkerConfig{
		{Name: "A", SystemPrompt: "prompt", Model: "anthropic/claude-opus-4-6"},
		{Name: "B", SystemPrompt: "prompt", Model: "openai/gpt-4o"},
	}

	cost := EstimateCost(thinkers, "What should we build?")
	if cost <= 0 {
		t.Error("expected positive cost estimate")
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	thinkers := []ThinkerConfig{
		{Name: "A", SystemPrompt: "prompt", Model: "unknown/model"},
	}

	cost := EstimateCost(thinkers, "test")
	if cost <= 0 {
		t.Error("expected positive cost even for unknown model (uses defaults)")
	}
}

func TestCheckCostLimit_Under(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "short", Model: "openai/gpt-4o-mini"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{MaxCostPerRound: 100.0} // very high limit

	est, exceeds := CheckCostLimit(req, config)
	if exceeds {
		t.Errorf("expected under limit, estimated: %f", est)
	}
}

func TestCheckCostLimit_Over(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "prompt", Model: "anthropic/claude-opus-4-6"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{MaxCostPerRound: 0.0000001} // impossibly low

	_, exceeds := CheckCostLimit(req, config)
	if !exceeds {
		t.Error("expected to exceed limit")
	}
}

func TestCheckCostLimit_NoLimit(t *testing.T) {
	req := FanOutRequest{
		Thinkers: []ThinkerConfig{
			{Name: "A", SystemPrompt: "prompt", Model: "anthropic/claude-opus-4-6"},
		},
		UserPrompt: "test",
	}

	config := FanOutConfig{MaxCostPerRound: 0} // no limit

	_, exceeds := CheckCostLimit(req, config)
	if exceeds {
		t.Error("expected no limit exceeded when MaxCostPerRound is 0")
	}
}

func TestCalculateFanOutCost(t *testing.T) {
	results := []ThinkerResult{
		{
			Name:  "A",
			Model: "anthropic/claude-opus-4-6",
			Tokens: TokenUsage{
				PromptTokens:     1000,
				CompletionTokens: 500,
				TotalTokens:      1500,
			},
			Duration: 2 * time.Second,
		},
		{
			Name:  "B",
			Model: "openai/gpt-4o",
			Tokens: TokenUsage{
				PromptTokens:     1000,
				CompletionTokens: 800,
				TotalTokens:      1800,
			},
			Duration: 1 * time.Second,
		},
	}

	cost := CalculateFanOutCost(results)

	if cost.TotalTokens != 3300 {
		t.Errorf("expected 3300 total tokens, got %d", cost.TotalTokens)
	}
	if cost.TotalUSD <= 0 {
		t.Error("expected non-zero total cost")
	}
	// Wall clock should be max of durations
	if cost.TotalDuration != 2*time.Second {
		t.Errorf("expected 2s wall clock, got %v", cost.TotalDuration)
	}
	if len(cost.Thinkers) != 2 {
		t.Errorf("expected 2 thinker costs, got %d", len(cost.Thinkers))
	}
}

func TestCalculateThinkerCost_KnownModel(t *testing.T) {
	usage := TokenUsage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}
	cost := CalculateThinkerCost("anthropic/claude-opus-4-6", usage)

	// $15/M input + $75/M output = $90
	if cost < 89 || cost > 91 {
		t.Errorf("expected ~$90, got $%.2f", cost)
	}
}

func TestCalculateThinkerCost_UnknownModel(t *testing.T) {
	usage := TokenUsage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}
	cost := CalculateThinkerCost("unknown/model", usage)

	// defaults: $3/M input + $15/M output = $18
	if cost < 17 || cost > 19 {
		t.Errorf("expected ~$18, got $%.2f", cost)
	}
}

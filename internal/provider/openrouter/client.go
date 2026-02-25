package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	defaultTimeout = 120 * time.Second
)

// Client is an HTTP client for the OpenRouter chat completions API.
// It handles retries with exponential backoff + jitter, and per-model
// circuit breakers via sony/gobreaker.
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	logger     *slog.Logger
	sleepFn    func(context.Context, time.Duration) // for testing

	mu       sync.Mutex
	breakers map[string]*gobreaker.CircuitBreaker[*ChatResponse]
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		cl.httpClient = c
	}
}

// WithBaseURL overrides the default OpenRouter base URL.
func WithBaseURL(url string) Option {
	return func(cl *Client) {
		cl.baseURL = url
	}
}

// WithLogger sets a structured logger for the client.
func WithLogger(l *slog.Logger) Option {
	return func(cl *Client) {
		cl.logger = l
	}
}

// WithSleepFunc overrides the retry sleep function (for testing).
func WithSleepFunc(fn func(context.Context, time.Duration)) Option {
	return func(cl *Client) {
		cl.sleepFn = fn
	}
}

// defaultSleep is the production sleep function — respects context cancellation.
func defaultSleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// NewClient creates an OpenRouter client with the given API key and options.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: defaultTimeout},
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		logger:     slog.Default(),
		sleepFn:    defaultSleep,
		breakers:   make(map[string]*gobreaker.CircuitBreaker[*ChatResponse]),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ChatCompletion makes a single chat completion request to OpenRouter.
// It handles retries and circuit breaking transparently.
func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	cb := c.getOrCreateBreaker(req.Model)

	resp, err := cb.Execute(func() (*ChatResponse, error) {
		return c.chatCompletionWithRetry(ctx, req)
	})
	if err != nil {
		// Wrap gobreaker sentinel errors for clarity.
		if err == gobreaker.ErrOpenState {
			return nil, &ClassifiedError{
				Type:    ErrProviderOverloaded,
				Message: fmt.Sprintf("circuit breaker open for model %s", req.Model),
			}
		}
		if err == gobreaker.ErrTooManyRequests {
			return nil, &ClassifiedError{
				Type:    ErrRateLimit,
				Message: fmt.Sprintf("circuit breaker half-open, too many probes for model %s", req.Model),
			}
		}
		return nil, err
	}
	return resp, nil
}

// chatCompletionWithRetry executes the HTTP request with retry logic.
func (c *Client) chatCompletionWithRetry(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.doRequest(ctx, req)
		if err == nil {
			return resp, nil
		}

		classified, ok := err.(*ClassifiedError)
		if !ok {
			// Non-classified error (e.g., context canceled, network failure).
			return nil, err
		}

		if !classified.Retryable() || attempt >= classified.MaxRetries() {
			return nil, classified
		}

		delay := c.retryDelay(classified, attempt)

		c.logger.Warn("retrying OpenRouter request",
			"model", req.Model,
			"error_type", classified.Type.String(),
			"attempt", attempt+1,
			"delay", delay,
		)

		c.sleepFn(ctx, delay)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

// doRequest performs a single HTTP request and parses the response.
func (c *Client) doRequest(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Classify network/timeout errors.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ClassifiedError{
			Type:    ErrTimeout,
			Message: err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ClassifiedError{
			Type:    ErrMalformedResponse,
			Message: fmt.Sprintf("read response body: %v", err),
		}
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, &ClassifiedError{
			Type:    ErrMalformedResponse,
			Message: fmt.Sprintf("parse response JSON: %v", err),
		}
	}

	if len(chatResp.Choices) == 0 {
		return nil, &ClassifiedError{
			Type:    ErrMalformedResponse,
			Message: "response contains no choices",
		}
	}

	return &chatResp, nil
}

// retryDelay calculates the delay before the next retry attempt.
// Uses exponential backoff + jitter. For rate limits, respects Retry-After.
func (c *Client) retryDelay(err *ClassifiedError, attempt int) time.Duration {
	if err.Type == ErrRateLimit && err.RetryAfter > 0 {
		return jitter(err.RetryAfter)
	}

	// Exponential backoff: 1s, 2s, 4s, 8s, 16s
	base := time.Second * time.Duration(1<<uint(attempt))
	if base > 16*time.Second {
		base = 16 * time.Second
	}
	return jitter(base)
}

// jitter applies random jitter: delay * (0.5 + rand.Float64()).
// This prevents thundering herd when multiple agents retry simultaneously.
func jitter(d time.Duration) time.Duration {
	factor := 0.5 + rand.Float64() // [0.5, 1.5)
	return time.Duration(float64(d) * factor)
}

// getOrCreateBreaker returns the circuit breaker for the given model,
// creating one if it doesn't exist. Per-model breakers isolate failures.
func (c *Client) getOrCreateBreaker(model string) *gobreaker.CircuitBreaker[*ChatResponse] {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cb, ok := c.breakers[model]; ok {
		return cb
	}

	cb := gobreaker.NewCircuitBreaker[*ChatResponse](gobreaker.Settings{
		Name:        "openrouter-" + model,
		MaxRequests: 1,                // Allow 1 probe request in half-open state
		Interval:    0,                // Don't clear counts in closed state
		Timeout:     30 * time.Second, // Time to wait before probing after open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			c.logger.Info("circuit breaker state change",
				"breaker", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			// Don't count client errors (auth, content filter) as circuit breaker failures.
			classified, ok := err.(*ClassifiedError)
			if !ok {
				return false
			}
			switch classified.Type {
			case ErrAuth, ErrContentFiltered, ErrContextTooLong:
				return true // These are not provider failures.
			default:
				return false
			}
		},
	})

	c.breakers[model] = cb
	return cb
}

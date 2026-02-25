package openrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrorType classifies LLM API errors for appropriate retry/handling strategy.
type ErrorType int

const (
	ErrRateLimit           ErrorType = iota // HTTP 429
	ErrProviderOverloaded                   // HTTP 502, 503
	ErrContextTooLong                       // HTTP 400 + context_length_exceeded
	ErrContentFiltered                      // HTTP 400 + content_filter
	ErrAuth                                 // HTTP 401, 403
	ErrMalformedResponse                    // JSON parse failure
	ErrTimeout                              // Request deadline exceeded
	ErrUnknown                              // Anything else
)

// String returns the human-readable name of the error type.
func (e ErrorType) String() string {
	switch e {
	case ErrRateLimit:
		return "rate_limit"
	case ErrProviderOverloaded:
		return "provider_overloaded"
	case ErrContextTooLong:
		return "context_length_exceeded"
	case ErrContentFiltered:
		return "content_filter"
	case ErrAuth:
		return "auth_error"
	case ErrMalformedResponse:
		return "malformed_response"
	case ErrTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ClassifiedError wraps an API error with its classification and metadata.
type ClassifiedError struct {
	Type       ErrorType
	StatusCode int
	Message    string
	RetryAfter time.Duration // Only set for rate limit errors
}

func (e *ClassifiedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("openrouter %s (HTTP %d): %s (retry after %s)", e.Type, e.StatusCode, e.Message, e.RetryAfter)
	}
	return fmt.Sprintf("openrouter %s (HTTP %d): %s", e.Type, e.StatusCode, e.Message)
}

// Retryable returns true if this error type supports automatic retry.
func (e *ClassifiedError) Retryable() bool {
	switch e.Type {
	case ErrRateLimit, ErrProviderOverloaded, ErrTimeout, ErrMalformedResponse, ErrContextTooLong:
		return true
	default:
		return false
	}
}

// MaxRetries returns the maximum number of retries for this error type.
func (e *ClassifiedError) MaxRetries() int {
	switch e.Type {
	case ErrRateLimit:
		return 5
	case ErrProviderOverloaded:
		return 5
	case ErrContextTooLong:
		return 1
	case ErrMalformedResponse:
		return 3
	case ErrTimeout:
		return 1
	default:
		return 0
	}
}

// openRouterErrorBody is the JSON error body returned by OpenRouter.
type openRouterErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// classifyHTTPError classifies an HTTP response as a specific error type.
func classifyHTTPError(resp *http.Response) *ClassifiedError {
	body, _ := io.ReadAll(resp.Body)

	var errBody openRouterErrorBody
	json.Unmarshal(body, &errBody) //nolint:errcheck // best-effort parse

	msg := errBody.Error.Message
	if msg == "" {
		msg = string(body)
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return &ClassifiedError{
			Type:       ErrRateLimit,
			StatusCode: resp.StatusCode,
			Message:    msg,
			RetryAfter: retryAfter,
		}

	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return &ClassifiedError{
			Type:       ErrProviderOverloaded,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}

	case http.StatusUnauthorized, http.StatusForbidden:
		return &ClassifiedError{
			Type:       ErrAuth,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}

	case http.StatusBadRequest:
		return classifyBadRequest(resp.StatusCode, msg, errBody)

	default:
		return &ClassifiedError{
			Type:       ErrUnknown,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}
	}
}

// classifyBadRequest further classifies HTTP 400 errors by examining the error body.
func classifyBadRequest(statusCode int, msg string, errBody openRouterErrorBody) *ClassifiedError {
	code := errBody.Error.Code
	errType := errBody.Error.Type
	combined := strings.ToLower(code + " " + errType + " " + msg)

	if strings.Contains(combined, "context_length_exceeded") ||
		strings.Contains(combined, "maximum context length") ||
		strings.Contains(combined, "too many tokens") {
		return &ClassifiedError{
			Type:       ErrContextTooLong,
			StatusCode: statusCode,
			Message:    msg,
		}
	}

	if strings.Contains(combined, "content_filter") ||
		strings.Contains(combined, "content_policy") ||
		strings.Contains(combined, "flagged") {
		return &ClassifiedError{
			Type:       ErrContentFiltered,
			StatusCode: statusCode,
			Message:    msg,
		}
	}

	return &ClassifiedError{
		Type:       ErrUnknown,
		StatusCode: statusCode,
		Message:    msg,
	}
}

// parseRetryAfter parses the Retry-After header value as seconds.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	secs, err := strconv.Atoi(header)
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

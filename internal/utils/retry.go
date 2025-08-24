package utils

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay between retries
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffMultiple float64       // Multiplier for exponential backoff
	Jitter          bool          // Add random jitter to prevent thundering herd
}

// DefaultRetryConfig returns sensible defaults for retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      config.GetDefaultMaxRetries(),
		InitialDelay:    config.GetDefaultInitialDelay(),
		MaxDelay:        config.GetDefaultMaxDelay(),
		BackoffMultiple: config.DefaultBackoffMultiple,
		Jitter:          config.DefaultJitterEnabled,
	}
}

// RetryableError represents an error that can be retried
type RetryableError struct {
	Err         error
	StatusCode  int
	ShouldRetry bool
	RetryAfter  time.Duration // From Retry-After header
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error (status: %d, should_retry: %t): %v", e.StatusCode, e.ShouldRetry, e.Err)
}

// IsRetryableStatusCode determines if an HTTP status code should be retried
func IsRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429 - Rate limited
		http.StatusInternalServerError, // 500 - Internal server error
		http.StatusBadGateway,          // 502 - Bad gateway
		http.StatusServiceUnavailable,  // 503 - Service unavailable
		http.StatusGatewayTimeout:      // 504 - Gateway timeout
		return true
	default:
		return false
	}
}

// RetryableHTTPClient wraps an HTTP client with retry logic
type RetryableHTTPClient struct {
	Client *http.Client
	Config RetryConfig
}

// NewRetryableHTTPClient creates a new retryable HTTP client
func NewRetryableHTTPClient(client *http.Client, config RetryConfig) *RetryableHTTPClient {
	if client == nil {
		// Default to no timeout - let context timeouts handle this
		client = &http.Client{}
	}

	return &RetryableHTTPClient{
		Client: client,
		Config: config,
	}
}

// Do executes an HTTP request with retry logic
func (r *RetryableHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= r.Config.MaxRetries; attempt++ {
		// Clone request for retry attempts
		reqClone := req.Clone(req.Context())

		// Execute request
		resp, err := r.Client.Do(reqClone)

		// If no error and successful status, return immediately
		if err == nil && !IsRetryableStatusCode(resp.StatusCode) {
			return resp, nil
		}

		// If MaxRetries is 0, return immediately regardless of status
		// This allows the caller to handle HTTP error responses directly
		if r.Config.MaxRetries == 0 {
			return resp, err
		}

		// Handle different error scenarios
		if err != nil {
			lastErr = &RetryableError{
				Err:         err,
				ShouldRetry: true, // Network errors are generally retryable
			}
		} else {
			// HTTP error response
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			lastErr = &RetryableError{
				Err:         fmt.Errorf("HTTP %d", resp.StatusCode),
				StatusCode:  resp.StatusCode,
				ShouldRetry: IsRetryableStatusCode(resp.StatusCode),
				RetryAfter:  retryAfter,
			}
			resp.Body.Close() // Don't leak connections
		}

		// Don't retry if this is not a retryable error
		if retryErr, ok := lastErr.(*RetryableError); ok && !retryErr.ShouldRetry {
			return nil, lastErr
		}

		// Don't wait after the last attempt
		if attempt == r.Config.MaxRetries {
			break
		}

		// Calculate delay for next attempt
		delay := r.calculateDelay(attempt, lastErr.(*RetryableError).RetryAfter)

		// Wait before retry, respecting context cancellation
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", r.Config.MaxRetries, lastErr)
}

// calculateDelay computes the delay before the next retry attempt
func (r *RetryableHTTPClient) calculateDelay(attempt int, retryAfter time.Duration) time.Duration {
	// If server specified Retry-After, respect it
	if retryAfter > 0 {
		if retryAfter > r.Config.MaxDelay {
			return r.Config.MaxDelay
		}
		return retryAfter
	}

	// Calculate exponential backoff
	delay := float64(r.Config.InitialDelay) * math.Pow(r.Config.BackoffMultiple, float64(attempt))

	// Apply jitter to prevent thundering herd
	if r.Config.Jitter {
		// Add ±20% jitter
		jitter := delay * 0.2 * (2*float64(time.Now().UnixNano())/1e9 - 1) // Simplified random -1 to 1
		delay += jitter
	}

	// Cap at maximum delay
	if delay > float64(r.Config.MaxDelay) {
		delay = float64(r.Config.MaxDelay)
	}

	// Ensure minimum delay
	if delay < float64(r.Config.InitialDelay) {
		delay = float64(r.Config.InitialDelay)
	}

	return time.Duration(delay)
}

// parseRetryAfter parses the Retry-After header value
func parseRetryAfter(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 0
	}

	// Try parsing as seconds
	if d, err := time.ParseDuration(retryAfter + "s"); err == nil {
		return d
	}

	// Try parsing as HTTP date (not implemented for simplicity)
	// This would require parsing RFC1123 format dates

	return 0
}

// RetryFunc is a function that can be retried
type RetryFunc func() error

// WithRetry executes a function with retry logic
func WithRetry(ctx context.Context, config RetryConfig, fn RetryFunc) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		err := fn()

		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if retryErr, ok := err.(*RetryableError); ok && !retryErr.ShouldRetry {
			return err // Not retryable
		}

		// Don't wait after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay
		delay := time.Duration(float64(config.InitialDelay) * math.Pow(config.BackoffMultiple, float64(attempt)))
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", config.MaxRetries, lastErr)
}

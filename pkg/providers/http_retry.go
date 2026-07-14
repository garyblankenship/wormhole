package providers

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// retryConfig holds configuration for provider HTTP retries.
type retryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay between retries
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffMultiple float64       // Multiplier for exponential backoff
	Jitter          bool          // Add random jitter to prevent thundering herd
}

func defaultRetryConfig() retryConfig {
	return retryConfig{
		MaxRetries:      config.GetDefaultMaxRetries(),
		InitialDelay:    config.GetDefaultInitialDelay(),
		MaxDelay:        config.GetDefaultMaxDelay(),
		BackoffMultiple: config.DefaultBackoffMultiple,
		Jitter:          config.DefaultJitterEnabled,
	}
}

// maxErrorBodyBytes bounds how much of a provider error body we retain for
// downstream classification (error bodies are small; this is a safety cap).
const maxErrorBodyBytes = 64 << 10

type retryableError struct {
	Err         error
	StatusCode  int
	ShouldRetry bool
	RetryAfter  time.Duration // From Retry-After header
	Body        []byte        // Bounded copy of the error response body, for downstream classification
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable error (status: %d, should_retry: %t): %v", e.StatusCode, e.ShouldRetry, e.Err)
}

// Unwrap returns the underlying error for error unwrapping
func (e *retryableError) Unwrap() error {
	return e.Err
}

// statusOverloaded is Anthropic's 529 overloaded_error — a transient
// "server overloaded" signal with no stdlib http.Status* constant.
const statusOverloaded = 529

func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, // 408 - Request timeout
		http.StatusTooManyRequests,     // 429 - Rate limited
		http.StatusInternalServerError, // 500 - Internal server error
		http.StatusBadGateway,          // 502 - Bad gateway
		http.StatusServiceUnavailable,  // 503 - Service unavailable
		http.StatusGatewayTimeout,      // 504 - Gateway timeout
		statusOverloaded:               // 529 - Anthropic overloaded_error
		return true
	default:
		return false
	}
}

type retryableHTTPClient struct {
	Client HTTPClient
	Config retryConfig
	// OnRetry, if non-nil, is invoked on the cloned request before a retry
	// attempt (attempt >= 1). retryErr describes the previous failed attempt
	// and previousRequest is the exact request that produced it.
	OnRetry func(reqClone *http.Request, attempt int, retryErr *retryableError, previousRequest *http.Request)
}

func newRetryableHTTPClient(client HTTPClient, config retryConfig) *retryableHTTPClient {
	if client == nil {
		// Default to no timeout - let context timeouts handle this
		client = &http.Client{}
	}

	return &retryableHTTPClient{
		Client: client,
		Config: config,
	}
}

// Do executes an HTTP request with retry logic
func (r *retryableHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error
	var lastRetryErr *retryableError
	var previousRequest *http.Request

	for attempt := 0; attempt <= r.Config.MaxRetries; attempt++ {
		// Clone request for retry attempts
		reqClone := req.Clone(req.Context())
		if req.Body != nil {
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("recreate request body: %w", err)
				}
				reqClone.Body = body
			} else if attempt > 0 {
				return nil, fmt.Errorf("request body is not replayable for retry attempt %d", attempt+1)
			}
		}

		// Rotate credentials on retries (e.g. next API key after a 429).
		if attempt > 0 && r.OnRetry != nil {
			r.OnRetry(reqClone, attempt, lastRetryErr, previousRequest)
		}

		// Execute request
		resp, err := r.Client.Do(reqClone)
		previousRequest = reqClone

		// If no error and successful status, return immediately
		if err == nil && !isRetryableStatusCode(resp.StatusCode) {
			return resp, nil
		}

		// If MaxRetries is 0, return immediately regardless of status
		// This allows the caller to handle HTTP error responses directly
		if r.Config.MaxRetries == 0 {
			return resp, err
		}

		// Handle different error scenarios
		if err != nil {
			lastRetryErr = &retryableError{
				Err:         err,
				ShouldRetry: true, // Network errors are generally retryable
			}
		} else {
			// HTTP error response
			retryAfter := types.ParseRetryAfterHeader(resp.Header, time.Now())
			// Capture a bounded copy of the error body before closing it, so the
			// provider's structured error (e.g. insufficient_quota) survives to the
			// final surfaced error even after retries are exhausted.
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
			lastRetryErr = &retryableError{
				Err:         fmt.Errorf("HTTP %d", resp.StatusCode),
				StatusCode:  resp.StatusCode,
				ShouldRetry: isRetryableStatusCode(resp.StatusCode),
				RetryAfter:  retryAfter,
				Body:        body,
			}
			if err := resp.Body.Close(); err != nil {
				log.Printf("warning: failed to close response body: %v", err)
			}
		}
		lastErr = lastRetryErr

		// Don't retry if this is not a retryable error
		if lastRetryErr != nil && !lastRetryErr.ShouldRetry {
			return nil, lastErr
		}

		// Don't wait after the last attempt
		if attempt == r.Config.MaxRetries {
			break
		}

		// Calculate delay for next attempt
		delay := r.calculateDelay(attempt, lastRetryErr.RetryAfter)

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
func (r *retryableHTTPClient) calculateDelay(attempt int, retryAfter time.Duration) time.Duration {
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
		// Add ±20% jitter using cryptographically secure randomness
		jitter := delay * 0.2 * secureRandomFloat()
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

// secureRandomFloat returns a cryptographically secure random float between -1 and 1
func secureRandomFloat() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	// Keep the top 53 bits so conversion to float64 is exact, then scale the
	// result from [0, 1) to [-1, 1).
	const denominator = float64(uint64(1) << 53)
	n := uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
	return (float64(n>>11)/denominator)*2 - 1
}

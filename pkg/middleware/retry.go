package middleware

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// jitterRand returns a value in [0, 1) using math/rand's global locked source.
// Go 1.20+ auto-seeds the global source; concurrent callers are safe.
func jitterRand() float64 {
	return rand.Float64() // #nosec G404 - non-cryptographic jitter
}

// RetryConfig holds configuration for retry middleware
type RetryConfig struct {
	MaxRetries      int              // Maximum number of retry attempts
	InitialDelay    time.Duration    // Initial delay between retries
	MaxDelay        time.Duration    // Maximum delay between retries
	BackoffMultiple float64          // Multiplier for exponential backoff
	Jitter          bool             // Add random jitter to prevent thundering herd
	RetryableFunc   func(error) bool // Custom function to determine if error is retryable
}

// DefaultRetryConfig returns sensible defaults for retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          true,
	}
}

// RetryMiddleware creates a middleware that retries failed requests
func RetryMiddleware(config RetryConfig) Middleware {
	return func(handler Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			var lastErr error

			for attempt := 0; attempt <= config.MaxRetries; attempt++ {
				result, err := handler(ctx, req)
				if err == nil {
					return result, nil
				}

				lastErr = err

				// Check if error is retryable using custom function
				if config.RetryableFunc != nil && !config.RetryableFunc(err) {
					return nil, wrapIfNotWormholeError("retry", err)
				}

				// Don't wait after the last attempt
				if attempt == config.MaxRetries {
					break
				}

				// Calculate delay with exponential backoff
				delay := calculateRetryDelay(config, attempt)

				// Wait before retry, respecting context cancellation.
				// Use NewTimer + Stop() to avoid leaked timers on early cancel.
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, wrapMiddlewareError("retry", "execute", ctx.Err())
				case <-timer.C:
					// Continue to next attempt
				}
			}

			return nil, wrapIfNotWormholeError("retry", lastErr)
		}
	}
}

// calculateRetryDelay computes the delay before the next retry attempt
func calculateRetryDelay(config RetryConfig, attempt int) time.Duration {
	// Calculate exponential backoff
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffMultiple, float64(attempt))

	// Apply jitter to prevent thundering herd
	if config.Jitter {
		// Add ±25% jitter using properly seeded random generator
		jitterFactor := 0.75 + jitterRand()*0.5 // Random between 0.75 and 1.25
		delay *= jitterFactor
	}

	// Cap at maximum delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// Ensure minimum delay
	if delay < float64(config.InitialDelay) {
		delay = float64(config.InitialDelay)
	}

	return time.Duration(delay)
}

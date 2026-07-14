package middleware

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
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
	RetryableFunc   func(error) bool // Custom function to determine if error is retryable; nil falls back to DefaultRetryableFunc
}

// DefaultRetryConfig returns sensible defaults for retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          true,
		RetryableFunc:   DefaultRetryableFunc,
	}
}

// DefaultRetryableFunc classifies err as retryable using WormholeError.Retryable
// when err is a *types.WormholeError (e.g. an auth/400 error surfaced by the
// HTTP client layer), so RetryMiddleware stops retrying errors the provider
// has already told us are permanent instead of multiplying with the HTTP
// layer's own retries. Errors of any other type remain retryable, preserving
// the middleware's prior behavior for uncategorized errors.
func DefaultRetryableFunc(err error) bool {
	if werr, ok := types.AsWormholeError(err); ok {
		return werr.IsRetryable()
	}
	return true
}

// RetryMiddleware creates a middleware that retries failed requests
func RetryMiddleware(config RetryConfig) Middleware {
	retryable := config.RetryableFunc
	if retryable == nil {
		retryable = DefaultRetryableFunc
	}

	return func(handler Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			var lastErr error

			for attempt := 0; attempt <= config.MaxRetries; attempt++ {
				result, err := handler(ctx, req)
				if err == nil {
					return result, nil
				}

				lastErr = err

				// Check if error is retryable
				if !retryable(err) {
					return nil, wrapIfNotWormholeError("retry", err)
				}

				// Don't wait after the last attempt
				if attempt == config.MaxRetries {
					break
				}

				// Calculate delay with exponential backoff, honoring a
				// provider-supplied Retry-After when present since it is
				// authoritative over our own backoff estimate.
				delay := calculateRetryDelay(config, attempt)
				if werr, ok := types.AsWormholeError(err); ok && werr.RetryAfter > 0 {
					delay = capRetryDelay(werr.RetryAfter, config.MaxDelay)
				}

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

func capRetryDelay(delay, maxDelay time.Duration) time.Duration {
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
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

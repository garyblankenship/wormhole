package middleware

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	// seededRand is a properly seeded random generator for jitter calculations
	seededRand *rand.Rand
	randOnce   sync.Once
)

// getSeededRand returns a properly seeded random generator
func getSeededRand() *rand.Rand {
	randOnce.Do(func() {
		var seed int64
		if err := binary.Read(crand.Reader, binary.BigEndian, &seed); err != nil {
			// Fallback to time-based seed if crypto/rand fails
			seed = time.Now().UnixNano()
		}
		// Create seeded random source
		src := rand.NewSource(seed)
		seededRand = rand.New(src)
	})
	return seededRand
}

// RetryConfig holds configuration for retry middleware
type RetryConfig struct {
	MaxRetries      int                // Maximum number of retry attempts
	InitialDelay    time.Duration      // Initial delay between retries
	MaxDelay        time.Duration      // Maximum delay between retries
	BackoffMultiple float64            // Multiplier for exponential backoff
	Jitter          bool               // Add random jitter to prevent thundering herd
	RetryableFunc   func(error) bool   // Custom function to determine if error is retryable
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
					return nil, wrapIfNotWormholeError("retry", "execute", err)
				}

				// Don't wait after the last attempt
				if attempt == config.MaxRetries {
					break
				}

				// Calculate delay with exponential backoff
				delay := calculateRetryDelay(config, attempt)

				// Wait before retry, respecting context cancellation
				select {
				case <-ctx.Done():
					return nil, wrapMiddlewareError("retry", "execute", ctx.Err())
				case <-time.After(delay):
					// Continue to next attempt
				}
			}

			return nil, wrapIfNotWormholeError("retry", "execute", lastErr)
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
		jitterFactor := 0.75 + getSeededRand().Float64()*0.5 // Random between 0.75 and 1.25
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

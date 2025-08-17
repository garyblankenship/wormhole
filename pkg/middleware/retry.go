package middleware

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	Multiplier    float64
	Jitter        bool
	RetryableFunc func(error) bool
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableFunc: func(err error) bool {
			// By default, retry on any error
			return err != nil
		},
	}
}

// Retry executes a function with retry logic
func Retry(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context before attempting
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute function
		err := fn()

		// Success
		if err == nil {
			return nil
		}

		// Check if error is retryable
		if config.RetryableFunc != nil && !config.RetryableFunc(err) {
			return err
		}

		lastErr = err

		// Don't delay after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff
		if attempt > 0 {
			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		// Add jitter if configured
		actualDelay := delay
		if config.Jitter {
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.3)
			actualDelay = delay + jitter
		}

		// Wait before next attempt
		select {
		case <-time.After(actualDelay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// RetryMiddleware implements retry with exponential backoff.
//
// Example usage:
//
//	config := middleware.DefaultRetryConfig() // Recommended defaults
//	middleware.RetryMiddleware(config)
//
// Custom configuration:
//
//	config := middleware.RetryConfig{
//	    MaxRetries: 5,
//	    InitialDelay: 2 * time.Second,
//	    MaxDelay: 30 * time.Second,
//	    Multiplier: 2.0,
//	    Jitter: true,
//	}
func RetryMiddleware(config RetryConfig) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			var result interface{}
			err := Retry(ctx, config, func() error {
				var retryErr error
				result, retryErr = next(ctx, req)
				return retryErr
			})
			return result, err
		}
	}
}

// CircuitBreakerRetryConfig combines circuit breaker with retry
type CircuitBreakerRetryConfig struct {
	RetryConfig
	CircuitThreshold int
	CircuitTimeout   time.Duration
}

// CircuitBreakerRetryMiddleware combines circuit breaking with retry logic
func CircuitBreakerRetryMiddleware(config CircuitBreakerRetryConfig) Middleware {
	breaker := NewCircuitBreaker(config.CircuitThreshold, config.CircuitTimeout)

	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			var result interface{}

			err := Retry(ctx, config.RetryConfig, func() error {
				res, execErr := breaker.Execute(ctx, func() (interface{}, error) {
					var err error
					result, err = next(ctx, req)
					return result, err
				})
				result = res
				return execErr
			})

			return result, err
		}
	}
}

// AdaptiveRetryConfig extends RetryConfig with adaptive behavior
type AdaptiveRetryConfig struct {
	RetryConfig
	SuccessThreshold int // Number of successes to reduce delay
	FailureThreshold int // Number of failures to increase delay
}

// AdaptiveRetry tracks success/failure patterns and adjusts retry behavior
type AdaptiveRetry struct {
	config          AdaptiveRetryConfig
	consecutiveOk   int
	consecutiveFail int
	currentDelay    time.Duration
}

// NewAdaptiveRetry creates a new adaptive retry handler
func NewAdaptiveRetry(config AdaptiveRetryConfig) *AdaptiveRetry {
	return &AdaptiveRetry{
		config:       config,
		currentDelay: config.InitialDelay,
	}
}

// Execute runs a function with adaptive retry logic
func (ar *AdaptiveRetry) Execute(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= ar.config.MaxRetries; attempt++ {
		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute function
		err := fn()

		if err == nil {
			// Success - adjust metrics
			ar.consecutiveOk++
			ar.consecutiveFail = 0

			// Reduce delay if consistently successful
			if ar.consecutiveOk >= ar.config.SuccessThreshold {
				ar.currentDelay = time.Duration(float64(ar.currentDelay) / ar.config.Multiplier)
				if ar.currentDelay < ar.config.InitialDelay {
					ar.currentDelay = ar.config.InitialDelay
				}
			}

			return nil
		}

		// Failure - adjust metrics
		ar.consecutiveFail++
		ar.consecutiveOk = 0

		// Check if error is retryable
		if ar.config.RetryableFunc != nil && !ar.config.RetryableFunc(err) {
			return err
		}

		lastErr = err

		// Increase delay if consistently failing
		if ar.consecutiveFail >= ar.config.FailureThreshold {
			ar.currentDelay = time.Duration(float64(ar.currentDelay) * ar.config.Multiplier)
			if ar.currentDelay > ar.config.MaxDelay {
				ar.currentDelay = ar.config.MaxDelay
			}
		}

		// Don't delay after last attempt
		if attempt == ar.config.MaxRetries {
			break
		}

		// Apply jitter if configured
		actualDelay := ar.currentDelay
		if ar.config.Jitter {
			jitter := time.Duration(rand.Float64() * float64(ar.currentDelay) * 0.3)
			actualDelay = ar.currentDelay + jitter
		}

		// Wait before next attempt
		select {
		case <-time.After(actualDelay):
			// Continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// AdaptiveRetryMiddleware creates middleware with adaptive retry behavior
func AdaptiveRetryMiddleware(config AdaptiveRetryConfig) Middleware {
	retry := NewAdaptiveRetry(config)

	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			var result interface{}
			err := retry.Execute(ctx, func() error {
				var retryErr error
				result, retryErr = next(ctx, req)
				return retryErr
			})
			return result, err
		}
	}
}

// ExponentialBackoff calculates exponential backoff delay
func ExponentialBackoff(attempt int, base time.Duration, max time.Duration) time.Duration {
	delay := base * time.Duration(math.Pow(2, float64(attempt)))
	if delay > max {
		return max
	}
	return delay
}

// LinearBackoff calculates linear backoff delay
func LinearBackoff(attempt int, base time.Duration, max time.Duration) time.Duration {
	delay := base * time.Duration(attempt+1)
	if delay > max {
		return max
	}
	return delay
}

// FibonacciBackoff calculates Fibonacci sequence backoff delay
func FibonacciBackoff(attempt int, base time.Duration, max time.Duration) time.Duration {
	fib := fibonacci(attempt + 1)
	delay := base * time.Duration(fib)
	if delay > max {
		return max
	}
	return delay
}

func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

// RetryableError marks an error as retryable
type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return e.Err.Error()
}

// IsRetryable checks if an error is marked as retryable
func IsRetryable(err error) bool {
	var retryable RetryableError
	return errors.As(err, &retryable)
}

package wormhole

import (
	"context"
	"sync"
	"time"
)

// ConcurrencyLimiter implements a simple semaphore for limiting concurrent operations
type ConcurrencyLimiter struct {
	sem chan struct{}
}

// NewConcurrencyLimiter creates a new limiter with the given capacity
func NewConcurrencyLimiter(capacity int) *ConcurrencyLimiter {
	if capacity <= 0 {
		// Unlimited capacity - use buffered channel that never blocks
		capacity = 1024 // Large buffer to avoid blocking
	}
	return &ConcurrencyLimiter{
		sem: make(chan struct{}, capacity),
	}
}

// Acquire attempts to acquire a slot in the limiter
// Returns true if acquired, false if context expired or cancelled
func (l *ConcurrencyLimiter) Acquire(ctx context.Context) bool {
	select {
	case l.sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Release releases a slot in the limiter
func (l *ConcurrencyLimiter) Release() {
	select {
	case <-l.sem:
		// Successfully removed from channel
	default:
		// Channel was empty - shouldn't happen in correct usage
	}
}

// Capacity returns the current capacity of the limiter
func (l *ConcurrencyLimiter) Capacity() int {
	return cap(l.sem)
}

// SimpleCircuitBreaker implements a basic circuit breaker pattern
type SimpleCircuitBreaker struct {
	mu               sync.RWMutex
	failureCount     int
	threshold        int
	resetTimeout     time.Duration
	lastFailureTime  time.Time
	state            circuitBreakerState
	tripExpiry       time.Time
	halfOpenCalls    int
	maxHalfOpenCalls int
}

// circuitBreakerState represents the state of the circuit breaker
type circuitBreakerState int

const (
	stateClosed circuitBreakerState = iota
	stateOpen
	stateHalfOpen
)

// NewSimpleCircuitBreaker creates a new circuit breaker
func NewSimpleCircuitBreaker(threshold int, resetTimeout time.Duration) *SimpleCircuitBreaker {
	return &SimpleCircuitBreaker{
		threshold:        threshold,
		resetTimeout:     resetTimeout,
		state:            stateClosed,
		maxHalfOpenCalls: 1, // Allow 1 test call in half-open state by default
	}
}

// RecordSuccess resets the failure count and clears the tripped state
func (cb *SimpleCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	// Success in half-open state transitions to closed
	if cb.state == stateHalfOpen {
		cb.state = stateClosed
		cb.halfOpenCalls = 0
	}
}

// RecordFailure records a failure and trips the breaker if threshold is reached
func (cb *SimpleCircuitBreaker) RecordFailure() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.failureCount++
	cb.lastFailureTime = now

	// Check state transitions
	switch cb.state {
	case stateClosed:
		if cb.failureCount >= cb.threshold {
			cb.state = stateOpen
			cb.tripExpiry = now.Add(cb.resetTimeout)
		}
	case stateHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.state = stateOpen
		cb.tripExpiry = now.Add(cb.resetTimeout)
		cb.halfOpenCalls = 0
	case stateOpen:
		// Check if open breaker has expired (transition to half-open)
		if now.After(cb.tripExpiry) {
			cb.state = stateHalfOpen
			cb.halfOpenCalls = 0
			cb.failureCount = 0
		}
	}

	// Return true if circuit is open (tripped)
	return cb.state == stateOpen
}

// IsTripped returns true if the circuit breaker is currently tripped (open)
func (cb *SimpleCircuitBreaker) IsTripped() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Check if open breaker has expired (transition to half-open)
	if cb.state == stateOpen && now.After(cb.tripExpiry) {
		cb.state = stateHalfOpen
		cb.halfOpenCalls = 0
		cb.failureCount = 0
	}

	// In half-open state, check if we've exceeded test call limit
	if cb.state == stateHalfOpen && cb.halfOpenCalls >= cb.maxHalfOpenCalls {
		// Exceeded half-open call limit, treat as open
		return true
	}

	// Increment half-open call counter when checking (call will proceed)
	if cb.state == stateHalfOpen {
		cb.halfOpenCalls++
	}

	return cb.state == stateOpen
}

// RetryExecutor handles retry logic for tool execution
type RetryExecutor struct {
	maxRetries int
}

// NewRetryExecutor creates a new retry executor
func NewRetryExecutor(maxRetries int) *RetryExecutor {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &RetryExecutor{
		maxRetries: maxRetries,
	}
}

// ExecuteWithRetry executes a function with retry logic
func (r *RetryExecutor) ExecuteWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := fn(ctx); err != nil {
			lastErr = err

			// Check if we should retry
			if attempt == r.maxRetries {
				return lastErr
			}

			// Wait before retry with exponential backoff
			// Safe shift to prevent integer overflow (G115)
			shift := attempt
			if shift < 0 {
				shift = 0
			}
			if shift >= 64 { // Maximum safe shift for 64-bit integers
				shift = 63
			}
			waitTime := time.Duration(100*(1<<uint(shift))) * time.Millisecond // #nosec G115 - shift bounded 0-63
			select {
			case <-time.After(waitTime):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	}

	return lastErr
}

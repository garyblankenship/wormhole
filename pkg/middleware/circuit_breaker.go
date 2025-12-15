package middleware

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// StateClosed allows requests through
	StateClosed CircuitState = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests for testing
	StateHalfOpen
)

var (
	// ErrCircuitOpen is returned when circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            CircuitState
	failures         int
	successes        int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastFailureTime  time.Time
	halfOpenCalls    int
	maxHalfOpenCalls int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: failureThreshold / 2, // Need half successful calls to close
		timeout:          timeout,
		maxHalfOpenCalls: 3, // Allow 3 test calls in half-open state
	}
}

// Execute wraps a function call with circuit breaker logic
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() (any, error)) (any, error) {
	cb.mu.Lock()

	// Check if we should transition from open to half-open
	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = StateHalfOpen
			cb.halfOpenCalls = 0
			cb.successes = 0
		} else {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
	}

	// Check if we've exceeded half-open call limit
	if cb.state == StateHalfOpen {
		if cb.halfOpenCalls >= cb.maxHalfOpenCalls {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
		cb.halfOpenCalls++
	}

	cb.mu.Unlock()

	// Execute the function
	result, err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		return cb.handleError(result, err)
	}

	return cb.handleSuccess(result), nil
}

func (cb *CircuitBreaker) handleError(result any, err error) (any, error) {
	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.failureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.state = StateOpen
		cb.failures = cb.failureThreshold
	}

	return result, err
}

func (cb *CircuitBreaker) handleSuccess(result any) any {
	cb.failures = 0

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = StateClosed
			cb.successes = 0
		}
	}

	return result
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// CircuitBreakerMiddleware creates a middleware with circuit breaker protection
func CircuitBreakerMiddleware(threshold int, timeout time.Duration) Middleware {
	breaker := NewCircuitBreaker(threshold, timeout)

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			return breaker.Execute(ctx, func() (any, error) {
				return next(ctx, req)
			})
		}
	}
}

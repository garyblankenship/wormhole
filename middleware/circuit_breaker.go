package middleware

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
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
	ErrCircuitOpen = types.NewWormholeError(types.ErrorCodeMiddleware, "circuit breaker is open", true)
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
	halfOpenCalls    atomic.Int32 // Atomic for CAS-based admission control
	maxHalfOpenCalls int32        // int32 for atomic comparison
}

type circuitBreakerRegistry struct {
	mu               sync.RWMutex
	breakers         map[string]*CircuitBreaker
	failureThreshold int
	timeout          time.Duration
}

func newCircuitBreakerRegistry(failureThreshold int, timeout time.Duration) *circuitBreakerRegistry {
	return &circuitBreakerRegistry{
		breakers:         make(map[string]*CircuitBreaker),
		failureThreshold: failureThreshold,
		timeout:          timeout,
	}
}

func (r *circuitBreakerRegistry) breaker(provider, operation string) *CircuitBreaker {
	key := provider + "\x00" + operation
	r.mu.RLock()
	breaker := r.breakers[key]
	r.mu.RUnlock()
	if breaker != nil {
		return breaker
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if breaker = r.breakers[key]; breaker == nil {
		breaker = NewCircuitBreaker(r.failureThreshold, r.timeout)
		r.breakers[key] = breaker
	}
	return breaker
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold int, timeout time.Duration) *CircuitBreaker {
	// maxHalfOpen is the probe budget admitted per half-open cycle. successThreshold
	// must never exceed it: if it does, the breaker can admit fewer probes than it
	// needs to close, so once the provider recovers all probes succeed but the count
	// never reaches successThreshold and the breaker wedges half-open forever.
	const maxHalfOpen = 3
	successThreshold := failureThreshold / 2
	if successThreshold > maxHalfOpen {
		successThreshold = maxHalfOpen
	}
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		maxHalfOpenCalls: maxHalfOpen,
	}
}

// Execute wraps a function call with circuit breaker logic
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() (any, error)) (any, error) {
	if err := cb.admit(); err != nil {
		return nil, err
	}

	result, err := fn()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			cb.recordCancellation()
			return result, err
		}
		return result, cb.recordError(err)
	}
	cb.recordSuccess()
	return result, nil
}

// recordCancellation releases a half-open probe without treating a caller-aborted
// request as evidence that the provider is unhealthy.
func (cb *CircuitBreaker) recordCancellation() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == StateHalfOpen && cb.halfOpenCalls.Load() > 0 {
		cb.halfOpenCalls.Add(-1)
	}
}

func (cb *CircuitBreaker) admit() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if we should transition from open to half-open
	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = StateHalfOpen
			cb.halfOpenCalls.Store(0)
			cb.successes = 0
		} else {
			return wrapMiddlewareError("circuit_breaker", "execute", ErrCircuitOpen)
		}
	}

	// Half-open admission using atomic CAS to prevent race conditions
	if cb.state == StateHalfOpen {
		// CAS loop ensures exactly maxHalfOpenCalls requests pass
		for {
			current := cb.halfOpenCalls.Load()
			if current >= cb.maxHalfOpenCalls {
				return wrapMiddlewareError("circuit_breaker", "execute", ErrCircuitOpen)
			}
			// Atomic increment - only proceeds if no concurrent modification
			if cb.halfOpenCalls.CompareAndSwap(current, current+1) {
				break
			}
			// CAS failed (another goroutine incremented), retry
		}
	}
	return nil
}

func (cb *CircuitBreaker) recordError(err error) error {
	if classifyCircuitError(err) == circuitErrorNeutral {
		cb.recordCancellation()
		return err
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	_, wrapped := cb.handleError(nil, wrapIfNotWormholeError("circuit_breaker", err))
	return wrapped
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.handleSuccess(nil)
}

func (cb *CircuitBreaker) handleError(result any, err error) (any, error) {
	cb.failures += circuitFailureWeight(err, cb.failureThreshold)
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
		cb.halfOpenCalls.Store(0) // Reset for next half-open cycle
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
			cb.halfOpenCalls.Store(0) // Reset for next half-open cycle
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

// Close is a no-op for circuit breaker (no background resources)
func (cb *CircuitBreaker) Close() error {
	return nil
}

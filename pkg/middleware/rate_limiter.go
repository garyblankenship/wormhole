package middleware

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu           sync.Mutex
	rate         int           // tokens per second
	capacity     int           // max tokens in bucket
	tokens       float64       // current tokens
	lastRefill   time.Time     // last refill time
	requestQueue chan struct{} // queue for waiting requests
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	capacity := requestsPerSecond * 2 // Allow burst of 2x rate

	return &RateLimiter{
		rate:         requestsPerSecond,
		capacity:     capacity,
		tokens:       float64(capacity),
		lastRefill:   time.Now(),
		requestQueue: make(chan struct{}, capacity),
	}
}

// Wait blocks until a token is available or context is canceled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if err := rl.TryAcquire(); err == nil {
		return nil
	}

	// Add to queue
	select {
	case rl.requestQueue <- struct{}{}:
		// Successfully queued
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrRateLimitExceeded
	}

	// Wait for token to become available
	ticker := time.NewTicker(time.Second / time.Duration(rl.rate))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Remove from queue
			select {
			case <-rl.requestQueue:
			default:
			}
			return ctx.Err()
		case <-ticker.C:
			if err := rl.TryAcquire(); err == nil {
				// Remove from queue
				<-rl.requestQueue
				return nil
			}
		}
	}
}

// TryAcquire attempts to acquire a token without blocking
func (rl *RateLimiter) TryAcquire() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= 1 {
		rl.tokens--
		return nil
	}

	return ErrRateLimitExceeded
}

// refill adds tokens based on elapsed time
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Calculate tokens to add
	tokensToAdd := elapsed.Seconds() * float64(rl.rate)

	// Add tokens up to capacity
	rl.tokens += tokensToAdd
	if rl.tokens > float64(rl.capacity) {
		rl.tokens = float64(rl.capacity)
	}

	rl.lastRefill = now
}

// GetAvailableTokens returns the current number of available tokens
func (rl *RateLimiter) GetAvailableTokens() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()
	return int(rl.tokens)
}

// HealthMetrics represents provider health metrics for adaptive rate limiting
type HealthMetrics struct {
	CircuitState      CircuitState     // Circuit breaker state
	Healthy           bool             // Health checker status
	ErrorRate         float64          // Error rate (0.0 to 1.0)
	ResponseTime      time.Duration    // Response time from health checker
	ConsecutiveFails  int              // Consecutive failures
	LastCheck         time.Time        // Last health check time
}

// AdaptiveRateLimiter adjusts rate based on response times and health metrics
type AdaptiveRateLimiter struct {
	*RateLimiter
	mu              sync.RWMutex
	minRate         int
	maxRate         int
	targetLatency   time.Duration
	latencyWindow   []time.Duration
	windowSize      int
	totalLatency    time.Duration // Running total of latencies in window
	windowCount     int           // Number of latencies in window
	adjustInterval  time.Duration
	lastAdjustment  time.Time
	healthMetrics   *HealthMetrics    // Optional health metrics
	useHealthAware  bool              // Whether to use health-aware adjustment
}

// NewAdaptiveRateLimiter creates a rate limiter that adjusts based on latency
func NewAdaptiveRateLimiter(initialRate, minRate, maxRate int, targetLatency time.Duration) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		RateLimiter:    NewRateLimiter(initialRate),
		minRate:        minRate,
		maxRate:        maxRate,
		targetLatency:  targetLatency,
		latencyWindow:  make([]time.Duration, 0, 100),
		windowSize:     100,
		adjustInterval: 10 * time.Second,
		lastAdjustment: time.Now(),
		healthMetrics:  nil,
		useHealthAware: false,
	}
}

// NewHealthAwareAdaptiveRateLimiter creates a rate limiter that adjusts based on both latency and health metrics
func NewHealthAwareAdaptiveRateLimiter(initialRate, minRate, maxRate int, targetLatency time.Duration) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		RateLimiter:    NewRateLimiter(initialRate),
		minRate:        minRate,
		maxRate:        maxRate,
		targetLatency:  targetLatency,
		latencyWindow:  make([]time.Duration, 0, 100),
		windowSize:     100,
		adjustInterval: 10 * time.Second,
		lastAdjustment: time.Now(),
		healthMetrics:  &HealthMetrics{},
		useHealthAware: true,
	}
}

// RecordLatency records a request latency and adjusts rate if needed
func (arl *AdaptiveRateLimiter) RecordLatency(latency time.Duration) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	// Add to window
	arl.latencyWindow = append(arl.latencyWindow, latency)
	arl.totalLatency += latency
	arl.windowCount++

	// If window exceeds size, remove oldest element
	if len(arl.latencyWindow) > arl.windowSize {
		oldest := arl.latencyWindow[0]
		arl.latencyWindow = arl.latencyWindow[1:]
		arl.totalLatency -= oldest
		arl.windowCount--
	}

	// Check if we should adjust
	if time.Since(arl.lastAdjustment) < arl.adjustInterval {
		return
	}

	// Calculate average latency
	if arl.windowCount < arl.windowSize/2 {
		return // Not enough data
	}

	avgLatency := arl.totalLatency / time.Duration(arl.windowCount)

	// Adjust rate based on latency and health metrics
	arl.adjustRate(avgLatency)

	arl.lastAdjustment = time.Now()
}

// RecordHealthMetrics updates the health metrics for the adaptive rate limiter
func (arl *AdaptiveRateLimiter) RecordHealthMetrics(metrics *HealthMetrics) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	if arl.healthMetrics != nil {
		arl.healthMetrics = metrics
	}
}

// adjustRate adjusts the rate based on latency and health metrics
func (arl *AdaptiveRateLimiter) adjustRate(avgLatency time.Duration) {
	// Start with base adjustment based on latency
	adjustmentFactor := 1.0

	// Calculate latency adjustment
	if avgLatency > arl.targetLatency*120/100 { // 20% above target
		adjustmentFactor *= 0.9 // Decrease by 10%
	} else if avgLatency < arl.targetLatency*80/100 { // 20% below target
		adjustmentFactor *= 1.1 // Increase by 10%
	}

	// Apply health metrics if using health-aware mode
	if arl.useHealthAware && arl.healthMetrics != nil {
		healthAdjustment := arl.calculateHealthAdjustment()
		adjustmentFactor *= healthAdjustment
	}

	// Calculate new rate with bounds
	newRate := int(float64(arl.rate) * adjustmentFactor)

	// Ensure rate stays within bounds
	if newRate < arl.minRate {
		newRate = arl.minRate
	} else if newRate > arl.maxRate {
		newRate = arl.maxRate
	}

	arl.rate = newRate
}

// calculateHealthAdjustment calculates the rate adjustment factor based on health metrics
func (arl *AdaptiveRateLimiter) calculateHealthAdjustment() float64 {
	if arl.healthMetrics == nil {
		return 1.0
	}

	healthScore := arl.calculateHealthScore()

	// Map health score to adjustment factor:
	// 0.0-0.3 (critical) -> 0.5 (cut rate by 50%)
	// 0.3-0.6 (poor) -> 0.75 (cut rate by 25%)
	// 0.6-0.8 (fair) -> 0.9 (cut rate by 10%)
	// 0.8-1.0 (good) -> 1.0 (no adjustment)

	if healthScore < 0.3 {
		return 0.5
	} else if healthScore < 0.6 {
		return 0.75
	} else if healthScore < 0.8 {
		return 0.9
	}
	return 1.0
}

// calculateHealthScore calculates a composite health score from all metrics
func (arl *AdaptiveRateLimiter) calculateHealthScore() float64 {
	if arl.healthMetrics == nil {
		return 1.0
	}

	score := 0.0
	totalWeight := 0

	// Circuit breaker state (weight: 4)
	circuitWeight := 4
	switch arl.healthMetrics.CircuitState {
	case StateClosed:
		score += 1.0 * float64(circuitWeight)
	case StateHalfOpen:
		score += 0.5 * float64(circuitWeight) // Half score for half-open
	case StateOpen:
		score += 0.0 * float64(circuitWeight)
	}
	totalWeight += circuitWeight

	// Health checker status (weight: 3)
	healthWeight := 3
	if arl.healthMetrics.Healthy {
		score += 1.0 * float64(healthWeight)
	}
	totalWeight += healthWeight

	// Error rate (weight: 2)
	errorWeight := 2
	errorScore := 1.0 - arl.healthMetrics.ErrorRate
	if errorScore < 0 {
		errorScore = 0
	}
	score += errorScore * float64(errorWeight)
	totalWeight += errorWeight

	// Consecutive failures (weight: 1)
	failWeight := 1
	failScore := 1.0
	if arl.healthMetrics.ConsecutiveFails > 0 {
		// Exponential decay: 1 fail = 0.5, 2 fails = 0.25, 3 fails = 0.125
		failScore = 1.0 / math.Pow(2.0, float64(arl.healthMetrics.ConsecutiveFails))
	}
	score += failScore * float64(failWeight)
	totalWeight += failWeight

	// Normalize score to 0-1 range
	if totalWeight > 0 {
		return score / float64(totalWeight)
	}
	return 1.0
}

// RateLimitMiddleware creates a middleware with rate limiting
func RateLimitMiddleware(requestsPerSecond int) Middleware {
	limiter := NewRateLimiter(requestsPerSecond)

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			if err := limiter.Wait(ctx); err != nil {
				if err == ErrRateLimitExceeded {
					return nil, wrapMiddlewareError("rate_limiter", "wait", err)
				}
				return nil, wrapMiddlewareError("rate_limiter", "wait", err)
			}
			resp, err := next(ctx, req)
			return resp, wrapIfNotWormholeError("rate_limiter", "execute", err)
		}
	}
}

// AdaptiveRateLimitMiddleware creates a middleware with adaptive rate limiting
func AdaptiveRateLimitMiddleware(initialRate, minRate, maxRate int, targetLatency time.Duration) Middleware {
	limiter := NewAdaptiveRateLimiter(initialRate, minRate, maxRate, targetLatency)

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			if err := limiter.Wait(ctx); err != nil {
				if err == ErrRateLimitExceeded {
					return nil, wrapMiddlewareError("adaptive_rate_limiter", "wait", err)
				}
				return nil, wrapMiddlewareError("adaptive_rate_limiter", "wait", err)
			}

			resp, err := next(ctx, req)

			// Record latency for adaptation
			limiter.RecordLatency(time.Since(start))

			return resp, wrapIfNotWormholeError("adaptive_rate_limiter", "execute", err)
		}
	}
}

// HealthAwareAdaptiveRateLimitMiddleware creates a middleware with health-aware adaptive rate limiting
func HealthAwareAdaptiveRateLimitMiddleware(initialRate, minRate, maxRate int, targetLatency time.Duration, providerName string, checker *HealthChecker, breaker *CircuitBreaker) Middleware {
	limiter := NewHealthAwareAdaptiveRateLimiter(initialRate, minRate, maxRate, targetLatency)

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			if err := limiter.Wait(ctx); err != nil {
				if err == ErrRateLimitExceeded {
					return nil, wrapMiddlewareError("health_aware_adaptive_rate_limiter", "wait", err)
				}
				return nil, wrapMiddlewareError("health_aware_adaptive_rate_limiter", "wait", err)
			}

			resp, err := next(ctx, req)
			latency := time.Since(start)

			// Record latency
			limiter.RecordLatency(latency)

			// Update health metrics if health checker and circuit breaker are provided
			if checker != nil && breaker != nil {
				// Get health status
				healthStatus := checker.GetStatus(providerName)

				// Calculate error rate (simplified - in practice would track errors)
				errorRate := 0.0
				if err != nil {
					errorRate = 1.0 // Simple: error = 100% error rate for this request
				}

				// Create health metrics
				metrics := &HealthMetrics{
					CircuitState:     breaker.GetState(),
					Healthy:          healthStatus.Healthy,
					ErrorRate:        errorRate,
					ResponseTime:     healthStatus.ResponseTime,
					ConsecutiveFails: healthStatus.ConsecutiveFails,
					LastCheck:        healthStatus.LastCheck,
				}

				limiter.RecordHealthMetrics(metrics)
			}

			return resp, wrapIfNotWormholeError("health_aware_adaptive_rate_limiter", "execute", err)
		}
	}
}

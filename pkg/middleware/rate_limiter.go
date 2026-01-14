package middleware

import (
	"context"
	"errors"
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

// AdaptiveRateLimiter adjusts rate based on response times
type AdaptiveRateLimiter struct {
	*RateLimiter
	mu             sync.RWMutex
	minRate        int
	maxRate        int
	targetLatency  time.Duration
	latencyWindow  []time.Duration
	windowSize     int
	adjustInterval time.Duration
	lastAdjustment time.Time
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
	}
}

// RecordLatency records a request latency and adjusts rate if needed
func (arl *AdaptiveRateLimiter) RecordLatency(latency time.Duration) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	// Add to window
	arl.latencyWindow = append(arl.latencyWindow, latency)
	if len(arl.latencyWindow) > arl.windowSize {
		arl.latencyWindow = arl.latencyWindow[1:]
	}

	// Check if we should adjust
	if time.Since(arl.lastAdjustment) < arl.adjustInterval {
		return
	}

	// Calculate average latency
	if len(arl.latencyWindow) < arl.windowSize/2 {
		return // Not enough data
	}

	var totalLatency time.Duration
	for _, l := range arl.latencyWindow {
		totalLatency += l
	}
	avgLatency := totalLatency / time.Duration(len(arl.latencyWindow))

	// Adjust rate based on latency
	if avgLatency > arl.targetLatency*120/100 { // 20% above target
		// Decrease rate, ensuring it doesn't go below minimum
		newRate := max(arl.rate*9/10, arl.minRate)
		arl.rate = newRate
	} else if avgLatency < arl.targetLatency*80/100 { // 20% below target
		// Increase rate, ensuring it doesn't go above maximum
		newRate := min(arl.rate*11/10, arl.maxRate)
		arl.rate = newRate
	}

	arl.lastAdjustment = time.Now()
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

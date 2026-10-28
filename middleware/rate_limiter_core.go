package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded.
	ErrRateLimitExceeded = types.NewWormholeError(types.ErrorCodeRateLimit, "rate limit exceeded", true)
)

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	mu           sync.Mutex
	rate         atomic.Int64 // requests/second, shared by the acquire and refill paths
	capacity     int
	tokens       float64
	lastRefill   time.Time
	requestQueue chan struct{}
	closed       atomic.Bool
	done         chan struct{}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	if requestsPerSecond < 1 {
		requestsPerSecond = 1
	}
	capacity := requestsPerSecond * 2

	rl := &RateLimiter{
		capacity:     capacity,
		tokens:       float64(capacity),
		lastRefill:   time.Now(),
		requestQueue: make(chan struct{}, capacity),
		done:         make(chan struct{}),
	}
	rl.rate.Store(int64(requestsPerSecond))
	return rl
}

// Wait blocks until a token is available or context is canceled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if rl.closed.Load() {
		return ErrRateLimitExceeded
	}

	if err := rl.TryAcquire(); err == nil {
		return nil
	}

	select {
	case rl.requestQueue <- struct{}{}:
	case <-rl.done:
		return ErrRateLimitExceeded
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrRateLimitExceeded
	}
	defer func() { <-rl.requestQueue }()

	ticker := time.NewTicker(time.Second / time.Duration(rl.rate.Load()))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rl.done:
			return ErrRateLimitExceeded
		case <-ticker.C:
			if err := rl.TryAcquire(); err == nil {
				return nil
			}
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
func (rl *RateLimiter) TryAcquire() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.closed.Load() {
		return ErrRateLimitExceeded
	}
	rl.refill()
	if rl.tokens >= 1 {
		rl.tokens--
		return nil
	}

	return ErrRateLimitExceeded
}

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := elapsed.Seconds() * float64(rl.rate.Load())

	rl.tokens += tokensToAdd
	if rl.tokens > float64(rl.capacity) {
		rl.tokens = float64(rl.capacity)
	}

	rl.lastRefill = now
}

// GetAvailableTokens returns the current number of available tokens.
func (rl *RateLimiter) GetAvailableTokens() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()
	return int(rl.tokens)
}

// Close wakes queued waiters and prevents any later token acquisition.
// The request queue remains open because concurrent waiters may enqueue.
func (rl *RateLimiter) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.closed.CompareAndSwap(false, true) {
		close(rl.done)
	}
	return nil
}

package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded.
	ErrRateLimitExceeded = types.NewWormholeError(types.ErrorCodeRateLimit, "rate limit exceeded", true)
)

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	mu           sync.Mutex
	rate         int
	capacity     int
	tokens       float64
	lastRefill   time.Time
	requestQueue chan struct{}
	closed       atomic.Bool
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	capacity := requestsPerSecond * 2

	return &RateLimiter{
		rate:         requestsPerSecond,
		capacity:     capacity,
		tokens:       float64(capacity),
		lastRefill:   time.Now(),
		requestQueue: make(chan struct{}, capacity),
	}
}

// Wait blocks until a token is available or context is canceled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if rl.closed.Load() {
		return ErrRateLimitExceeded
	}

	if err := rl.TryAcquire(); err == nil {
		return nil
	}

	select {
	case rl.requestQueue <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrRateLimitExceeded
	}

	ticker := time.NewTicker(time.Second / time.Duration(rl.rate))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			select {
			case <-rl.requestQueue:
			default:
			}
			return ctx.Err()
		case <-ticker.C:
			if err := rl.TryAcquire(); err == nil {
				<-rl.requestQueue
				return nil
			}
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
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

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := elapsed.Seconds() * float64(rl.rate)

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

// Close marks the rate limiter as closed. It is safe to call concurrently
// with Wait and may be called more than once. After Close, Wait returns
// ErrRateLimitExceeded instead of blocking. The request queue channel is
// intentionally not closed: doing so would race with concurrent Wait
// writers (write-to-closed-channel panic); the channel is reclaimed by GC.
func (rl *RateLimiter) Close() error {
	rl.closed.CompareAndSwap(false, true)
	return nil
}

package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-07: expiration removes list nodes and replacement resets expiration.
func TestRemediationC407LRUExpirationReplacement(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cache := NewLRUCache(2)
		cache.Set("first", "old", time.Second)
		<-time.After(500 * time.Millisecond)
		cache.Set("first", "new", 2*time.Second)
		cache.Set("second", 2, time.Hour)
		<-time.After(500 * time.Millisecond)
		if value, ok := cache.Get("first"); !ok || value != "new" {
			t.Fatalf("replacement = %v/%v", value, ok)
		}
		cache.Set("third", 3, time.Hour)
		if _, ok := cache.Get("second"); ok {
			t.Fatal("Get did not promote replacement")
		}
		<-time.After(1500 * time.Millisecond)
		if _, ok := cache.Get("first"); ok {
			t.Fatal("entry survived expiration boundary")
		}
		if len(cache.cache) != 1 || cache.head.next != cache.tail.prev || cache.head.next.key != "third" {
			t.Fatal("expired node remained in list or map")
		}
		cache.Set("third", 4, 0)
		if _, ok := cache.Get("third"); ok || len(cache.cache) != 0 || cache.head.next != cache.tail {
			t.Fatal("zero TTL replacement remained")
		}
		cache.Set("negative", 5, -time.Second)
		if _, ok := cache.Get("negative"); ok {
			t.Fatal("negative TTL remained")
		}
	})
}

// C4-07: concurrent promotions and replacement retain list and map integrity.
func TestRemediationC407LRUConcurrentAccess(t *testing.T) {
	t.Parallel()
	cache := NewLRUCache(8)
	var workers sync.WaitGroup
	for i := range 8 {
		workers.Go(func() {
			key := string(rune('a' + i))
			for range 100 {
				cache.Set(key, i, time.Hour)
				cache.Get(key)
				cache.Delete(key)
			}
		})
	}
	workers.Wait()
	cache.Clear()
	if len(cache.cache) != 0 || cache.head.next != cache.tail || cache.tail.prev != cache.head {
		t.Fatal("cache topology invalid after concurrent use")
	}
}

// C4-08: the bounded queue limits admission without promising FIFO order.
func TestRemediationC408RateAdmissionBoundedNonFIFO(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		limiter := NewRateLimiter(1)
		for range limiter.capacity {
			if err := limiter.TryAcquire(); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		outcomes := make(chan error, limiter.capacity)
		for range limiter.capacity {
			go func() { outcomes <- limiter.Wait(ctx) }()
		}
		synctest.Wait()
		if len(limiter.requestQueue) != limiter.capacity {
			t.Fatalf("queued = %d", len(limiter.requestQueue))
		}
		if err := limiter.Wait(context.Background()); !errors.Is(err, ErrRateLimitExceeded) {
			t.Fatalf("overflow admission = %v", err)
		}
		// A newly available token can be taken without regard to queued waiters.
		limiter.mu.Lock()
		limiter.tokens = 1
		limiter.mu.Unlock()
		if err := limiter.TryAcquire(); err != nil {
			t.Fatalf("fresh caller admission = %v", err)
		}
		if len(limiter.requestQueue) != limiter.capacity {
			t.Fatal("direct admission consumed queued waiter")
		}
		cancel()
		synctest.Wait()
		for range limiter.capacity {
			if err := <-outcomes; !errors.Is(err, context.Canceled) {
				t.Fatalf("queued cancellation = %v", err)
			}
		}
		if len(limiter.requestQueue) != 0 {
			t.Fatal("canceled callers leaked admission slots")
		}
	})
}

// C4-09: an upstream deadline while the caller remains live is a failure.
func TestRemediationC409ProviderDeadlineTripsCircuit(t *testing.T) {
	t.Parallel()
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "stream"}[streaming], func(t *testing.T) {
			t.Parallel()
			circuit := NewTypedCircuitBreakerMiddleware(1, time.Hour)
			ctx := context.WithValue(context.Background(), CtxKeyProvider, "test")
			if streaming {
				handler := circuit.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
					stream := make(chan types.StreamChunk, 1)
					stream <- types.StreamChunk{Error: context.DeadlineExceeded}
					close(stream)
					return stream, nil
				})
				stream, err := handler(ctx, types.TextRequest{})
				if err != nil {
					t.Fatal(err)
				}
				for chunk := range stream {
					if !errors.Is(chunk.Error, context.DeadlineExceeded) {
						t.Fatalf("chunk = %+v", chunk)
					}
				}
				if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, ErrCircuitOpen) {
					t.Fatalf("provider deadline did not trip stream breaker: %v", err)
				}
			} else {
				handler := circuit.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
					return nil, context.DeadlineExceeded
				})
				if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("upstream deadline = %v", err)
				}
				if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, ErrCircuitOpen) {
					t.Fatalf("provider deadline did not trip breaker: %v", err)
				}
			}
			if ctx.Err() != nil {
				t.Fatalf("caller unexpectedly canceled: %v", ctx.Err())
			}
		})
	}
}

// C4-09: caller cancellation releases a half-open probe without a failure.
func TestRemediationC409CallerCancellationNeutral(t *testing.T) {
	t.Parallel()
	breaker := NewCircuitBreaker(1, time.Hour)
	breaker.state = StateHalfOpen
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := breaker.Execute(ctx, func() (any, error) { return nil, ctx.Err() }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if breaker.GetState() != StateHalfOpen || breaker.failures != 0 || breaker.halfOpenCalls.Load() != 0 {
		t.Fatal("caller cancellation altered breaker health or retained probe")
	}
	if _, err := breaker.Execute(context.Background(), func() (any, error) { return "ok", nil }); err != nil {
		t.Fatal(err)
	}
	if breaker.GetState() != StateClosed {
		t.Fatal("healthy probe did not close circuit")
	}
}

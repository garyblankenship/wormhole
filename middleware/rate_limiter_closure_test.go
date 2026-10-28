package middleware

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
)

func TestRateLimiterCloseWakesQueuedWaiters(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rl := NewRateLimiter(1)
		rl.tokens = 0
		results := make(chan error, 2)
		for range 2 {
			go func() { results <- rl.Wait(context.Background()) }()
		}
		synctest.Wait()
		if got := len(rl.requestQueue); got != 2 {
			t.Fatalf("queued waiters = %d", got)
		}
		for range 2 {
			if err := rl.Close(); err != nil {
				t.Fatal(err)
			}
		}
		for range 2 {
			if err := <-results; err != ErrRateLimitExceeded {
				t.Fatalf("closed waiter = %v", err)
			}
		}
		if got := len(rl.requestQueue); got != 0 {
			t.Fatalf("waiter membership retained: %d", got)
		}
		if err := rl.TryAcquire(); err != ErrRateLimitExceeded {
			t.Fatalf("acquire after close = %v", err)
		}
	})
}

func TestRateLimiterCloseAcquireContention(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(100)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			<-start
			_ = rl.TryAcquire()
		})
	}
	wg.Go(func() {
		<-start
		_ = rl.Close()
	})
	close(start)
	wg.Wait()
	remaining := rl.tokens
	for range 10 {
		if err := rl.TryAcquire(); err != ErrRateLimitExceeded {
			t.Fatalf("post-close acquisition = %v", err)
		}
	}
	if rl.tokens != remaining {
		t.Fatal("post-close acquisition changed token count")
	}
}

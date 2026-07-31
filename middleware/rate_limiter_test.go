package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestTypedRateLimitMiddleware(t *testing.T) {
	t.Parallel()
	mw := NewTypedRateLimitMiddleware(10)
	handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "response"}, nil
	})
	for range 5 {
		response, err := handler(context.Background(), testTextRequest("request"))
		if err != nil || response.Text != "response" {
			t.Fatalf("typed rate limited request = (%#v, %v)", response, err)
		}
	}
}

func TestTypedRateLimitMiddlewareNormalizesNonPositiveRate(t *testing.T) {
	t.Parallel()
	for _, rate := range []int{-1, 0} {
		mw := NewTypedRateLimitMiddleware(rate)
		handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
			return &types.TextResponse{Text: "response"}, nil
		})
		if _, err := handler(context.Background(), testTextRequest("request")); err != nil {
			t.Fatalf("rate %d: %v", rate, err)
		}
	}
}

func TestRateLimitCloseRace(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(5)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_ = rl.Wait(ctx)
		}()
	}
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		}()
	}
	wg.Wait()
	if err := rl.Wait(context.Background()); err != ErrRateLimitExceeded {
		t.Fatalf("Wait after Close = %v, want %v", err, ErrRateLimitExceeded)
	}
}

func TestConcurrentTypedRateLimiting(t *testing.T) {
	t.Parallel()
	mw := NewTypedRateLimitMiddleware(10)
	var calls atomic.Int64
	handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls.Add(1)
		return &types.TextResponse{Text: "response"}, nil
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := handler(context.Background(), testTextRequest("request")); err != nil {
				t.Errorf("typed rate limit handler: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 20 {
		t.Errorf("handler calls = %d, want 20", got)
	}
}

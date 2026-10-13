package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestRateLimiterRejectsCancelledBurstWithoutSpendingTokens(t *testing.T) {
	t.Parallel()
	for _, expired := range []bool{false, true} {
		name := "canceled"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			want := context.Canceled
			if expired {
				ctx, cancel = context.WithDeadline(t.Context(), time.Unix(0, 0))
				want = context.DeadlineExceeded
			}
			t.Cleanup(cancel)
			cancel()
			limiter := NewRateLimiter(1)
			t.Cleanup(func() { _ = limiter.Close() })
			before := limiter.GetAvailableTokens()
			if err := limiter.Wait(ctx); !errors.Is(err, want) {
				t.Fatalf("Wait = %v, want %v", err, want)
			}
			if got := limiter.GetAvailableTokens(); got != before {
				t.Fatalf("canceled request spent burst tokens: %d -> %d", before, got)
			}
		})
	}
}

func TestTypedRateLimitCancelledBurstNeverForwards(t *testing.T) {
	t.Parallel()
	mw := NewTypedRateLimitMiddleware(1)
	t.Cleanup(func() { _ = mw.limiter.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancel()
	called := false
	handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) { called = true; return nil, nil })
	if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("handler error = %v", err)
	}
	if called {
		t.Fatal("canceled request reached next handler")
	}
}

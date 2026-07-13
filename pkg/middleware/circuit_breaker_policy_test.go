package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func circuitContext(provider, method string) context.Context {
	ctx := context.WithValue(context.Background(), CtxKeyProvider, provider)
	return context.WithValue(ctx, CtxKeyMethod, method)
}

func TestCircuitBreakerMiddlewareIsolatesProviderAndOperation(t *testing.T) {
	mw := CircuitBreakerMiddleware(1, time.Hour)
	failure := errors.New("provider unavailable")
	wrapped := mw(func(ctx context.Context, _ any) (any, error) {
		if ctx.Value(CtxKeyProvider) == "primary" && ctx.Value(CtxKeyMethod) == "text" {
			return nil, failure
		}
		return "ok", nil
	})

	primaryText := circuitContext("primary", "text")
	if _, err := wrapped(primaryText, nil); !errors.Is(err, failure) {
		t.Fatalf("first primary text error = %v, want provider failure", err)
	}
	if _, err := wrapped(primaryText, nil); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("second primary text error = %v, want open circuit", err)
	}

	for _, key := range []struct {
		provider string
		method   string
	}{
		{provider: "fallback", method: "text"},
		{provider: "primary", method: "embeddings"},
	} {
		result, err := wrapped(circuitContext(key.provider, key.method), nil)
		if err != nil || result != "ok" {
			t.Fatalf("%s/%s = (%v, %v), want (ok, nil)", key.provider, key.method, result, err)
		}
	}
}

func TestCircuitBreakerMiddlewareRegistryIsRaceSafe(t *testing.T) {
	mw := CircuitBreakerMiddleware(1000, time.Hour)
	wrapped := mw(func(context.Context, any) (any, error) { return "ok", nil })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			provider := "one"
			if i%2 == 0 {
				provider = "two"
			}
			if _, err := wrapped(circuitContext(provider, "text"), nil); err != nil {
				t.Errorf("wrapped call: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestCircuitBreakerErrorPolicyOpensFastForTerminalClasses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{"rate limit", types.ErrRateLimited},
		{"quota", types.ErrQuotaExceeded},
		{"auth", types.ErrInvalidAPIKey},
		{"config", types.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cb := NewCircuitBreaker(5, time.Hour)
			_, err := cb.Execute(context.Background(), func() (any, error) {
				return nil, tt.err
			})
			require.Error(t, err)
			assert.Equal(t, StateOpen, cb.GetState())
		})
	}
}

func TestCircuitBreakerErrorPolicyKeepsTransientThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(5, time.Hour)
	_, err := cb.Execute(context.Background(), func() (any, error) {
		return nil, errors.New("temporary failure")
	})
	require.Error(t, err)
	assert.Equal(t, StateClosed, cb.GetState())
}

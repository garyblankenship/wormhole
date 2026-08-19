package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func circuitContext(provider string) context.Context {
	return context.WithValue(context.Background(), CtxKeyProvider, provider)
}

func TestTypedCircuitBreakerMiddlewareIsolatesProviderAndOperation(t *testing.T) {
	t.Parallel()
	mw := NewTypedCircuitBreakerMiddleware(1, time.Hour)
	failure := errors.New("provider unavailable")
	text := mw.ApplyText(func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
		if ctx.Value(CtxKeyProvider) == "primary" {
			return nil, failure
		}
		return &types.TextResponse{Text: "ok"}, nil
	})
	embeddings := mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})

	primary := circuitContext("primary")
	if _, err := text(primary, testTextRequest("request")); !errors.Is(err, failure) {
		t.Fatalf("first primary text error = %v, want provider failure", err)
	}
	if _, err := text(primary, testTextRequest("request")); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("second primary text error = %v, want open circuit", err)
	}
	if response, err := text(circuitContext("fallback"), testTextRequest("request")); err != nil || response.Text != "ok" {
		t.Fatalf("fallback text = (%#v, %v), want (ok, nil)", response, err)
	}
	if _, err := embeddings(primary, types.EmbeddingsRequest{Model: "embed", Input: []string{"request"}}); err != nil {
		t.Fatalf("primary embeddings leaked text circuit state: %v", err)
	}
}

func TestTypedCircuitBreakerMiddlewareRegistryIsRaceSafe(t *testing.T) {
	t.Parallel()
	mw := NewTypedCircuitBreakerMiddleware(1000, time.Hour)
	handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "ok"}, nil
	})
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			provider := "one"
			if i%2 == 0 {
				provider = "two"
			}
			if _, err := handler(circuitContext(provider), testTextRequest("request")); err != nil {
				t.Errorf("wrapped call: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestCircuitBreakerErrorPolicyOpensFastForTerminalClasses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "rate limit", err: types.ErrRateLimited},
		{name: "quota", err: types.ErrQuotaExceeded},
		{name: "auth", err: types.ErrInvalidAPIKey},
		{name: "config", err: types.ErrInvalidRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cb := NewCircuitBreaker(5, time.Hour)
			_, err := cb.Execute(context.Background(), func() (any, error) { return nil, test.err })
			if err == nil || cb.GetState() != StateOpen {
				t.Fatalf("terminal error = %v, state = %v; want open", err, cb.GetState())
			}
		})
	}
}

func TestCircuitBreakerErrorPolicyKeepsTransientThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(5, time.Hour)
	_, err := cb.Execute(context.Background(), func() (any, error) { return nil, errors.New("temporary failure") })
	if err == nil || cb.GetState() != StateClosed {
		t.Fatalf("transient error = %v, state = %v; want closed", err, cb.GetState())
	}
}

func TestCircuitBreakerIgnoresIncompleteGeneration(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(1, time.Hour)
	responses := []*types.TextResponse{
		{FinishReason: types.FinishReasonLength},
		{FinishReason: types.FinishReasonStop, Thinking: &types.Thinking{Content: "private"}},
		{FinishReason: types.FinishReasonStop},
	}
	for _, response := range responses {
		incomplete := types.NewIncompleteGenerationError(response)
		_, err := cb.Execute(context.Background(), func() (any, error) { return nil, incomplete })
		if !errors.Is(err, incomplete) || cb.GetState() != StateClosed {
			t.Fatalf("incomplete generation = %v, state = %v; want original error and closed", err, cb.GetState())
		}
	}
}

func TestCircuitBreakerReleasesHalfOpenIncompleteGeneration(t *testing.T) {
	t.Parallel()

	truncated := types.NewIncompleteGenerationError(&types.TextResponse{
		FinishReason: types.FinishReasonLength,
	})
	cb := NewCircuitBreaker(1, time.Hour)
	cb.state = StateHalfOpen

	for range 2 {
		_, err := cb.Execute(context.Background(), func() (any, error) { return nil, truncated })
		if !errors.Is(err, truncated) || cb.GetState() != StateHalfOpen {
			t.Fatalf("incomplete generation = %v, state = %v; want original error and half-open", err, cb.GetState())
		}
		if got := cb.halfOpenCalls.Load(); got != 0 {
			t.Fatalf("half-open calls = %d, want released admission", got)
		}
	}
}

func TestCircuitBreakerHalfOpenSuccessThreshold(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(4, time.Hour)
	cb.state = StateHalfOpen
	cb.failures = cb.failureThreshold

	for attempt := range cb.successThreshold {
		result, err := cb.Execute(context.Background(), func() (any, error) { return "ok", nil })
		if err != nil || result != "ok" {
			t.Fatalf("successful probe %d = (%v, %v)", attempt, result, err)
		}
	}
	if cb.GetState() != StateClosed || cb.failures != 0 || cb.successes != 0 {
		t.Fatalf("recovered state = %v failures=%d successes=%d", cb.GetState(), cb.failures, cb.successes)
	}
	if err := cb.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

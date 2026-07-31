package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestTypedCacheIsScopedDetachedAndSkipsStreams(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(8)
	t.Cleanup(func() { _ = cache.Close() })
	middleware := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Minute})
	request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "model"}}
	calls := 0
	handler := middleware.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls++
		return &types.TextResponse{Text: "one"}, nil
	})
	first, err := handler(context.WithValue(context.Background(), CtxKeyProvider, "openai"), request)
	if err != nil {
		t.Fatal(err)
	}
	first.Text = "changed"
	second, err := handler(context.WithValue(context.Background(), CtxKeyProvider, "openai"), request)
	if err != nil || second.Text != "one" || calls != 1 {
		t.Fatalf("cache result=%#v calls=%d err=%v", second, calls, err)
	}
	_, _ = handler(context.WithValue(context.Background(), CtxKeyProvider, "anthropic"), request)
	if calls != 2 {
		t.Fatalf("provider identity leaked cache: calls=%d", calls)
	}
	streams := 0
	stream := middleware.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		streams++
		ch := make(chan types.StreamChunk)
		close(ch)
		return ch, nil
	})
	_, _ = stream(context.Background(), request)
	_, _ = stream(context.Background(), request)
	if streams != 2 {
		t.Fatalf("stream calls=%d, want 2", streams)
	}
}

func TestTypedCachePreservesTypedNilResponses(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	middleware := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Minute})
	calls := 0
	handler := middleware.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls++
		return nil, nil
	})
	for range 2 {
		response, err := handler(context.Background(), types.TextRequest{})
		if err != nil || response != nil {
			t.Fatalf("response=%#v err=%v, want nil response without error", response, err)
		}
	}
	if calls != 1 {
		t.Fatalf("handler calls=%d, want one cached call", calls)
	}
}

func TestTypedCircuitAndRatePolicies(t *testing.T) {
	t.Parallel()
	circuit := NewTypedCircuitBreakerMiddleware(1, time.Hour)
	fail := circuit.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return nil, errors.New("failed")
	})
	ctx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	_, _ = fail(ctx, types.TextRequest{})
	if _, err := fail(ctx, types.TextRequest{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("circuit err=%v", err)
	}
	other := circuit.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})
	if _, err := other(ctx, types.EmbeddingsRequest{}); err != nil {
		t.Fatalf("operation isolation: %v", err)
	}
	otherProvider := circuit.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{}, nil
	})
	otherCtx := context.WithValue(context.Background(), CtxKeyProvider, "anthropic")
	if _, err := otherProvider(otherCtx, types.TextRequest{}); err != nil {
		t.Fatalf("provider isolation: %v", err)
	}

	rate := NewTypedRateLimitMiddleware(1)
	if _, err := rate.ApplyRerank(func(context.Context, types.RerankRequest) (*types.RerankResponse, error) {
		return &types.RerankResponse{}, nil
	})(context.Background(), types.RerankRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := rate.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(context.Background(), types.AudioRequest{}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rate.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{}, nil
	})(canceled, types.TextRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("shared rate limit cancellation error = %v, want context cancellation", err)
	}
}

func TestTypedCircuitBreakerRecordsStreamChunkErrors(t *testing.T) {
	t.Parallel()
	circuit := NewTypedCircuitBreakerMiddleware(1, time.Hour)
	handler := circuit.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		stream := make(chan types.StreamChunk, 1)
		stream <- types.StreamChunk{Error: errors.New("stream failed")}
		close(stream)
		return stream, nil
	})
	ctx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	stream, err := handler(ctx, types.TextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("second stream error = %v, want open circuit", err)
	}
}

func TestTypedCircuitBreakerIgnoresCallerCancellation(t *testing.T) {
	t.Parallel()
	circuit := NewTypedCircuitBreakerMiddleware(1, time.Hour)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), CtxKeyProvider, "openai"))
	cancel()

	handler := circuit.ApplyText(func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
		return nil, ctx.Err()
	})
	if _, err := handler(ctx, types.TextRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v, want context cancellation", err)
	}
	healthy := circuit.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{}, nil
	})
	providerCtx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	if _, err := healthy(providerCtx, types.TextRequest{}); err != nil {
		t.Fatalf("caller cancellation opened circuit: %v", err)
	}
}

func TestTypedCircuitBreakerIgnoresStreamCancellation(t *testing.T) {
	t.Parallel()
	circuit := NewTypedCircuitBreakerMiddleware(1, time.Hour)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), CtxKeyProvider, "openai"))
	streamStarted := make(chan struct{})
	handler := circuit.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		stream := make(chan types.StreamChunk)
		close(streamStarted)
		return stream, nil
	})
	stream, err := handler(ctx, types.TextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	<-streamStarted
	cancel()
	for range stream {
	}

	healthy := circuit.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		stream := make(chan types.StreamChunk)
		close(stream)
		return stream, nil
	})
	providerCtx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	stream, err = healthy(providerCtx, types.TextRequest{})
	if err != nil {
		t.Fatalf("stream cancellation opened circuit: %v", err)
	}
	for range stream {
	}
}

func TestTypedPoliciesCoverEveryProviderOperation(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(32)
	t.Cleanup(func() { _ = cache.Close() })
	chain := types.NewProviderChain(
		NewTypedRateLimitMiddleware(100),
		NewTypedCircuitBreakerMiddleware(10, time.Minute),
		NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Minute}),
	)
	ctx := context.WithValue(context.Background(), CtxKeyProvider, "openai")

	if _, err := chain.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{}, nil
	})(ctx, types.TextRequest{}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if _, err := chain.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		stream := make(chan types.StreamChunk)
		close(stream)
		return stream, nil
	})(ctx, types.TextRequest{}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if _, err := chain.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{}, nil
	})(ctx, types.StructuredRequest{}); err != nil {
		t.Fatalf("structured: %v", err)
	}
	if _, err := chain.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})(ctx, types.EmbeddingsRequest{}); err != nil {
		t.Fatalf("embeddings: %v", err)
	}
	if _, err := chain.ApplyRerank(func(context.Context, types.RerankRequest) (*types.RerankResponse, error) {
		return &types.RerankResponse{}, nil
	})(ctx, types.RerankRequest{}); err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if _, err := chain.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(ctx, types.AudioRequest{}); err != nil {
		t.Fatalf("audio: %v", err)
	}
	if _, err := chain.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{}, nil
	})(ctx, types.ImageRequest{}); err != nil {
		t.Fatalf("image: %v", err)
	}
}

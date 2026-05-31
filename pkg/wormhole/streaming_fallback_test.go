package wormhole

import (
	"context"
	"errors"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

type fallbackStreamProvider struct {
	*types.BaseProvider
	streams map[string]func() (<-chan types.TextChunk, error)
}

func newFallbackStreamProvider(streams map[string]func() (<-chan types.TextChunk, error)) *fallbackStreamProvider {
	return &fallbackStreamProvider{
		BaseProvider: types.NewBaseProvider("fallback-stream"),
		streams:      streams,
	}
}

func (p *fallbackStreamProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	fn := p.streams[request.Model]
	if fn == nil {
		return nil, errors.New("unexpected model " + request.Model)
	}
	return fn()
}

func streamChunks(chunks ...types.TextChunk) func() (<-chan types.TextChunk, error) {
	return func() (<-chan types.TextChunk, error) {
		ch := make(chan types.TextChunk, len(chunks))
		for _, chunk := range chunks {
			ch <- chunk
		}
		close(ch)
		return ch, nil
	}
}

func collectStreamChunks(t *testing.T, stream <-chan types.StreamChunk) []types.StreamChunk {
	t.Helper()
	var chunks []types.StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func newStreamingFallbackClient(provider *fallbackStreamProvider) *Wormhole {
	return New(
		WithDiscovery(false),
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
	)
}

func TestTextRequestBuilderStreamFallsBackOnOpenError(t *testing.T) {
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return nil, errors.New("open failed")
		},
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "fallback" {
		t.Fatalf("chunks = %#v, want fallback content", chunks)
	}
}

func TestTextRequestBuilderStreamFallsBackOnFirstErrorChunk(t *testing.T) {
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary":  streamChunks(types.TextChunk{Error: errors.New("rate limited")}),
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "fallback" {
		t.Fatalf("chunks = %#v, want fallback content", chunks)
	}
}

func TestTextRequestBuilderStreamDoesNotFallBackAfterEmission(t *testing.T) {
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": streamChunks(
			types.TextChunk{Text: "primary"},
			types.TextChunk{Error: errors.New("late failure")},
		),
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 2 || chunks[0].Content() != "primary" || !chunks[1].HasError() {
		t.Fatalf("chunks = %#v, want primary content followed by error", chunks)
	}
}

func TestTextRequestBuilderStreamAllAttemptsFailBeforeEmission(t *testing.T) {
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return nil, errors.New("open failed")
		},
		"fallback": streamChunks(types.TextChunk{Error: errors.New("fallback failed")}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || !chunks[0].HasError() {
		t.Fatalf("chunks = %#v, want one accumulated error chunk", chunks)
	}
}

func TestTextRequestBuilderStreamContextCancellationClosesStream(t *testing.T) {
	blocked := make(chan types.TextChunk)
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return blocked, nil
		},
	})
	client := newStreamingFallbackClient(provider)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Text().Model("primary").Prompt("hi").Stream(ctx)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	cancel()
	for range stream {
		t.Fatal("expected stream to close without chunks after cancellation")
	}
}

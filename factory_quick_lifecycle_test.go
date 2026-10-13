package wormhole

import (
	"context"
	"errors"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type quickOwnedProvider struct {
	*types.BaseProvider
	stream func(context.Context, types.TextRequest) (<-chan types.TextChunk, error)
}

func (p *quickOwnedProvider) Stream(ctx context.Context, r types.TextRequest) (<-chan types.TextChunk, error) {
	return p.stream(ctx, r)
}
func (p *quickOwnedProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityStream}
}

func newQuickOwnedClient(t *testing.T, stream func(context.Context, types.TextRequest) (<-chan types.TextChunk, error)) *Wormhole {
	t.Helper()
	provider := &quickOwnedProvider{BaseProvider: types.NewBaseProvider("quick-owned"), stream: stream}
	client := New(WithDefaultProvider("quick-owned"), WithDiscovery(false), WithModelValidation(false),
		WithProviderConfig("quick-owned", types.ProviderConfig{DynamicModels: true}),
		WithCustomProvider("quick-owned", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
func TestQuickStreamClosesClientBeforeChannelCompletion(t *testing.T) {
	t.Parallel()
	client := newQuickOwnedClient(t, func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		out := make(chan types.TextChunk, 1)
		out <- types.TextChunk{Text: "ok"}
		close(out)
		return out, nil
	})
	stream, err := quickStreamWithClient(t.Context(), client, "fixture", "request")
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		got += chunk.Content()
	}
	if got != "ok" || !client.IsShuttingDown() {
		t.Fatal("quick stream did not preserve content and close its owned client")
	}
}
func TestQuickStreamClosesClientOnKickoffFailure(t *testing.T) {
	t.Parallel()
	client := newQuickOwnedClient(t, func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		t.Error("invalid request contacted provider")
		return nil, nil
	})
	stream, err := quickStreamWithClient(t.Context(), client, "", "request")
	if err == nil || stream != nil || !client.IsShuttingDown() {
		t.Fatalf("invalid kickoff did not close client: %v", err)
	}
}
func TestQuickStreamPreservesAsyncProviderErrorAndCloses(t *testing.T) {
	t.Parallel()
	want := errors.New("fixture stream failure")
	client := newQuickOwnedClient(t, func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) { return nil, want })
	stream, err := quickStreamWithClient(t.Context(), client, "fixture", "request")
	if err != nil {
		t.Fatal(err)
	}
	var got error
	for chunk := range stream {
		if chunk.Error != nil {
			got = chunk.Error
		}
	}
	if !errors.Is(got, want) || !client.IsShuttingDown() {
		t.Fatalf("async error or cleanup lost: %v", got)
	}
}

func TestQuickStreamCancellationUnblocksAbandonedOutput(t *testing.T) {
	t.Parallel()
	client := newQuickOwnedClient(t, func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		out := make(chan types.TextChunk, 1)
		out <- types.TextChunk{Text: "ok"}
		close(out)
		return out, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream, err := quickStreamWithClient(ctx, client, "fixture", "request")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for range stream {
	}
	if !client.IsShuttingDown() {
		t.Fatal("canceled stream retained its owned client")
	}
}

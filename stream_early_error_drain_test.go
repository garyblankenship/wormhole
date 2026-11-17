package wormhole

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

type earlyErrorDrainProvider struct {
	*types.BaseProvider
	blockedSend bool
	closed      chan struct{}
	rescue      chan struct{}
	calls       int
}

func (p *earlyErrorDrainProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	p.calls++
	if request.Model == "fallback" {
		return streamChunks(types.TextChunk{Text: "fallback"})()
	}
	stream := make(chan types.TextChunk)
	go func() {
		defer close(p.closed)
		defer close(stream)
		select {
		case stream <- types.TextChunk{Error: errors.New("early failure")}:
		case <-ctx.Done():
			return
		}
		if p.blockedSend {
			// Model a custom provider that has already committed to one more send.
			// The bounded drain must receive it without exposing it to the caller.
			select {
			case stream <- types.TextChunk{Text: "discarded"}:
			case <-p.rescue:
			}
		} else {
			<-ctx.Done()
		}
	}()
	return stream, nil
}

func TestStreamEarlyErrorDrainReleasesProvider(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "cooperative"
		if blocked {
			name = "blocked sender"
		}
		t.Run(name, func(t *testing.T) {
			provider := &earlyErrorDrainProvider{BaseProvider: types.NewBaseProvider("mock"), blockedSend: blocked, closed: make(chan struct{}), rescue: make(chan struct{})}
			t.Cleanup(func() { close(provider.rescue) })
			client := New(WithDiscovery(false), WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("mock", types.ProviderConfig{}))
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(ctx)
			require.NoError(t, err)
			chunks := collectStreamChunks(t, stream)
			require.Len(t, chunks, 1)
			require.Equal(t, "fallback", chunks[0].Content())
			require.NoError(t, chunks[0].Error)
			require.Equal(t, 2, provider.calls)
			select {
			case <-provider.closed:
			case <-ctx.Done():
				t.Fatal("failed attempt remained blocked")
			}
		})
	}
}

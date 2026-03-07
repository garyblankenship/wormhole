package wormhole_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingTextProvider struct {
	*types.BaseProvider
	started   chan struct{}
	unblock   chan struct{}
	startOnce sync.Once
	callCount atomic.Int32
}

func newBlockingTextProvider(name string) *blockingTextProvider {
	return &blockingTextProvider{
		BaseProvider: types.NewBaseProvider(name),
		started:      make(chan struct{}),
		unblock:      make(chan struct{}),
	}
}

func (p *blockingTextProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat}
}

func (p *blockingTextProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	p.callCount.Add(1)
	p.startOnce.Do(func() { close(p.started) })

	select {
	case <-p.unblock:
		return &types.TextResponse{
			ID:           "blocking-1",
			Model:        request.Model,
			Text:         "done",
			FinishReason: types.FinishReasonStop,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type countingTextProvider struct {
	*types.BaseProvider
	callCount atomic.Int32
}

func newCountingTextProvider(name string) *countingTextProvider {
	return &countingTextProvider{BaseProvider: types.NewBaseProvider(name)}
}

func (p *countingTextProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat}
}

func (p *countingTextProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	call := p.callCount.Add(1)
	return &types.TextResponse{
		ID:           "counting",
		Model:        request.Model,
		Text:         "response-" + request.Messages[0].GetContent().(string),
		FinishReason: types.FinishReasonStop,
		Usage: &types.Usage{
			CompletionTokens: int(call),
		},
	}, nil
}

type captureConfigProvider struct {
	*types.BaseProvider
	config types.ProviderConfig
}

func (p *captureConfigProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat}
}

func (p *captureConfigProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	return &types.TextResponse{
		ID:           "capture",
		Model:        request.Model,
		Text:         p.config.BaseURL,
		FinishReason: types.FinishReasonStop,
	}, nil
}

func TestShutdownWaitsForInflightRequest(t *testing.T) {
	provider := newBlockingTextProvider("blocking")
	client := wormhole.New(
		wormhole.WithDefaultProvider("blocking"),
		wormhole.WithCustomProvider("blocking", func(cfg types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		wormhole.WithProviderConfig("blocking", types.ProviderConfig{}),
	)

	requestDone := make(chan error, 1)
	go func() {
		_, err := client.Text().Model("test-model").Prompt("hello").Generate(context.Background())
		requestDone <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request to start")
	}

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- client.Shutdown(ctx)
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before in-flight request finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(provider.unblock)
	require.NoError(t, <-requestDone)
	require.NoError(t, <-shutdownDone)

	_, err := client.Text().Model("test-model").Prompt("after shutdown").Generate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestIdempotencyDeduplicatesRepeatedRequests(t *testing.T) {
	provider := newCountingTextProvider("counting")
	client := wormhole.New(
		wormhole.WithDefaultProvider("counting"),
		wormhole.WithCustomProvider("counting", func(cfg types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		wormhole.WithProviderConfig("counting", types.ProviderConfig{}),
		wormhole.WithIdempotencyKey("same-request", time.Minute),
	)

	ctx := context.Background()
	builder := client.Text().Model("test-model").Prompt("repeat me")

	first, err := builder.Generate(ctx)
	require.NoError(t, err)
	second, err := builder.Generate(ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), provider.callCount.Load())
	assert.Equal(t, first.Text, second.Text)
	assert.Equal(t, first.Usage.CompletionTokens, second.Usage.CompletionTokens)

	_, err = client.Text().Model("test-model").Prompt("different").Generate(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(2), provider.callCount.Load())

	client.ClearIdempotencyCache()
	_, err = builder.Generate(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(3), provider.callCount.Load())
}

func TestBaseURLOverridePreservesProviderConfigAndFactory(t *testing.T) {
	var (
		mu          sync.Mutex
		capturedCfg types.ProviderConfig
	)

	client := wormhole.New(
		wormhole.WithDefaultProvider("capture"),
		wormhole.WithCustomProvider("capture", func(cfg types.ProviderConfig) (types.Provider, error) {
			mu.Lock()
			capturedCfg = cfg
			mu.Unlock()
			return &captureConfigProvider{BaseProvider: types.NewBaseProvider("capture"), config: cfg}, nil
		}),
		wormhole.WithProviderConfig("capture", types.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: "https://original.example/v1",
			Headers: map[string]string{"X-Test": "keep"},
			Timeout: 7,
			Params:  map[string]any{"sentinel": "yes"},
		}),
	)

	resp, err := client.Text().
		BaseURL("https://override.example/v1").
		Model("test-model").
		Prompt("hello").
		Generate(context.Background())
	require.NoError(t, err)

	mu.Lock()
	gotCfg := capturedCfg
	mu.Unlock()

	assert.Equal(t, "https://override.example/v1", resp.Text)
	assert.Equal(t, "https://override.example/v1", gotCfg.BaseURL)
	assert.Equal(t, "test-key", gotCfg.APIKey)
	assert.Equal(t, 7, gotCfg.Timeout)
	assert.Equal(t, "keep", gotCfg.Headers["X-Test"])
	assert.Equal(t, "yes", gotCfg.Params["sentinel"])
}

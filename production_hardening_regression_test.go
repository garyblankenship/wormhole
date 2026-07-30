package wormhole_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/middleware"
	"github.com/garyblankenship/wormhole/v2/types"
)

type blockingTextProvider struct {
	*types.BaseProvider
	started    chan struct{}
	unblock    chan struct{}
	startOnce  sync.Once
	callCount  atomic.Int32
	closeCount atomic.Int32
	closeErr   error
}

func (p *blockingTextProvider) Close() error {
	p.closeCount.Add(1)
	return p.closeErr
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
		ID:           p.Name(),
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
	t.Parallel()
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

func TestShutdownTimeoutIsCallerLocalAndCleanupContinues(t *testing.T) {
	t.Parallel()

	wantCleanupErr := errors.New("provider cleanup failed")
	provider := newBlockingTextProvider("blocking")
	provider.closeErr = wantCleanupErr
	client := wormhole.New(
		wormhole.WithDiscovery(false),
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
	<-provider.started

	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	require.ErrorIs(t, client.Shutdown(shutdownCtx), context.Canceled)
	assert.True(t, client.IsShuttingDown())
	assert.Equal(t, int32(0), provider.closeCount.Load())

	close(provider.unblock)
	require.NoError(t, <-requestDone)
	require.ErrorIs(t, client.Shutdown(context.Background()), wantCleanupErr)
	assert.Equal(t, int32(1), provider.closeCount.Load())

	require.ErrorIs(t, client.Shutdown(context.Background()), wantCleanupErr)
	assert.Equal(t, int32(1), provider.closeCount.Load())
}

func TestProviderMethodsRejectAfterShutdownWithoutFactory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*wormhole.Wormhole) error
	}{
		{
			name: "Provider",
			call: func(client *wormhole.Wormhole) error {
				_, err := client.Provider("custom")
				return err
			},
		},
		{
			name: "ProviderWithHandle",
			call: func(client *wormhole.Wormhole) error {
				_, err := client.ProviderWithHandle("custom")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var factoryCalls atomic.Int32
			client := wormhole.New(
				wormhole.WithDiscovery(false),
				wormhole.WithCustomProvider("custom", func(types.ProviderConfig) (types.Provider, error) {
					factoryCalls.Add(1)
					return newCountingTextProvider("custom"), nil
				}),
				wormhole.WithProviderConfig("custom", types.ProviderConfig{}),
			)

			require.NoError(t, client.Shutdown(context.Background()))
			err := tt.call(client)
			require.ErrorContains(t, err, "client is shutting down")
			assert.Equal(t, int32(0), factoryCalls.Load())
		})
	}
}

func TestProviderCreationRejectedWhenShutdownWins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*wormhole.Wormhole) error
	}{
		{
			name: "Provider",
			call: func(client *wormhole.Wormhole) error {
				_, err := client.Provider("custom")
				return err
			},
		},
		{
			name: "ProviderWithHandle",
			call: func(client *wormhole.Wormhole) error {
				_, err := client.ProviderWithHandle("custom")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factoryStarted := make(chan struct{})
			releaseFactory := make(chan struct{})
			provider := newBlockingTextProvider("custom")
			client := wormhole.New(
				wormhole.WithDiscovery(false),
				wormhole.WithCustomProvider("custom", func(types.ProviderConfig) (types.Provider, error) {
					close(factoryStarted)
					<-releaseFactory
					return provider, nil
				}),
				wormhole.WithProviderConfig("custom", types.ProviderConfig{}),
			)

			acquireDone := make(chan error, 1)
			go func() {
				acquireDone <- tt.call(client)
			}()
			<-factoryStarted

			shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
			cancelShutdown()
			require.ErrorIs(t, client.Shutdown(shutdownCtx), context.Canceled)
			close(releaseFactory)

			require.ErrorContains(t, <-acquireDone, "client is shutting down")
			require.NoError(t, client.Shutdown(context.Background()))
			assert.Equal(t, int32(1), provider.closeCount.Load())
		})
	}
}

func TestIdempotencyDeduplicatesRepeatedRequests(t *testing.T) {
	t.Parallel()
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

func TestIdempotencyElapsedTTLDoesNotDuplicateInFlightProviderCall(t *testing.T) {
	t.Parallel()

	provider := newBlockingTextProvider("blocking")
	client := wormhole.New(
		wormhole.WithDefaultProvider("blocking"),
		wormhole.WithCustomProvider("blocking", func(cfg types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		wormhole.WithProviderConfig("blocking", types.ProviderConfig{}),
		wormhole.WithIdempotencyKey("same-request", time.Millisecond),
	)

	firstDone := make(chan error, 1)
	go func() {
		_, err := client.Text().Model("test-model").Prompt("repeat me").Generate(context.Background())
		firstDone <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first request to start")
	}
	// Let the old, pre-remediation in-flight expiry elapse. The provider remains
	// blocked, so the second caller must still wait on its original entry.
	<-time.After(2 * time.Millisecond)

	duplicateCtx, cancelDuplicate := context.WithCancel(context.Background())
	duplicateDone := make(chan error, 1)
	go func() {
		_, err := client.Text().Model("test-model").Prompt("repeat me").Generate(duplicateCtx)
		duplicateDone <- err
	}()

	select {
	case err := <-duplicateDone:
		t.Fatalf("duplicate returned before the original provider request completed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	assert.Equal(t, int32(1), provider.callCount.Load())

	cancelDuplicate()
	require.ErrorIs(t, <-duplicateDone, context.Canceled)
	close(provider.unblock)
	require.NoError(t, <-firstDone)
}

// Regression: two requests identical except for ProviderOptions must NOT collide on
// the same idempotency key. Before folding GetProviderOptions() into the hash, the
// key derived only from json.Marshal(request), and ProviderOptions carries json:"-"
// so it was silently excluded — the second call below would have been wrongly
// deduplicated against the first (callCount would stay at 1).
func TestIdempotencyDistinguishesProviderOptions(t *testing.T) {
	t.Parallel()
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

	_, err := client.Text().Model("test-model").Prompt("repeat me").
		ProviderOptions(map[string]any{"temperature": 0.1}).Generate(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), provider.callCount.Load())

	_, err = client.Text().Model("test-model").Prompt("repeat me").
		ProviderOptions(map[string]any{"temperature": 0.9}).Generate(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(2), provider.callCount.Load())
}

func TestIdempotencyDuplicateWaitHonorsCallerContext(t *testing.T) {
	t.Parallel()
	provider := newBlockingTextProvider("blocking")
	client := wormhole.New(
		wormhole.WithDefaultProvider("blocking"),
		wormhole.WithCustomProvider("blocking", func(cfg types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		wormhole.WithProviderConfig("blocking", types.ProviderConfig{}),
		wormhole.WithIdempotencyKey("same-request", time.Minute),
	)

	firstDone := make(chan error, 1)
	go func() {
		_, err := client.Text().Model("test-model").Prompt("repeat me").Generate(context.Background())
		firstDone <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first request to start")
	}

	duplicateCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Text().Model("test-model").Prompt("repeat me").Generate(duplicateCtx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), provider.callCount.Load())

	close(provider.unblock)
	require.NoError(t, <-firstDone)
}

func TestIdempotencyOwnerHonorsCallerContext(t *testing.T) {
	t.Parallel()
	provider := newBlockingTextProvider("blocking")
	client := wormhole.New(
		wormhole.WithDefaultProvider("blocking"),
		wormhole.WithCustomProvider("blocking", func(cfg types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		wormhole.WithProviderConfig("blocking", types.ProviderConfig{}),
		wormhole.WithIdempotencyKey("same-request", time.Minute),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	_, err := client.Text().Model("test-model").Prompt("repeat me").Generate(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(1), provider.callCount.Load())
}

func TestLegacyCacheMiddlewareNamespacesByProvider(t *testing.T) {
	t.Parallel()
	providerA := newCountingTextProvider("provider-a")
	providerB := newCountingTextProvider("provider-b")
	cache := middleware.NewTTLCache(10, time.Minute)
	client := wormhole.New(
		wormhole.WithDefaultProvider("provider-a"),
		wormhole.WithCustomProvider("provider-a", func(cfg types.ProviderConfig) (types.Provider, error) {
			return providerA, nil
		}),
		wormhole.WithCustomProvider("provider-b", func(cfg types.ProviderConfig) (types.Provider, error) {
			return providerB, nil
		}),
		wormhole.WithProviderConfig("provider-a", types.ProviderConfig{}),
		wormhole.WithProviderConfig("provider-b", types.ProviderConfig{}),
		wormhole.WithMiddleware(middleware.CacheMiddleware(middleware.CacheConfig{
			Cache: cache,
			TTL:   time.Minute,
		})),
	)

	ctx := context.Background()
	first, err := client.Text().Using("provider-a").Model("same-model").Prompt("same prompt").Generate(ctx)
	require.NoError(t, err)
	second, err := client.Text().Using("provider-b").Model("same-model").Prompt("same prompt").Generate(ctx)
	require.NoError(t, err)

	assert.Equal(t, "provider-a", first.ID)
	assert.Equal(t, "provider-b", second.ID)
	assert.Equal(t, int32(1), providerA.callCount.Load())
	assert.Equal(t, int32(1), providerB.callCount.Load())
}

func TestBaseURLOverridePreservesProviderConfigAndFactory(t *testing.T) {
	t.Parallel()
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

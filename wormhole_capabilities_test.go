package wormhole

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

func TestCapabilitiesFromConfiguredProvider(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock")
	client := New(
		WithCustomProvider("mock", wmtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDefaultProvider("mock"),
		WithDiscovery(false),
	)

	caps := client.ProviderCapabilities("mock")
	assert.True(t, caps.SupportsText())
	assert.True(t, caps.SupportsStructured())
	assert.True(t, caps.SupportsEmbeddings())
	assert.True(t, caps.SupportsImages())
	assert.True(t, caps.SupportsAudio())
	assert.True(t, caps.SupportsStreaming())
	assert.False(t, caps.SupportsToolCalling())
	assert.NotEmpty(t, caps.All())
}

func TestProviderCapabilitiesUsesScopedLease(t *testing.T) {
	t.Parallel()

	for _, lookup := range []struct {
		name string
		run  func(*Wormhole) error
	}{
		{"provider", func(client *Wormhole) error {
			_ = client.ProviderCapabilities("mock")
			return nil
		}},
		{"model fallback", func(client *Wormhole) error {
			_, err := client.ModelCapabilities("mock", "missing")
			return err
		}},
	} {
		for _, cleanup := range []string{"age", "max-count"} {
			t.Run(lookup.name+"/"+cleanup, func(t *testing.T) {
				t.Parallel()
				provider := newLifecycleProvider("mock")
				client := New(
					WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
					WithProviderConfig("mock", types.ProviderConfig{}),
					WithDefaultProvider("mock"),
					WithDiscovery(false),
				)
				defer func() { _ = client.Shutdown(context.Background()) }()

				if err := lookup.run(client); err != nil {
					t.Fatalf("capability lookup: %v", err)
				}
				if cleanup == "age" {
					client.CleanupStaleProviders(0, 0)
				} else {
					other := newLifecycleProvider("other")
					client.providerFactories["other"] = func(types.ProviderConfig) (types.Provider, error) { return other, nil }
					client.config.Providers["other"] = types.ProviderConfig{}
					_ = client.ProviderCapabilities("other")
					atomic.StoreInt64(&client.providers["mock"].lastUsed, time.Now().Add(-time.Hour).UnixNano())
					client.CleanupStaleProviders(24*time.Hour, 1)
				}
				if got := provider.closeCount.Load(); got != 1 {
					t.Fatalf("capability lookup provider close count = %d, want 1", got)
				}
			})
		}
	}
}

func TestCapabilitiesHelperMethods(t *testing.T) {
	t.Parallel()
	modelCaps := []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityImages,
		types.CapabilityAudio,
		types.CapabilityFunctions,
		types.CapabilityStream,
		types.CapabilityVision,
	}

	caps := capabilitiesFromModelCapabilities("test-provider", modelCaps)
	assert.True(t, caps.SupportsText())
	assert.True(t, caps.SupportsStructured())
	assert.True(t, caps.SupportsEmbeddings())
	assert.True(t, caps.SupportsImages())
	assert.True(t, caps.SupportsAudio())
	assert.True(t, caps.SupportsToolCalling())
	assert.True(t, caps.SupportsStreaming())
	assert.True(t, caps.SupportsVision())
}

type discoveryMockFetcher struct{}

func (discoveryMockFetcher) Name() string { return "custom-fetcher" }
func (discoveryMockFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	return []*types.ModelInfo{
		{
			ID:           "custom-vision-model",
			Provider:     "custom-fetcher",
			Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityVision},
		},
	}, nil
}

func TestModelCapabilitiesFromDiscovery(t *testing.T) {
	t.Parallel()
	client := New(
		WithDiscoveryConfig(discovery.DiscoveryConfig{
			DisableFileCache:         true,
			DisableBackgroundRefresh: true,
			CacheTTL:                 time.Hour,
		}),
	)
	require.NotNil(t, client.discoveryService)
	client.discoveryService.RegisterFetcher(discoveryMockFetcher{})
	require.NoError(t, client.RefreshModels())

	caps, err := client.ModelCapabilities("custom-fetcher", "custom-vision-model")
	require.NoError(t, err)
	assert.True(t, caps.SupportsText())
	assert.True(t, caps.SupportsVision())
}

func TestConservativeProviderCapabilities(t *testing.T) {
	t.Parallel()
	client := New(WithDiscovery(false))

	openaiCaps := client.ProviderCapabilities("openai")
	assert.True(t, openaiCaps.SupportsText())
	assert.False(t, openaiCaps.SupportsImages())

	unknownCaps := client.ProviderCapabilities("unknown")
	assert.False(t, unknownCaps.SupportsText())
	assert.Empty(t, unknownCaps.All())
}

func TestModelCapabilitiesValidationAndFallback(t *testing.T) {
	t.Parallel()
	client := New(WithDiscovery(false))

	_, err := client.ModelCapabilities("", "model")
	require.Error(t, err)

	_, err = client.ModelCapabilities("openai", "")
	require.Error(t, err)

	caps, err := client.ModelCapabilities("openai", "missing-model")
	require.NoError(t, err)
	assert.True(t, caps.SupportsText())
}

func TestCapabilitiesNilReceiver(t *testing.T) {
	t.Parallel()
	var caps *Capabilities
	assert.False(t, caps.Has(CapabilityText))
	assert.Nil(t, caps.All())
	assert.False(t, caps.SupportsText())
	assert.False(t, caps.SupportsVision())
}

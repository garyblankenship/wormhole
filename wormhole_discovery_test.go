package wormhole

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/discovery"
	"github.com/garyblankenship/wormhole/v2/types"
)

type contextProbeFetcher struct{}

func (contextProbeFetcher) Name() string {
	return "ctxprobe"
}

func (contextProbeFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return []*types.ModelInfo{
			{ID: "ctxprobe-model-1", Provider: "ctxprobe"},
		}, nil
	}
}

func TestListAvailableModelsWithContextPropagatesCancellation(t *testing.T) {
	t.Parallel()

	client := New(WithDiscoveryConfig(discovery.DiscoveryConfig{
		CacheTTL:                 time.Hour,
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}))
	require.NotNil(t, client.discoveryService)
	client.discoveryService.RegisterFetcher(contextProbeFetcher{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ListAvailableModelsWithContext(ctx, "ctxprobe")

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "error = %v", err)
}

func TestRefreshModelsWithContextPropagatesCancellation(t *testing.T) {
	t.Parallel()

	client := New(WithDiscoveryConfig(discovery.DiscoveryConfig{
		CacheTTL:                 time.Hour,
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}))
	require.NotNil(t, client.discoveryService)
	client.discoveryService.RegisterFetcher(contextProbeFetcher{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.RefreshModelsWithContext(ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "ctxprobe")
	require.Contains(t, err.Error(), context.Canceled.Error())
}

func TestInitializeDiscoveryServiceRegistersConfiguredProviderFetchers(t *testing.T) {
	t.Parallel()

	client := New(
		WithDiscoveryConfig(discovery.DiscoveryConfig{
			DisableFileCache:         true,
			DisableBackgroundRefresh: true,
		}),
		WithOpenAI("openai-key"),
		WithGemini("gemini-key"),
		WithOpenAICompatible("custom", "https://custom.example.test/v1", types.ProviderConfig{
			APIKey:  "custom-key",
			Headers: map[string]string{"X-Custom": "yes"},
		}),
	)

	providers := client.ModelDiscoveryProviders()
	require.Contains(t, providers, "openai")
	require.Contains(t, providers, "gemini")
	require.Contains(t, providers, "custom")
}

func TestInitializeDiscoveryServiceUsesAPIKeysFallback(t *testing.T) {
	t.Parallel()

	client := New(
		WithDiscoveryConfig(discovery.DiscoveryConfig{
			DisableFileCache:         true,
			DisableBackgroundRefresh: true,
		}),
		WithProviderConfig("openai", types.ProviderConfig{APIKeys: []string{"test-openai-key"}}),
		WithProviderConfig("gemini", types.ProviderConfig{APIKeys: []string{"test-gemini-key"}}),
	)

	providers := client.ModelDiscoveryProviders()
	require.Contains(t, providers, "openai")
	require.Contains(t, providers, "gemini")
}

func TestDiscoveryConvenienceMethodsAndLifecycle(t *testing.T) {
	t.Parallel()

	client := New(
		WithDiscoveryConfig(discovery.DiscoveryConfig{
			DisableFileCache:         true,
			DisableBackgroundRefresh: true,
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
	)
	require.NotNil(t, client.discoveryService)
	client.discoveryService.RegisterFetcher(contextProbeFetcher{})

	// Test ConfiguredProviders
	configured := client.ConfiguredProviders()
	assert.Contains(t, configured, "mock")

	// Test ListAvailableModels wrapper
	models, err := client.ListAvailableModels("ctxprobe")
	require.NoError(t, err)
	assert.Len(t, models, 1)

	// Test RefreshModels wrapper
	err = client.RefreshModels()
	require.NoError(t, err)

	// Test ClearModelCache
	client.ClearModelCache()

	// Test StopModelDiscovery
	client.StopModelDiscovery()
}

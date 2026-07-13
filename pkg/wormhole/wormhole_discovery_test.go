package wormhole

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/require"
)

type contextProbeFetcher struct{}

func (contextProbeFetcher) Name() string {
	return "ctxprobe"
}

func (contextProbeFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
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

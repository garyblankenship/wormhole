package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestListAvailableModelsWithStatus(t *testing.T) {
	t.Parallel()

	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{
		CacheTTL:                 time.Hour,
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}, staticModelFetcher{name: "healthy", models: []*types.ModelInfo{{ID: "model", Provider: "healthy"}}})
	t.Cleanup(client.StopModelDiscovery)

	result, err := client.ListAvailableModelsWithStatus(context.Background(), "healthy")
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.False(t, result.Stale)
}

func TestListAvailableModelsWithStatusPropagatesCancellation(t *testing.T) {
	t.Parallel()

	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}, contextProbeFetcher{})
	t.Cleanup(client.StopModelDiscovery)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListAvailableModelsWithStatus(ctx, "ctxprobe")
	require.ErrorIs(t, err, context.Canceled)
}

func TestListAvailableModelsWithStatusReportsDisabledDiscovery(t *testing.T) {
	t.Parallel()

	client := New(WithDiscovery(false))
	_, err := client.ListAvailableModelsWithStatus(context.Background(), "healthy")
	require.EqualError(t, err, "model discovery is not enabled")
}

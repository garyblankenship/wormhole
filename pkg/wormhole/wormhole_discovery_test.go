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
		CacheTTL:        time.Hour,
		EnableFileCache: false,
		RefreshInterval: 0,
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
		CacheTTL:        time.Hour,
		EnableFileCache: false,
		RefreshInterval: 0,
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

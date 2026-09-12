package wormhole

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

type diagnosticModelFetcher struct {
	name   string
	models []*types.ModelInfo
	err    error
}

var errDiagnosticCatalog = errors.New("catalog unavailable")

func (f diagnosticModelFetcher) Name() string { return f.name }

func (f diagnosticModelFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.models, f.err
}

func TestSelectModelsWithDiagnostics(t *testing.T) {
	t.Parallel()

	healthy := diagnosticModelFetcher{name: "healthy", models: []*types.ModelInfo{{ID: "model", Provider: "healthy"}}}
	failed := diagnosticModelFetcher{name: "failed", err: errDiagnosticCatalog}
	empty := diagnosticModelFetcher{name: "empty", models: []*types.ModelInfo{}}

	tests := []struct {
		name       string
		config     discovery.DiscoveryConfig
		fetchers   []discovery.ModelFetcher
		providers  []string
		cancel     bool
		wantIDs    []string
		wantStale  []bool
		wantError  string
		wantErr    error
		wantModels bool
	}{
		{name: "success", fetchers: []discovery.ModelFetcher{healthy}, providers: []string{"healthy"}, wantIDs: []string{"model"}, wantStale: []bool{false}},
		{name: "partial failure", fetchers: []discovery.ModelFetcher{healthy, failed}, providers: []string{"healthy", "failed"}, wantIDs: []string{"model"}, wantStale: []bool{false, false}},
		{name: "all failed", fetchers: []discovery.ModelFetcher{failed}, providers: []string{"failed"}, wantStale: []bool{false}, wantError: "model selection failed: failed: failed to fetch models: catalog unavailable", wantErr: errDiagnosticCatalog},
		{name: "empty catalog", fetchers: []discovery.ModelFetcher{empty}, providers: []string{"empty"}, wantIDs: []string{}, wantStale: []bool{false}},
		{name: "stale fallback", config: discovery.DiscoveryConfig{OfflineMode: true}, providers: []string{"openai"}, wantStale: []bool{true}, wantModels: true},
		{name: "canceled context", fetchers: []discovery.ModelFetcher{healthy}, providers: []string{"healthy"}, cancel: true, wantStale: []bool{false}, wantError: "model selection failed: healthy: failed to fetch models: context canceled", wantErr: context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			config.DisableFileCache = true
			config.DisableBackgroundRefresh = true
			if config.CacheTTL == 0 {
				config.CacheTTL = time.Hour
			}
			client := New(WithDiscovery(false))
			client.discoveryService = discovery.NewDiscoveryService(config, tt.fetchers...)
			t.Cleanup(client.StopModelDiscovery)

			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			result, err := client.SelectModelsWithDiagnostics(ctx, ModelQuery{Providers: tt.providers})
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, result)
			gotIDs := make([]string, 0, len(result.Models))
			for _, model := range result.Models {
				gotIDs = append(gotIDs, model.ID)
			}
			if tt.wantModels {
				assert.NotEmpty(t, result.Models)
			} else if len(tt.wantIDs) == 0 {
				assert.Empty(t, gotIDs)
			} else {
				assert.Equal(t, tt.wantIDs, gotIDs)
			}
			require.Len(t, result.Diagnostics, len(tt.providers))
			for i, diagnostic := range result.Diagnostics {
				assert.Equal(t, tt.providers[i], diagnostic.Provider)
				assert.Equal(t, tt.wantStale[i], diagnostic.Stale)
			}
			if tt.wantErr != nil {
				assert.ErrorIs(t, result.Diagnostics[len(result.Diagnostics)-1].Err, tt.wantErr)
			}
		})
	}
}

func TestSelectModelsWithDiagnosticsKeepsErrorsAlongsidePartialMatches(t *testing.T) {
	t.Parallel()

	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}, diagnosticModelFetcher{name: "healthy", models: []*types.ModelInfo{{ID: "model", Provider: "healthy"}}}, diagnosticModelFetcher{name: "failed", err: errDiagnosticCatalog})
	t.Cleanup(client.StopModelDiscovery)

	result, err := client.SelectModelsWithDiagnostics(context.Background(), ModelQuery{Providers: []string{"healthy", "failed"}})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	require.Len(t, result.Diagnostics, 2)
	assert.ErrorIs(t, result.Diagnostics[1].Err, errDiagnosticCatalog)

	models, err := client.SelectModels(context.Background(), ModelQuery{Providers: []string{"healthy", "failed"}})
	require.NoError(t, err)
	require.Len(t, models, 1)
}

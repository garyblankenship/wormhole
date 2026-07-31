package wormhole

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type faultMatrixModelFetcher struct {
	name   string
	models []*types.ModelInfo
	err    error
}

func (f faultMatrixModelFetcher) Name() string { return f.name }

func (f faultMatrixModelFetcher) FetchModels(context.Context) ([]*types.ModelInfo, error) {
	return f.models, f.err
}

func TestModelCatalogFaultMatrixSelection(t *testing.T) {
	t.Parallel()

	healthy := faultMatrixModelFetcher{
		name: "healthy",
		models: []*types.ModelInfo{{
			ID:           "shared-model",
			Provider:     "healthy",
			Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityFunctions},
		}},
	}
	failed := faultMatrixModelFetcher{name: "failed", err: errors.New("catalog unavailable")}
	empty := faultMatrixModelFetcher{name: "empty", models: []*types.ModelInfo{}}

	tests := []struct {
		name      string
		providers []string
		want      []string
		wantCaps  []types.ModelCapability
		wantError string
	}{
		{
			name:      "mixed success omits failed provider",
			providers: []string{"healthy", "failed"},
			want:      []string{"healthy/shared-model"},
			wantCaps:  []types.ModelCapability{types.CapabilityText, types.CapabilityFunctions},
		},
		{
			name:      "explicit duplicate provider preserves duplicate rows",
			providers: []string{"healthy", "healthy"},
			want:      []string{"healthy/shared-model", "healthy/shared-model"},
		},
		{
			name:      "genuinely empty catalog is not a failure",
			providers: []string{"empty"},
			want:      []string{},
		},
		{
			name:      "all failed catalogs expose failure metadata",
			providers: []string{"failed"},
			wantError: "model selection failed: failed: failed to fetch models: catalog unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := New(WithDiscovery(false))
			client.discoveryService = discovery.NewDiscoveryService(
				discovery.DiscoveryConfig{
					DisableFileCache:         true,
					DisableBackgroundRefresh: true,
				},
				healthy,
				failed,
				empty,
			)
			t.Cleanup(client.StopModelDiscovery)

			models, err := client.SelectModels(context.Background(), ModelQuery{Providers: tt.providers})
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				assert.Nil(t, models)
				return
			}

			require.NoError(t, err)
			got := make([]string, 0, len(models))
			for _, model := range models {
				got = append(got, strings.Join([]string{model.Provider, model.ID}, "/"))
			}
			assert.Equal(t, tt.want, got)
			if len(tt.wantCaps) > 0 {
				require.Len(t, models, 1)
				assert.Equal(t, tt.wantCaps, models[0].Capabilities)
			}
		})
	}
}

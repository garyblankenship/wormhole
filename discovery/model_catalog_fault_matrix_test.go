package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogFaultMatrixDiscoveryStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fetcher   *switchingFetcher
		prime     bool
		expire    bool
		fail      bool
		wantIDs   []string
		wantCaps  []types.ModelCapability
		wantStale bool
		wantError string
	}{
		{
			name: "fresh catalog",
			fetcher: &switchingFetcher{
				name: "fresh",
				models: []*types.ModelInfo{{
					ID:           "chat-model",
					Provider:     "fresh",
					Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
				}},
			},
			wantIDs:  []string{"chat-model"},
			wantCaps: []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
		},
		{
			name: "stale catalog survives live failure",
			fetcher: &switchingFetcher{
				name: "stale",
				models: []*types.ModelInfo{{
					ID:           "cached-model",
					Provider:     "stale",
					Capabilities: []types.ModelCapability{types.CapabilityEmbeddings},
				}},
			},
			prime:     true,
			expire:    true,
			fail:      true,
			wantIDs:   []string{"cached-model"},
			wantCaps:  []types.ModelCapability{types.CapabilityEmbeddings},
			wantStale: true,
		},
		{
			name: "hard failure without cache",
			fetcher: &switchingFetcher{
				name: "failed",
			},
			fail:      true,
			wantError: "failed to fetch models: live endpoint unavailable",
		},
		{
			name: "genuinely empty catalog",
			fetcher: &switchingFetcher{
				name: "empty",
			},
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := NewDiscoveryService(DiscoveryConfig{
				CacheTTL:                 time.Hour,
				DisableFileCache:         true,
				DisableBackgroundRefresh: true,
			}, tt.fetcher)
			t.Cleanup(func() { require.NoError(t, service.Stop()) })

			if tt.prime {
				_, err := service.GetModelsWithStatus(context.Background(), tt.fetcher.name)
				require.NoError(t, err)
			}
			if tt.expire {
				service.cache.memoryMu.Lock()
				service.cache.memory[tt.fetcher.name].Timestamp = time.Now().Add(-2 * time.Hour)
				service.cache.memoryMu.Unlock()
			}
			tt.fetcher.fail.Store(tt.fail)

			result, err := service.GetModelsWithStatus(context.Background(), tt.fetcher.name)
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantStale, result.Stale)

			gotIDs := make([]string, 0, len(result.Models))
			for _, model := range result.Models {
				gotIDs = append(gotIDs, model.ID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)
			if len(tt.wantCaps) > 0 {
				require.Len(t, result.Models, 1)
				assert.Equal(t, tt.wantCaps, result.Models[0].Capabilities)
			}
		})
	}
}

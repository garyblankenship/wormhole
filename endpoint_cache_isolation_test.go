package wormhole

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

type endpointIsolationCache struct{ entries map[string]any }

func (c *endpointIsolationCache) Get(key string) (any, bool)             { v, ok := c.entries[key]; return v, ok }
func (c *endpointIsolationCache) Set(key string, v any, _ time.Duration) { c.entries[key] = v }
func (c *endpointIsolationCache) Delete(key string)                      { delete(c.entries, key) }
func (c *endpointIsolationCache) Clear()                                 { clear(c.entries) }
func (c *endpointIsolationCache) Close() error                           { return nil }

type endpointIsolationProvider struct {
	*types.BaseProvider
	endpoint string
	calls    map[string]int
}

func (p *endpointIsolationProvider) record(ctx context.Context, kind string) (*types.ResponseMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Value(middleware.CtxKeyProvider) != p.Name() || ctx.Value(middleware.CtxKeyMethod) != kind {
		return nil, fmt.Errorf("provider/method context changed")
	}
	p.calls[p.endpoint+"/"+kind]++
	return &types.ResponseMetadata{Provider: p.Name(), RequestID: p.endpoint + "/" + kind, StatusCode: 200}, nil
}

func (p *endpointIsolationProvider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	metadata, err := p.record(ctx, "embeddings")
	if err != nil {
		return nil, err
	}
	value := 1.25
	if strings.Contains(p.endpoint, "second.invalid") {
		value = 2.25
	}
	return &types.EmbeddingsResponse{Provider: p.Name(), Model: request.Model, ResponseMetadata: metadata,
		Embeddings: []types.Embedding{{Index: 0, Embedding: []float64{value}}}}, nil
}

func (p *endpointIsolationProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	metadata, err := p.record(ctx, "text")
	if err != nil {
		return nil, err
	}
	return &types.TextResponse{Model: request.Model, Text: p.endpoint, ResponseMetadata: metadata}, nil
}

func TestCacheNamespacesEndpointOverrides(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"embeddings", "text"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			first, second := "http://first.invalid/v1", "http://second.invalid/v1"
			cache := &endpointIsolationCache{entries: make(map[string]any)}
			calls := make(map[string]int)
			client := New(
				WithDefaultProvider("endpoint-fixture"),
				WithCustomProvider("endpoint-fixture", func(cfg types.ProviderConfig) (types.Provider, error) {
					return &endpointIsolationProvider{BaseProvider: types.NewBaseProvider("endpoint-fixture"), endpoint: cfg.BaseURL, calls: calls}, nil
				}),
				WithProviderConfig("endpoint-fixture", types.ProviderConfig{BaseURL: first}),
				WithDiscovery(false), WithModelValidation(false),
				WithProviderMiddleware(middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: time.Hour})),
			)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			for _, override := range []string{first, first, second, second, first, "", ""} {
				want := override
				if want == "" {
					want = first
				}
				if kind == "embeddings" {
					response, err := client.Embeddings().BaseURL(override).Model("fixture").Input("same input").Generate(t.Context())
					require.NoError(t, err)
					require.NotNil(t, response.ResponseMetadata)
					require.Equal(t, want+"/"+kind, response.ResponseMetadata.RequestID)
					value := 1.25
					if want == second {
						value = 2.25
					}
					require.Equal(t, []types.Embedding{{Index: 0, Embedding: []float64{value}}}, response.Embeddings)
				} else {
					response, err := client.Text().BaseURL(override).Model("fixture").Prompt("same prompt").Generate(t.Context())
					require.NoError(t, err)
					require.Equal(t, want, response.Text)
					require.NotNil(t, response.ResponseMetadata)
					require.Equal(t, want+"/"+kind, response.ResponseMetadata.RequestID)
				}
			}
			require.Equal(t, map[string]int{first + "/" + kind: 2, second + "/" + kind: 1}, calls)
			require.Len(t, cache.entries, 3)
			legacyEntries := 0
			for key := range cache.entries {
				require.NotContains(t, key, first)
				require.NotContains(t, key, second)
				require.True(t, strings.HasPrefix(key, kind+":endpoint-fixture:"))
				if strings.Count(key, ":") == 2 {
					legacyEntries++
				}
			}
			require.Equal(t, 1, legacyEntries)
		})
	}
}

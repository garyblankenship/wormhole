package fetchers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestConfiguredFetchersUseConfiguredEndpointAndHeaderPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		newFetcher func(types.ProviderConfig) interface {
			FetchModels(context.Context) ([]*types.ModelInfo, error)
		}
		customHeader  string
		defaultAuth   string
		defaultHeader string
	}{
		{
			name: "openai",
			newFetcher: func(config types.ProviderConfig) interface {
				FetchModels(context.Context) ([]*types.ModelInfo, error)
			} {
				return NewOpenAIFetcherWithConfig(config)
			},
			customHeader: "Authorization", defaultAuth: "Bearer test-key",
		},
		{
			name: "anthropic",
			newFetcher: func(config types.ProviderConfig) interface {
				FetchModels(context.Context) ([]*types.ModelInfo, error)
			} {
				return NewAnthropicFetcherWithConfig(config)
			},
			customHeader: "x-api-key", defaultHeader: "test-key",
		},
		{
			name: "openrouter",
			newFetcher: func(config types.ProviderConfig) interface {
				FetchModels(context.Context) ([]*types.ModelInfo, error)
			} {
				return NewOpenRouterFetcherWithConfig(config)
			},
			customHeader: "Authorization", defaultAuth: "Bearer test-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/models", r.URL.Path)
				assert.Equal(t, "custom-auth", r.Header.Get(tt.customHeader))
				if tt.defaultAuth != "" {
					assert.NotEqual(t, tt.defaultAuth, r.Header.Get("Authorization"))
				}
				if tt.defaultHeader != "" {
					assert.NotEqual(t, tt.defaultHeader, r.Header.Get("x-api-key"))
				}
				assert.Equal(t, "trace", r.Header.Get("X-Trace"))
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			}))
			t.Cleanup(server.Close)
			useTestHTTPClient(t, server.Client())

			_, err := tt.newFetcher(types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL + "/v1/",
				Headers: map[string]string{tt.customHeader: "custom-auth", "X-Trace": "trace"},
			}).FetchModels(context.Background())
			require.NoError(t, err)
		})
	}
}

func TestConfiguredFetchersNoAuthSuppressesGeneratedAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		newFetcher func(types.ProviderConfig) interface {
			FetchModels(context.Context) ([]*types.ModelInfo, error)
		}
	}{
		{name: "openai", newFetcher: func(config types.ProviderConfig) interface {
			FetchModels(context.Context) ([]*types.ModelInfo, error)
		} {
			return NewOpenAIFetcherWithConfig(config)
		}},
		{name: "anthropic", newFetcher: func(config types.ProviderConfig) interface {
			FetchModels(context.Context) ([]*types.ModelInfo, error)
		} {
			return NewAnthropicFetcherWithConfig(config)
		}},
		{name: "openrouter", newFetcher: func(config types.ProviderConfig) interface {
			FetchModels(context.Context) ([]*types.ModelInfo, error)
		} {
			return NewOpenRouterFetcherWithConfig(config)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("x-api-key"))
				assert.Empty(t, r.Header.Get("anthropic-version"))
				assert.Equal(t, "local", r.Header.Get("X-Local"))
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			}))
			t.Cleanup(server.Close)
			useTestHTTPClient(t, server.Client())

			_, err := tt.newFetcher(types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
				NoAuth:  true,
				Headers: map[string]string{"X-Local": "local"},
			}).FetchModels(context.Background())
			require.NoError(t, err)
		})
	}
}

func TestConfiguredFetcherCopiesHeaders(t *testing.T) {
	headers := map[string]string{"X-Test": "original"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "original", r.Header.Get("X-Test"))
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	t.Cleanup(server.Close)
	useTestHTTPClient(t, server.Client())

	fetcher := NewOpenAIFetcherWithConfig(types.ProviderConfig{
		APIKey: "test-key", BaseURL: server.URL, Headers: headers,
	})
	headers["X-Test"] = "changed"
	_, err := fetcher.FetchModels(context.Background())
	require.NoError(t, err)
}

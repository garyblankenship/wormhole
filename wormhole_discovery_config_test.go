package wormhole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

func configuredDiscoveryTestConfig() discovery.DiscoveryConfig {
	return discovery.DiscoveryConfig{
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	}
}

func TestConfiguredDiscoveryUsesGenerationEndpointAndHeaderPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(string) Option
		generation string
		authHeader string
		customAuth string
	}{
		{
			name: "openai",
			configure: func(baseURL string) Option {
				return WithOpenAI("test-key", types.ProviderConfig{
					BaseURL: baseURL,
					Headers: map[string]string{"Authorization": "custom-auth", "X-Route": "configured"},
				})
			},
			generation: "/chat/completions", authHeader: "Authorization", customAuth: "custom-auth",
		},
		{
			name: "anthropic",
			configure: func(baseURL string) Option {
				return WithAnthropic("test-key", types.ProviderConfig{
					BaseURL: baseURL,
					Headers: map[string]string{"X-API-Key": "custom-auth", "X-Route": "configured"},
				})
			},
			generation: "/messages", authHeader: "X-API-Key", customAuth: "custom-auth",
		},
		{
			name: "openrouter",
			configure: func(baseURL string) Option {
				return WithProfiledOpenAICompatible("openrouter", types.ProviderConfig{
					APIKey:  "test-key",
					BaseURL: baseURL,
					Headers: map[string]string{"Authorization": "custom-auth", "X-Route": "configured"},
				})
			},
			generation: "/chat/completions", authHeader: "Authorization", customAuth: "custom-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var generationSeen, discoverySeen bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "configured", r.Header.Get("X-Route"))
				assert.Equal(t, tt.customAuth, r.Header.Get(tt.authHeader))

				switch r.URL.Path {
				case "/v1/models":
					discoverySeen = true
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "test-model"}}})
				case "/v1" + tt.generation:
					generationSeen = true
					writeConfiguredGenerationResponse(w, tt.name)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			client := New(
				WithDefaultProvider(tt.name),
				tt.configure(server.URL+"/v1"),
				WithDiscoveryConfig(configuredDiscoveryTestConfig()),
				WithModelValidation(false),
			)
			t.Cleanup(func() { _ = client.Close() })

			_, err := client.Text().Using(tt.name).Model("test-model").Prompt("hello").Generate(context.Background())
			require.NoError(t, err)
			err = client.RefreshModelsWithContext(context.Background())
			require.NoError(t, err)
			_, err = client.ListAvailableModels(tt.name)
			require.NoError(t, err)
			assert.True(t, generationSeen, "generation did not reach configured endpoint")
			assert.True(t, discoverySeen, "discovery did not reach configured endpoint")
		})
	}
}

func TestConfiguredDiscoveryUsesEnvironmentBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURLEnv string
		keyEnv     string
		key        string
		authHeader string
	}{
		{name: "openai", baseURLEnv: "OPENAI_BASE_URL", keyEnv: "OPENAI_API_KEY", key: "sk-env-key", authHeader: "Authorization"},
		{name: "anthropic", baseURLEnv: "ANTHROPIC_BASE_URL", keyEnv: "ANTHROPIC_API_KEY", key: "sk-ant-env-key", authHeader: "X-API-Key"},
		{name: "openrouter", baseURLEnv: "OPENROUTER_BASE_URL", keyEnv: "OPENROUTER_API_KEY", key: "sk-or-env-key", authHeader: "Authorization"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var generationSeen, discoverySeen bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantAuth := "Bearer " + tt.key
				if tt.name == "anthropic" {
					wantAuth = tt.key
				}
				assert.Equal(t, wantAuth, r.Header.Get(tt.authHeader))

				switch r.URL.Path {
				case "/v1/models":
					discoverySeen = true
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "test-model"}}})
				case "/v1/chat/completions", "/v1/messages":
					generationSeen = true
					writeConfiguredGenerationResponse(w, tt.name)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv(tt.baseURLEnv, server.URL+"/v1")
			t.Setenv(tt.keyEnv, tt.key)

			client := New(
				WithDefaultProvider(tt.name),
				WithProviderFromEnv(tt.name),
				WithDiscoveryConfig(configuredDiscoveryTestConfig()),
				WithModelValidation(false),
			)
			t.Cleanup(func() { _ = client.Close() })

			_, err := client.Text().Using(tt.name).Model("test-model").Prompt("hello").Generate(context.Background())
			require.NoError(t, err)
			err = client.RefreshModelsWithContext(context.Background())
			require.NoError(t, err)
			_, err = client.ListAvailableModels(tt.name)
			require.NoError(t, err)
			assert.True(t, generationSeen, "generation did not reach environment endpoint")
			assert.True(t, discoverySeen, "discovery did not reach environment endpoint")
		})
	}
}

func TestConfiguredDiscoveryRegistersKeylessNoAuthFetchers(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("X-API-Key"))
				assert.Empty(t, r.Header.Get("Anthropic-Version"))
				assert.Equal(t, "local", r.Header.Get("X-Local"))
				assert.Equal(t, "/v1/models", r.URL.Path)
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "local-model"}}})
			}))
			t.Cleanup(server.Close)

			client := New(
				WithProviderConfig(provider, types.ProviderConfig{
					BaseURL: server.URL + "/v1",
					NoAuth:  true,
					Headers: map[string]string{"X-Local": "local"},
				}),
				WithDiscoveryConfig(configuredDiscoveryTestConfig()),
			)
			t.Cleanup(func() { _ = client.Close() })

			require.Contains(t, client.ModelDiscoveryProviders(), provider)
			err := client.RefreshModelsWithContext(context.Background())
			require.NoError(t, err)
			_, err = client.ListAvailableModels(provider)
			require.NoError(t, err)
		})
	}
}

func writeConfiguredGenerationResponse(w http.ResponseWriter, provider string) {
	w.Header().Set("Content-Type", "application/json")
	if provider == "anthropic" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg-test", "type": "message", "model": "test-model",
			"content":     []any{map[string]any{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-test", "object": "chat.completion", "created": 1, "model": "test-model",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
	})
}

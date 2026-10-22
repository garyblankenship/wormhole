package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/discovery"
)

func TestExplicitLocalProviderGeneration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("loopback generation")
	}
	for _, tt := range []struct {
		name, provider, baseURLEnv, apiKeyEnv, apiKey string
	}{
		{name: "no key", provider: "lmstudio", baseURLEnv: "LMSTUDIO_BASE_URL"},
		{name: "authenticated", provider: "vllm", baseURLEnv: "VLLM_BASE_URL", apiKeyEnv: "VLLM_API_KEY", apiKey: "test-backend-key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wantAuth := ""
			if tt.apiKey != "" {
				wantAuth = "Bearer " + tt.apiKey
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("unexpected backend authorization")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if r.URL.Path != "/v1/chat/completions" {
					t.Error("unexpected backend path")
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(map[string]any{
					"id": "fixture", "choices": []any{map[string]any{
						"index": 0, "message": map[string]any{"role": "assistant", "content": "local fixture"}, "finish_reason": "stop",
					}},
				})
				if err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(upstream.Close)
			env := map[string]string{tt.baseURLEnv: upstream.URL + "/v1", "WORMHOLE_API_KEY": "test-ingress-key"}
			if tt.apiKeyEnv != "" {
				env[tt.apiKeyEnv] = tt.apiKey
			}
			opts := explicitLocalProviderOptions(func(key string) string { return env[key] })
			opts = append(opts, wormhole.WithDefaultProvider(tt.provider), wormhole.WithDiscovery(false),
				wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{DisableFileCache: true, DisableBackgroundRefresh: true}))
			client := wormhole.New(opts...)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			response, err := client.Text().Model("fixture-model").Prompt("hello").Generate(t.Context())
			require.NoError(t, err)
			require.Equal(t, "local fixture", response.Text)
		})
	}
}

func TestExplicitLocalProviderOptions(t *testing.T) {
	t.Parallel()
	type backend struct {
		url, key string
	}
	tests := []struct {
		name string
		env  map[string]string
		want map[string]backend
	}{
		{name: "unset remains disabled"},
		{name: "explicit local", env: map[string]string{"VLLM_BASE_URL": "http://127.0.0.1:8000/v1"}, want: map[string]backend{"vllm": {url: "http://127.0.0.1:8000/v1"}}},
		{name: "other local", env: map[string]string{"LMSTUDIO_BASE_URL": "http://localhost:1234/v1"}, want: map[string]backend{"lmstudio": {url: "http://localhost:1234/v1"}}},
		{name: "authenticated vllm", env: map[string]string{"VLLM_BASE_URL": "http://127.0.0.1:8000/v1", "VLLM_API_KEY": "test-vllm-key"}, want: map[string]backend{"vllm": {url: "http://127.0.0.1:8000/v1", key: "test-vllm-key"}}},
		{name: "authenticated lmstudio", env: map[string]string{"LMSTUDIO_BASE_URL": "http://localhost:1234/v1", "LMSTUDIO_API_KEY": "test-lmstudio-key"}, want: map[string]backend{"lmstudio": {url: "http://localhost:1234/v1", key: "test-lmstudio-key"}}},
		{name: "authenticated ollama compatible", env: map[string]string{"OLLAMA_OPENAI_BASE_URL": "http://localhost:11434/v1", "OLLAMA_OPENAI_API_KEY": "test-ollama-key"}, want: map[string]backend{"ollama-openai": {url: "http://localhost:11434/v1", key: "test-ollama-key"}}},
		{name: "key alone does not opt in", env: map[string]string{"VLLM_API_KEY": "test-vllm-key"}},
		{name: "separate credentials", env: map[string]string{
			"VLLM_BASE_URL": "http://127.0.0.1:8000/v1", "VLLM_API_KEY": "test-vllm-key",
			"LMSTUDIO_BASE_URL": "http://localhost:1234/v1", "LMSTUDIO_API_KEY": "test-lmstudio-key",
			"WORMHOLE_API_KEY": "test-ingress-key",
		}, want: map[string]backend{
			"vllm":     {url: "http://127.0.0.1:8000/v1", key: "test-vllm-key"},
			"lmstudio": {url: "http://localhost:1234/v1", key: "test-lmstudio-key"},
		}},
		{name: "remote URL does not opt in", env: map[string]string{"ZAI_BASE_URL": "https://example.invalid/v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &wormhole.Config{}
			for _, option := range explicitLocalProviderOptions(func(key string) string { return tt.env[key] }) {
				option(cfg)
			}
			require.Len(t, cfg.Providers, len(tt.want))
			for name, expected := range tt.want {
				require.Equal(t, expected.url, cfg.Providers[name].BaseURL)
				require.Equal(t, expected.key, cfg.Providers[name].APIKey)
				require.Equal(t, expected.key == "", cfg.Providers[name].NoAuth)
				require.True(t, cfg.Providers[name].DynamicModels)
			}
		})
	}
}

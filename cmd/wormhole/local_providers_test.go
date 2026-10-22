package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/stretchr/testify/require"
)

func TestExplicitLocalProviderGeneration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("loopback generation")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected auth/path: %q %s", r.Header.Get("Authorization"), r.URL.Path)
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
	opts := explicitLocalProviderOptions(func(key string) string {
		if key == "LMSTUDIO_BASE_URL" {
			return upstream.URL + "/v1"
		}
		return ""
	})
	opts = append(opts, wormhole.WithDefaultProvider("lmstudio"), wormhole.WithDiscovery(false),
		wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{DisableFileCache: true, DisableBackgroundRefresh: true}))
	client := wormhole.New(opts...)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	response, err := client.Text().Model("fixture-model").Prompt("hello").Generate(t.Context())
	require.NoError(t, err)
	require.Equal(t, "local fixture", response.Text)
}

func TestExplicitLocalProviderOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{name: "unset remains disabled"},
		{name: "explicit local", env: map[string]string{"VLLM_BASE_URL": "http://127.0.0.1:8000/v1"}, want: map[string]string{"vllm": "http://127.0.0.1:8000/v1"}},
		{name: "other local", env: map[string]string{"LMSTUDIO_BASE_URL": "http://localhost:1234/v1"}, want: map[string]string{"lmstudio": "http://localhost:1234/v1"}},
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
			for name, baseURL := range tt.want {
				require.Equal(t, baseURL, cfg.Providers[name].BaseURL)
				require.Empty(t, cfg.Providers[name].APIKey)
				require.True(t, cfg.Providers[name].NoAuth)
				require.True(t, cfg.Providers[name].DynamicModels)
			}
		})
	}
}

package fetchers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTestHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()
	defaultClient = client
	defaultClientOnce = sync.Once{}
	defaultClientOnce.Do(func() {})
	t.Cleanup(func() {
		defaultClient = nil
		defaultClientOnce = sync.Once{}
	})
}

func hasCapability(model *types.ModelInfo, capability types.ModelCapability) bool {
	for _, existing := range model.Capabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

func TestAnthropicFetcher(t *testing.T) {
	var sawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		sawHeaders = r.Header.Get("x-api-key") == "anthropic-key" &&
			r.Header.Get("anthropic-version") == "2023-06-01"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-4-5", "display_name": "", "created_at": "2026-01-01T00:00:00Z", "type": "model"},
				{"id": "claude-custom", "display_name": "Custom Claude", "created_at": "2026-01-01T00:00:00Z", "type": "model"},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	fetcher := NewAnthropicFetcher("anthropic-key")
	fetcher.baseURL = server.URL

	models, err := fetcher.FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.True(t, sawHeaders)
	assert.Equal(t, "anthropic", fetcher.Name())
	assert.Equal(t, "Claude Sonnet 4.5", models[0].Name)
	assert.Equal(t, "Custom Claude", models[1].Name)
	assert.Equal(t, 200000, models[0].MaxTokens)
	assert.True(t, hasCapability(models[0], types.CapabilityVision))
}

func TestAnthropicFetcherRequiresAPIKey(t *testing.T) {
	_, err := NewAnthropicFetcher("").FetchModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestOpenRouterFetcher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":             "openai/gpt-5",
					"name":           "GPT-5",
					"context_length": 400000,
					"architecture":   map[string]any{"modality": "text->text"},
					"moderation":     map[string]any{"illicit": false},
				},
				{
					"id":             "google/gemini-vision",
					"name":           "Gemini Vision",
					"context_length": 100000,
					"architecture":   map[string]any{"modality": "text+image->text"},
					"moderation":     map[string]any{"illicit": false},
				},
				{
					"id":             "bad/model",
					"name":           "Filtered",
					"context_length": 1,
					"architecture":   map[string]any{"modality": "text->text"},
					"moderation":     map[string]any{"illicit": true},
				},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	fetcher := NewOpenRouterFetcher()
	fetcher.baseURL = server.URL

	models, err := fetcher.FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, providerOpenRouter, fetcher.Name())
	assert.Equal(t, "openai", models[0].Provider)
	assert.Equal(t, 400000, models[0].MaxTokens)
	assert.True(t, hasCapability(models[0], types.CapabilityChat))
	assert.Equal(t, "google", models[1].Provider)
	assert.True(t, hasCapability(models[1], types.CapabilityVision))
}

func TestOllamaFetcher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"name":    "llava:latest",
					"size":    1,
					"digest":  "abc",
					"details": map[string]any{"parameter_size": "13B"},
				},
				{
					"name":    "nomic-embed-text:latest",
					"size":    1,
					"digest":  "def",
					"details": map[string]any{"parameter_size": "7B"},
				},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	models, err := NewOllamaFetcher(server.URL).FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "llava:latest", models[0].ID)
	assert.Equal(t, "Llava", models[0].Name)
	assert.Equal(t, 16384, models[0].MaxTokens)
	assert.True(t, hasCapability(models[0], types.CapabilityVision))
	assert.Equal(t, "Nomic-embed-text", models[1].Name)
	assert.True(t, hasCapability(models[1], types.CapabilityEmbeddings))
}

func TestGeminiFetcher(t *testing.T) {
	var sawKey bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		sawKey = r.URL.Query().Get("key") == "gemini-key"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"name":                       "models/gemini-2.5-flash",
					"displayName":                "Gemini 2.5 Flash",
					"inputTokenLimit":            1000000,
					"supportedGenerationMethods": []string{"generateContent"},
				},
				{
					"name":                       "models/text-embedding-004",
					"supportedGenerationMethods": []string{"embedContent"},
				},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	models, err := NewGeminiFetcher(server.URL, "gemini-key").FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.True(t, sawKey)
	assert.Equal(t, "gemini-2.5-flash", models[0].ID)
	assert.Equal(t, "gemini", models[0].Provider)
	assert.True(t, hasCapability(models[0], types.CapabilityStream))
	assert.True(t, hasCapability(models[1], types.CapabilityEmbeddings))
}

func TestGeminiFetcherRequiresAPIKey(t *testing.T) {
	_, err := NewGeminiFetcher("", "").FetchModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestGeminiFetcherMalformedJSONAndStatusErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{`))
			},
			want: "unexpected EOF",
		},
		{
			name: "non-2xx",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			useTestHTTPClient(t, server.Client())

			_, err := NewGeminiFetcher(server.URL, "key").FetchModels(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestGeminiFetcherContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewGeminiFetcher("https://example.test", "key").FetchModels(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestOpenAICompatibleFetcher(t *testing.T) {
	var sawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		sawHeaders = r.Header.Get("Authorization") == "Bearer compatible-key" &&
			r.Header.Get("X-Provider") == "custom"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-compatible"},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	fetcher := NewOpenAICompatibleFetcher("compatible", server.URL, "compatible-key", map[string]string{"X-Provider": "custom"})
	models, err := fetcher.FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.True(t, sawHeaders)
	assert.Equal(t, "compatible", fetcher.Name())
	assert.Equal(t, "compatible", models[0].Provider)
	assert.True(t, hasCapability(models[0], types.CapabilityText))
}

func TestOpenAICompatibleFetcherStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	_, err := NewOpenAICompatibleFetcher("compatible", server.URL, "", nil).FetchModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 502")
}

func TestFetchJSONReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	req, err := newGetRequest(context.Background(), server.URL)
	require.NoError(t, err)

	var out map[string]any
	err = fetchJSON(req, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 418")
}

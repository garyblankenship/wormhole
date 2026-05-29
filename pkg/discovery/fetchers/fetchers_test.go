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

func TestOpenAIFetcher(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "gpt-5-mini", "object": "model", "created": 1, "owned_by": "openai"},
				{"id": "text-embedding-3-small", "object": "model", "created": 1, "owned_by": "openai"},
				{"id": "dall-e-3", "object": "model", "created": 1, "owned_by": "openai"},
			},
		})
	}))
	defer server.Close()
	useTestHTTPClient(t, server.Client())

	fetcher := NewOpenAIFetcher("test-key")
	fetcher.baseURL = server.URL

	models, err := fetcher.FetchModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 3)
	assert.True(t, sawAuth)
	assert.Equal(t, "openai", fetcher.Name())
	assert.Equal(t, "Gpt 5 Mini", models[0].Name)
	assert.True(t, hasCapability(models[0], types.CapabilityChat))
	assert.True(t, hasCapability(models[1], types.CapabilityEmbeddings))
	assert.True(t, hasCapability(models[2], types.CapabilityImages))
}

func TestOpenAIFetcherRequiresAPIKey(t *testing.T) {
	_, err := NewOpenAIFetcher("").FetchModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
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

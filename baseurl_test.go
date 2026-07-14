package wormhole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

type baseURLTestServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newBaseURLTestServer(t *testing.T) *baseURLTestServer {
	t.Helper()

	ts := &baseURLTestServer{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.hits.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"))

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-baseurl",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "test-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"name":"baseurl"}`,
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			})
		case "/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{{
					"object":    "embedding",
					"index":     0,
					"embedding": []float64{0.1, 0.2, 0.3},
				}},
				"model": "test-embedding-model",
				"usage": map[string]any{
					"prompt_tokens": 1,
					"total_tokens":  1,
				},
			})
		case "/images/generations":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "gpt-image-1", req["model"])
			assert.Equal(t, "draw", req["prompt"])
			_ = json.NewEncoder(w).Encode(map[string]any{
				"created": 1699999999,
				"data": []map[string]any{{
					"url": "https://example.test/image.png",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestBaseURLFunctionality(t *testing.T) {
	t.Parallel()
	defaultServer := newBaseURLTestServer(t)
	overrideServer := newBaseURLTestServer(t)
	client := New(
		WithDefaultProvider("openai"),
		WithOpenAICompatible("openai", defaultServer.URL, types.ProviderConfig{
			APIKey: "test-key",
		}),
	)

	ctx := context.Background()

	t.Run("BaseURL changes target endpoint", func(t *testing.T) {
		defaultHits := defaultServer.hits.Load()
		overrideHits := overrideServer.hits.Load()
		resp, err := client.Text().
			BaseURL(overrideServer.URL).
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		require.NoError(t, err)
		assert.Equal(t, `{"name":"baseurl"}`, resp.Text)
		assert.Equal(t, defaultHits, defaultServer.hits.Load())
		assert.Equal(t, overrideHits+1, overrideServer.hits.Load())
	})

	t.Run("Without BaseURL uses default provider", func(t *testing.T) {
		defaultHits := defaultServer.hits.Load()
		resp, err := client.Text().
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		require.NoError(t, err)
		assert.Equal(t, `{"name":"baseurl"}`, resp.Text)
		assert.Equal(t, defaultHits+1, defaultServer.hits.Load())
	})

	t.Run("BaseURL works with structured requests", func(t *testing.T) {
		overrideHits := overrideServer.hits.Load()
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		}

		resp, err := client.Structured().
			BaseURL(overrideServer.URL).
			Model("test-model").
			Prompt("test").
			Schema(schema).
			Mode(types.StructuredModeJSON).
			Generate(ctx)

		require.NoError(t, err)
		assert.NotNil(t, resp.Data)
		assert.Equal(t, overrideHits+1, overrideServer.hits.Load())
	})

	t.Run("BaseURL works with embeddings", func(t *testing.T) {
		overrideHits := overrideServer.hits.Load()
		resp, err := client.Embeddings().
			BaseURL(overrideServer.URL).
			Model("test-embedding-model").
			Input("test text").
			Generate(ctx)

		require.NoError(t, err)
		require.Len(t, resp.Embeddings, 1)
		assert.InEpsilonSlice(t, []float64{0.1, 0.2, 0.3}, resp.Embeddings[0].Embedding, 0.000001)
		assert.Equal(t, overrideHits+1, overrideServer.hits.Load())
	})

	t.Run("BaseURL works with image generation", func(t *testing.T) {
		overrideHits := overrideServer.hits.Load()
		resp, err := client.Image().
			BaseURL(overrideServer.URL).
			Model("gpt-image-1").
			Prompt("draw").
			Generate(ctx)

		require.NoError(t, err)
		require.Len(t, resp.Images, 1)
		assert.Equal(t, "https://example.test/image.png", resp.Images[0].URL)
		assert.Equal(t, overrideHits+1, overrideServer.hits.Load())
	})
}

func TestBaseURLValidation(t *testing.T) {
	t.Parallel()
	defaultServer := newBaseURLTestServer(t)
	client := New(
		WithDefaultProvider("openai"),
		WithOpenAICompatible("openai", defaultServer.URL, types.ProviderConfig{
			APIKey: "test-key",
		}),
	)
	ctx := context.Background()

	t.Run("Empty BaseURL uses default", func(t *testing.T) {
		t.Parallel()
		defaultHits := defaultServer.hits.Load()
		resp, err := client.Text().
			BaseURL("").
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		require.NoError(t, err)
		assert.Equal(t, `{"name":"baseurl"}`, resp.Text)
		assert.Equal(t, defaultHits+1, defaultServer.hits.Load())
	})

	t.Run("Invalid BaseURL fails appropriately", func(t *testing.T) {
		t.Parallel()
		_, err := client.Text().
			BaseURL("://invalid").
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		assert.Error(t, err)
	})
}

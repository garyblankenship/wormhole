package fetchers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestOpenAIFetcher(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "gpt-5-mini", "object": "model", "created": 1735689600, "owned_by": "system"},
				{"id": "text-embedding-3-small", "object": "model", "created": 1735776000, "owned_by": "openai"},
				{"id": "dall-e-3", "object": "model", "created": 1735862400, "owned_by": "openai-internal"},
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
	assert.Equal(t, int64(1735689600), models[0].Created)
	assert.Equal(t, "system", models[0].OwnedBy)
	assert.True(t, hasCapability(models[0], types.CapabilityChat))
	assert.True(t, hasCapability(models[1], types.CapabilityEmbeddings))
	assert.True(t, hasCapability(models[2], types.CapabilityImages))
}

func TestOpenAIFetcherRequiresAPIKey(t *testing.T) {
	_, err := NewOpenAIFetcher("").FetchModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

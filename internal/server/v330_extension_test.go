package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"

	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330ProxyExtensionValidation(t *testing.T) {
	for _, extension := range []any{map[string]any{"unknown": true}, map[string]any{"include": []string{"credentials"}}, map[string]any{"structured_mode": "strict"}, map[string]any{"cache_prewarm": true}, map[string]any{"structured_mode": "tools"}} {
		proxy := newKwargsTestProxy(t, wormhole.WithDiscovery(false))
		body, err := json.Marshal(map[string]any{"model": "openai/stub", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "wormhole": extension})
		require.NoError(t, err)
		response := performRequest(proxy, http.MethodPost, "/v1/chat/completions", string(body))
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func TestV330ProxyMetadataAllowlist(t *testing.T) {
	w := httptest.NewRecorder()
	metadata := &types.ResponseMetadata{RequestID: "request-123", Headers: map[string][]string{"X-Request-ID": {"request-123"}, "X-Ratelimit-Remaining-Tokens": {"90"}, "X-SDK-Only": {"private"}, "Set-Cookie": {"secret"}}}
	forwardResponseMetadata(w, metadata)
	require.Equal(t, "request-123", w.Header().Get("X-Wormhole-Upstream-Request-ID"))
	require.Equal(t, "90", w.Header().Get("X-Ratelimit-Remaining-Tokens"))
	require.Empty(t, w.Header().Get("X-SDK-Only"))
	require.Empty(t, w.Header().Get("Set-Cookie"))
	require.Nil(t, extensionSnapshot(nil, metadata, nil, nil, nil, nil))
	snapshot := extensionSnapshot(&WormholeRequest{Include: []string{"response_metadata", "sources"}}, metadata, nil, nil, nil, nil)
	filtered := snapshot["response_metadata"].(*types.ResponseMetadata)
	require.Equal(t, metadata.RequestID, filtered.RequestID)
	require.NotContains(t, filtered.Headers, "X-SDK-Only")
	require.NotContains(t, filtered.Headers, "Set-Cookie")
	require.Contains(t, metadata.Headers, "X-SDK-Only")
	require.Contains(t, snapshot, "sources")
	require.NotContains(t, snapshot, "warnings")
}

func TestV330ProxyFileReferenceBinding(t *testing.T) {
	encoded, err := json.Marshal([]map[string]any{{"type": "file", "file": map[string]string{"file_id": "file-123"}}, {"type": "wormhole_file", "provider": "anthropic", "kind": "document", "id": "file-456"}})
	require.NoError(t, err)
	var content ChatMessageContent
	require.NoError(t, json.Unmarshal(encoded, &content))
	messages, err := parseChatMessages([]ChatCompletionRequestMessage{{Role: "user", Content: content}})
	require.NoError(t, err)
	bindFileReferences(messages, "openai")
	require.Equal(t, "openai", content.Media[0].(*types.FileReferenceMedia).Provider)
	require.Equal(t, "anthropic", content.Media[1].(*types.FileReferenceMedia).Provider)
}

func TestV330ProxyOptInAndTerminalSnapshots(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, include := range []bool{false, true} {
			metadata := &types.ResponseMetadata{Provider: "openai", RequestID: "upstream-id", StatusCode: 200, Headers: map[string][]string{"X-Request-ID": {"upstream-id"}, "X-Private": {"sdk-only"}}}
			usage := &types.Usage{PromptTokens: 7, TotalTokens: 7, CacheReadTokens: 4, CacheWriteTokens: 3, CacheWriteTokensByTTL: map[string]int{"5m": 3}}
			finish := types.FinishReasonStop
			source := types.Source{ID: "source-1", URL: "https://example.com"}
			provider := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{ID: "test", Text: "ok", FinishReason: finish, ResponseMetadata: metadata, Usage: usage, Sources: []types.Source{source}}).WithStreamChunks([]types.TextChunk{{Text: "ok", ResponseMetadata: metadata}, {FinishReason: &finish, ResponseMetadata: metadata, Sources: []types.Source{source}}, {Usage: usage}})
			proxy := newKwargsTestProxy(t, wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), wormhole.WithProviderConfig("openai", types.ProviderConfig{}), wormhole.WithDefaultProvider("openai"), wormhole.WithDiscovery(false))
			request := map[string]any{"model": "stub", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "stream": stream}
			if include {
				request["wormhole"] = map[string]any{"include": []string{"response_metadata", "usage_details", "sources"}}
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			response := performRequest(proxy, http.MethodPost, "/v1/chat/completions", string(body))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, "upstream-id", response.Header().Get("X-Wormhole-Upstream-Request-ID"))
			require.Empty(t, response.Header().Get("X-Private"))
			if include {
				require.Contains(t, response.Body.String(), `"wormhole":`)
				require.Contains(t, response.Body.String(), `"request_id":"upstream-id"`)
				require.NotContains(t, response.Body.String(), "sdk-only")
				require.Contains(t, response.Body.String(), `"cache_write_tokens_by_ttl":{"5m":3}`)
			} else {
				require.NotContains(t, response.Body.String(), `"wormhole":`)
			}
			if stream {
				require.Contains(t, response.Body.String(), "data: [DONE]")
			}
		}
	}
}

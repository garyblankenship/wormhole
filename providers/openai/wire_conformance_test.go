// Wire-conformance replay tests: feed real-shaped provider SSE payloads (testdata/*.sse) through the actual stream path and assert the mapped wormhole types. Guards wire-protocol regressions that synthetic struct tests miss.
package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/internal/testutil"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestOpenAIStreamWireConformance(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/stream_tool_call.sse")
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		_, _ = w.Write(data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	provider := New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-test"},
		Messages: []types.Message{
			types.NewUserMessage("What's the weather right now?"),
		},
	}

	stream, err := provider.Stream(context.Background(), request)
	require.NoError(t, err)

	var chunks []types.TextChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}

	merged := testutil.MergeTextChunks(chunks)
	assert.Equal(t, "Let me check.", merged.Text)

	var toolCallChunk types.TextChunk
	var toolCallChunkCount int
	for _, chunk := range chunks {
		if len(chunk.ToolCalls) > 0 {
			toolCallChunk = chunk
			toolCallChunkCount++
		}
	}

	assert.Equal(t, 1, toolCallChunkCount)
	require.Len(t, toolCallChunk.ToolCalls, 1)
	assert.Equal(t, "call_abc", toolCallChunk.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", toolCallChunk.ToolCalls[0].Name)
	city, ok := toolCallChunk.ToolCalls[0].Arguments["city"].(string)
	require.True(t, ok)
	assert.Equal(t, "SF", city)

	require.NotNil(t, toolCallChunk.FinishReason)
	expectedReason := provider.mapFinishReason("tool_calls")
	assert.Equal(t, expectedReason, *toolCallChunk.FinishReason)

	require.NotNil(t, merged.Usage)
	assert.Equal(t, 12, merged.Usage.PromptTokens)
	assert.Equal(t, 8, merged.Usage.CompletionTokens)
	assert.Equal(t, 20, merged.Usage.TotalTokens)
	assert.Equal(t, 3, merged.Usage.CacheReadTokens)
}

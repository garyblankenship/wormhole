package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/internal/testutil"
	"github.com/garyblankenship/wormhole/v2/providers/anthropic"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestAnthropicStreamToolCallAccumulation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		chunks := []string{
			"event: message_start\n" +
				"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}" + "\n\n",
			"event: content_block_start\n" +
				"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_abc\",\"name\":\"get_weather\"}}" + "\n\n",
			"event: content_block_delta\n" +
				fmt.Sprintf(
					"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%s}}",
					strconv.Quote(`{"location":`),
				) + "\n\n",
			"event: content_block_delta\n" +
				fmt.Sprintf(
					"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%s}}",
					strconv.Quote(`"Paris"}`),
				) + "\n\n",
			"event: content_block_stop\n" +
				"data: {\"type\":\"content_block_stop\",\"index\":0}" + "\n\n",
			"event: message_delta\n" +
				"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"usage\":{\"output_tokens\":5}}}" + "\n\n",
			"event: message_stop\n" +
				"data: {\"type\":\"message_stop\"}" + "\n\n",
		}

		for _, chunk := range chunks {
			_, _ = fmt.Fprint(w, chunk)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	provider := anthropic.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-sonnet-4-5"},
		Messages: []types.Message{
			types.NewUserMessage("Hello"),
		},
	}

	stream, err := provider.Stream(context.Background(), *request)
	require.NoError(t, err)

	var chunks []types.StreamChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}
	require.NotEmpty(t, chunks)

	var terminalChunk types.StreamChunk
	foundTerminal := false
	for _, chunk := range chunks {
		if chunk.IsDone() && len(chunk.ToolCalls) > 0 {
			terminalChunk = chunk
			foundTerminal = true
		}
	}
	require.True(t, foundTerminal)

	toolCalls := terminalChunk.ToolCalls
	require.Len(t, toolCalls, 1)
	tc := toolCalls[0]
	assert.Equal(t, "toolu_abc", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, "Paris", tc.Arguments["location"])

	merged := testutil.MergeTextChunks(chunks)
	require.Len(t, merged.ToolCalls, 1)
	assert.Equal(t, "toolu_abc", merged.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", merged.ToolCalls[0].Name)
	assert.Equal(t, "Paris", merged.ToolCalls[0].Arguments["location"])
}

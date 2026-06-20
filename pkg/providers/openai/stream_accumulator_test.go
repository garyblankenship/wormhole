package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamToolCallAccumulation(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\",\"unit\":\"celsius\"}"}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n",
			`data: [DONE]` + "\n\n",
		}

		for _, chunk := range chunks {
			_, _ = io.WriteString(w, chunk)
		}
	})

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	}

	provider.streamingTransformer = nil
	stream, err := provider.Stream(context.Background(), request)
	require.NoError(t, err)

	var chunks []types.TextChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}
	require.NotEmpty(t, chunks)

	var terminalChunk types.TextChunk
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
	assert.Equal(t, "call_abc", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, "Paris", tc.Arguments["location"])
	assert.Equal(t, "celsius", tc.Arguments["unit"])

	merged := utils.MergeTextChunks(chunks)
	require.Len(t, merged.ToolCalls, 1)
	assert.Equal(t, "call_abc", merged.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", merged.ToolCalls[0].Name)
	assert.Equal(t, "Paris", merged.ToolCalls[0].Arguments["location"])
	assert.Equal(t, "celsius", merged.ToolCalls[0].Arguments["unit"])
}

func TestOpenAIStreamToolCallAccumulationDefaultTransformer(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\",\"unit\":\"celsius\"}"}}]}}]}` + "\n\n",
			`data: {"id":"c1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n",
			`data: [DONE]` + "\n\n",
		}

		for _, chunk := range chunks {
			_, _ = io.WriteString(w, chunk)
		}
	})

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	}

	stream, err := provider.Stream(context.Background(), request)
	require.NoError(t, err)

	var chunks []types.TextChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}
	require.NotEmpty(t, chunks)

	var terminalChunk types.TextChunk
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
	assert.Equal(t, "call_abc", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, "Paris", tc.Arguments["location"])
	assert.Equal(t, "celsius", tc.Arguments["unit"])

	merged := utils.MergeTextChunks(chunks)
	require.Len(t, merged.ToolCalls, 1)
	assert.Equal(t, "call_abc", merged.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", merged.ToolCalls[0].Name)
	assert.Equal(t, "Paris", merged.ToolCalls[0].Arguments["location"])
	assert.Equal(t, "celsius", merged.ToolCalls[0].Arguments["unit"])
}

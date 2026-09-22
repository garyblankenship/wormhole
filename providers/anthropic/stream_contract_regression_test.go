package anthropic_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers/anthropic"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicStreamMalformedToolArgumentsRemainNonExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`data: {"type":"message_start","message":{"id":"msg-1","model":"claude-test","usage":{"input_tokens":1}}}` + "\n\n",
			`data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tool-1","name":"lookup"}}` + "\n\n",
			fmt.Sprintf(`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":%s}}`, strconv.Quote(`{"broken": }`)) + "\n\n",
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","usage":{"output_tokens":1}}}` + "\n\n",
			`data: {"type":"message_stop"}` + "\n\n",
		} {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()

	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	stream, err := provider.Stream(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test"},
		Messages:    []types.Message{types.NewUserMessage("lookup")},
	})
	require.NoError(t, err)

	var terminal *types.StreamChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		if chunk.IsDone() {
			copy := chunk
			terminal = &copy
		}
	}
	require.NotNil(t, terminal)
	require.Len(t, terminal.ToolCalls, 1)
	call := terminal.ToolCalls[0]
	assert.True(t, call.ArgsInvalid)
	assert.NotEmpty(t, call.ArgsParseError)
	assert.Nil(t, call.Arguments)
}

func TestAnthropicStreamToolCallsEmittedOnceAfterTrailingError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`data: {"type":"message_start","message":{"id":"msg-1","model":"claude-test","usage":{"input_tokens":1}}}` + "\n\n",
			`data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tool-1","name":"lookup"}}` + "\n\n",
			`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}` + "\n\n",
			`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"\"one\"}"}}` + "\n\n",
			`data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tool-2","name":"lookup"}}` + "\n\n",
			`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}` + "\n\n",
			`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"\"two\"}"}}` + "\n\n",
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","usage":{"output_tokens":1}}}` + "\n\n",
			`data: {"type":"error","error":{"type":"api_error","message":"after terminal"}}` + "\n\n",
		} {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()

	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	stream, err := provider.Stream(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude-test"}, Messages: []types.Message{types.NewUserMessage("lookup")}})
	require.NoError(t, err)
	var calls []types.ToolCall
	var sawError bool
	for chunk := range stream {
		if chunk.Error != nil {
			sawError = true
		}
		calls = append(calls, chunk.ToolCalls...)
	}
	require.True(t, sawError)
	require.Len(t, calls, 2)
	assert.Equal(t, "tool-1", calls[0].ID)
	assert.Equal(t, "tool-2", calls[1].ID)
	assert.Equal(t, `{"q":"one"}`, calls[0].Function.Arguments)
	assert.Equal(t, `{"q":"two"}`, calls[1].Function.Arguments)
}

func TestAnthropicStreamUsageMergesMessageStartAndDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`data: {"type":"message_start","message":{"id":"msg-1","model":"claude-test","usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":3}}}` + "\n\n",
			`data: {"type":"message_delta","delta":{},"usage":{"output_tokens":3}}` + "\n\n",
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n",
			`data: {"type":"message_stop"}` + "\n\n",
		} {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()
	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	stream, err := provider.Stream(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude-test"}, Messages: []types.Message{types.NewUserMessage("hello")}})
	require.NoError(t, err)
	var terminal *types.StreamChunk
	var usages []*types.Usage
	for chunk := range stream {
		assert.NoError(t, chunk.Error)
		if chunk.Usage != nil {
			usages = append(usages, chunk.Usage)
		}
		if chunk.IsDone() {
			c := chunk
			terminal = &c
		}
	}
	require.NotNil(t, terminal)
	require.NotNil(t, terminal.Usage)
	assert.Equal(t, 10, terminal.Usage.PromptTokens)
	assert.Equal(t, 5, terminal.Usage.CompletionTokens)
	assert.Equal(t, 15, terminal.Usage.TotalTokens)
	assert.Equal(t, 2, terminal.Usage.CacheReadTokens)
	assert.Equal(t, 3, terminal.Usage.CacheWriteTokens)
	require.Len(t, usages, 3)
	assert.Equal(t, 1, usages[0].CompletionTokens, "later events must not mutate earlier snapshots")
	assert.Equal(t, 3, usages[1].CompletionTokens)
}

func TestAnthropicStreamPendingToolCallSurfacesErrorOnCleanClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`data: {"type":"message_start","message":{"id":"msg-1","model":"claude-test","usage":{"input_tokens":1}}}` + "\n\n",
			`data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tool-1","name":"lookup"}}` + "\n\n",
			`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}` + "\n\n",
			"data: [DONE]\n\n",
		} {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()
	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	stream, err := provider.Stream(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude-test"}, Messages: []types.Message{types.NewUserMessage("lookup")}})
	require.NoError(t, err)
	var sawError bool
	for chunk := range stream {
		if chunk.Error != nil {
			sawError = true
			assert.ErrorContains(t, chunk.Error, "tool-call completion")
			require.Len(t, chunk.ToolCalls, 1)
			assert.Equal(t, "tool-1", chunk.ToolCalls[0].ID)
		}
	}
	assert.True(t, sawError)
}

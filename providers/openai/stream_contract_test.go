package openai

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectToolContractStream(t *testing.T, events []string) []types.TextChunk {
	t.Helper()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			_, _ = io.WriteString(w, "data: "+event+"\n\n")
		}
	})
	t.Cleanup(func() { _ = provider.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := provider.Stream(ctx, types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "test-model"},
		Messages:    []types.Message{types.NewUserMessage("test")},
	})
	require.NoError(t, err)
	var chunks []types.TextChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	require.NoError(t, ctx.Err())
	return chunks
}

func TestOpenAIStreamToolCallsEmittedOnce(t *testing.T) {
	t.Parallel()
	const finish = `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`
	for _, tc := range []struct {
		name      string
		trailing  string
		wantError bool
	}{
		{name: "usage", trailing: `{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`},
		{name: "repeated_finish", trailing: finish},
		{name: "parse_error", trailing: `{"broken":`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			chunks := collectToolContractStream(t, []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"first","arguments":"{\"x\":"}},{"index":1,"id":"call_b","type":"function","function":{"name":"second","arguments":"{\"y\":"}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}},{"index":0,"function":{"arguments":"1}"}}]}}]}`,
				finish, tc.trailing, "[DONE]",
			})
			var calls []types.ToolCall
			errors := 0
			for _, chunk := range chunks {
				assert.Nil(t, chunk.ToolCall, "singular fragments must not escape")
				if chunk.Delta != nil {
					assert.Empty(t, chunk.Delta.ToolCalls, "delta fragments must not escape")
				}
				if len(chunk.ToolCalls) > 0 {
					assert.True(t, chunk.IsDone())
					assert.NoError(t, chunk.Error)
					calls = append(calls, chunk.ToolCalls...)
				}
				if chunk.Error != nil {
					errors++
				}
			}
			if tc.wantError {
				assert.Equal(t, 1, errors)
			} else {
				assert.Zero(t, errors)
			}
			require.Len(t, calls, 2)
			assert.Equal(t, "call_a", calls[0].ID)
			assert.Equal(t, "first", calls[0].Name)
			assert.Equal(t, map[string]any{"x": float64(1)}, calls[0].Arguments)
			assert.Equal(t, "call_b", calls[1].ID)
			assert.Equal(t, "second", calls[1].Name)
			assert.Equal(t, map[string]any{"y": float64(2)}, calls[1].Arguments)
		})
	}
}

func TestOpenAIStreamMalformedToolArguments(t *testing.T) {
	t.Parallel()
	chunks := collectToolContractStream(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	})
	var calls []types.ToolCall
	for _, chunk := range chunks {
		assert.NoError(t, chunk.Error)
		calls = append(calls, chunk.ToolCalls...)
	}
	require.Len(t, calls, 1)
	assert.True(t, calls[0].ArgsInvalid)
	assert.NotEmpty(t, calls[0].ArgsParseError)
	assert.Nil(t, calls[0].Arguments)
}

func TestOpenAIStreamDoneMarkerWithUnfinishedToolCall(t *testing.T) {
	t.Parallel()
	chunks := collectToolContractStream(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}}]}}]}`,
		"[DONE]",
	})
	var errors int
	for _, chunk := range chunks {
		if chunk.Error != nil {
			errors++
			assert.ErrorContains(t, chunk.Error, "tool-call completion")
		} else {
			assert.False(t, chunk.HasToolCalls(), "unfinished calls cannot be emitted as successful")
		}
	}
	assert.Equal(t, 1, errors)
}

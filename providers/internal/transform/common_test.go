package transform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestMapFinishReason(t *testing.T) {
	t.Parallel()
	tests := map[string]types.FinishReason{
		"stop":                      types.FinishReasonStop,
		"end_turn":                  types.FinishReasonStop,
		"length":                    types.FinishReasonLength,
		"max_tokens":                types.FinishReasonLength,
		"tool_calls":                types.FinishReasonToolCalls,
		"function_call":             types.FinishReasonToolCalls,
		"tool_use":                  types.FinishReasonToolCalls,
		"content_filter":            types.FinishReasonContentFilter,
		"safety":                    types.FinishReasonContentFilter,
		"recitation":                types.FinishReasonContentFilter,
		"other":                     types.FinishReasonOther,
		"finish_reason_unspecified": types.FinishReasonOther,
		"unexpected":                types.FinishReasonOther,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, MapFinishReason(input))
		})
	}
}

func TestStreamingTransformerCustomConfig(t *testing.T) {
	t.Parallel()
	transformer := NewStreamingTransformer(StreamingConfig{
		TextFieldPath:     "items.0.text",
		ToolCallFieldPath: "items.0.tool",
		FinishReasonPath:  "done",
		UsagePath:         "usage",
		IDPath:            "id",
		ModelPath:         "model",
		TextAdapter: func(v any) (string, error) {
			return "adapted-" + v.(string), nil
		},
		ToolCallAdapter: func(v any) (*types.ToolCall, error) {
			return &types.ToolCall{ID: v.(string), Name: "tool"}, nil
		},
		UsageAdapter: func(v any) (*types.Usage, error) {
			return &types.Usage{TotalTokens: int(v.(float64))}, nil
		},
	})

	chunk, err := transformer.ParseChunk([]byte(`{
		"id":"id-1",
		"model":"model-1",
		"done":"length",
		"usage": 9,
		"items":[{"text":"hello","tool":"call-1"}]
	}`))
	require.NoError(t, err)
	assert.Equal(t, "id-1", chunk.ID)
	assert.Equal(t, "model-1", chunk.Model)
	assert.Equal(t, "adapted-hello", chunk.Text)
	require.NotNil(t, chunk.ToolCall)
	assert.Equal(t, "call-1", chunk.ToolCall.ID)
	assert.Equal(t, 9, chunk.Usage.TotalTokens)
	require.NotNil(t, chunk.FinishReason)
	assert.Equal(t, types.FinishReasonLength, *chunk.FinishReason)
}

func TestStreamingTransformerErrorPathsAndBatches(t *testing.T) {
	t.Parallel()
	batch := NewGeminiStreamingTransformer()
	_, err := batch.ParseChunk([]byte(`{}`))
	require.Error(t, err)
	_, err = batch.ParseChunks([]byte(`{}`))
	require.Error(t, err)

	single := NewOpenAIStreamingTransformer()
	chunks, err := single.ParseChunks([]byte(`{"choices":[{"delta":{"content":"hi"}}]}`))
	require.NoError(t, err)
	require.Len(t, chunks, 1)

	_, err = NewStreamingTransformer(StreamingConfig{
		TextFieldPath: "text",
		TextAdapter: func(any) (string, error) {
			return "", assert.AnError
		},
	}).ParseChunk([]byte(`{"text":"hi"}`))
	require.Error(t, err)

	_, err = NewStreamingTransformer(StreamingConfig{
		ToolCallFieldPath: "tool",
		ToolCallAdapter: func(any) (*types.ToolCall, error) {
			return nil, assert.AnError
		},
	}).ParseChunk([]byte(`{"tool":{}}`))
	require.Error(t, err)

	_, err = NewStreamingTransformer(StreamingConfig{
		UsagePath: "usage",
	}).ParseChunk([]byte(`{"usage":"bad"}`))
	require.Error(t, err)
}

func TestStreamingTransformerDefaultToolCallAndUsageVariants(t *testing.T) {
	t.Parallel()
	transformer := NewStreamingTransformer(StreamingConfig{
		ToolCallFieldPath: "tool_calls",
		UsagePath:         "usage",
	})

	toolCallsJSON, err := json.Marshal([]map[string]any{{
		"id":   "call-1",
		"type": "function",
		"function": map[string]any{
			"name":      "lookup",
			"arguments": `{"city":"London"}`,
		},
	}})
	require.NoError(t, err)

	chunk, err := transformer.ParseChunk([]byte(`{
		"tool_calls": ` + string(toolCallsJSON) + `,
		"usage": {
			"promptTokenCount": 1,
			"candidatesTokenCount": 2,
			"totalTokenCount": 3
		}
	}`))
	require.NoError(t, err)
	require.Len(t, chunk.ToolCalls, 1)
	assert.Equal(t, "lookup", chunk.ToolCalls[0].Name)
	assert.Equal(t, 3, chunk.Usage.TotalTokens)
}

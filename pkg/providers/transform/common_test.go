package transform

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestResponseTransformCommonHelpers(t *testing.T) {
	t.Parallel()
	transformer := NewResponseTransform()
	created := time.Unix(100, 0)
	usage := transformer.BuildUsageFromTokens(1, 2, 3)
	toolCalls := []types.ToolCall{{ID: "call-1", Name: "lookup"}}

	text := transformer.TransformTextResponse("id", "model", "hello", usage, created, toolCalls)
	assert.Equal(t, "id", text.ID)
	assert.Equal(t, "hello", text.Text)
	assert.Equal(t, toolCalls, text.ToolCalls)
	assert.Same(t, usage, text.Usage)

	structured := transformer.TransformStructuredResponse("id", "model", map[string]any{"ok": true}, usage, created)
	assert.Equal(t, map[string]any{"ok": true}, structured.Data)

	embedding := transformer.BuildEmbeddingFromVector(2, []float64{0.1})
	assert.Equal(t, 2, embedding.Index)
	embeddings := transformer.TransformEmbeddingsResponse("model", []types.Embedding{embedding}, usage, created)
	assert.Len(t, embeddings.Embeddings, 1)
}

func TestResponseTransformChoiceExtraction(t *testing.T) {
	t.Parallel()
	transformer := NewResponseTransform()
	choices := []map[string]any{
		{"message": map[string]any{"content": ""}},
		{"message": map[string]any{
			"content": "hello",
			"tool_calls": []any{
				map[string]any{
					"id":   "call-1",
					"type": "function",
					"function": map[string]any{
						"name":      "lookup",
						"arguments": `{"city":"London"}`,
					},
				},
			},
		}},
	}

	assert.Equal(t, "hello", transformer.ExtractTextFromChoices(choices))
	toolCalls := transformer.ExtractToolCallsFromChoices(choices)
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call-1", toolCalls[0].ID)
	assert.Equal(t, "lookup", toolCalls[0].Name)
	assert.Equal(t, "London", toolCalls[0].Arguments["city"])
	assert.Empty(t, transformer.ExtractTextFromChoices(nil))
}

func TestResponseTransformParseToolCallVariants(t *testing.T) {
	t.Parallel()
	transformer := NewResponseTransform()

	openaiCall := transformer.ParseToolCallFromMap(map[string]any{
		"id":   "call-1",
		"type": "function",
		"function": map[string]any{
			"name":      "lookup",
			"arguments": `{"city":"London"}`,
		},
	})
	require.NotNil(t, openaiCall)
	assert.Equal(t, "lookup", openaiCall.Name)
	assert.Equal(t, "London", openaiCall.Arguments["city"])

	badArgs := transformer.ParseToolCallFromMap(map[string]any{
		"function": map[string]any{"name": "bad", "arguments": `{`},
	})
	require.NotNil(t, badArgs)
	assert.Nil(t, badArgs.Arguments)

	geminiCall := transformer.ParseToolCallFromMap(map[string]any{
		"name": "lookup",
		"args": map[string]any{"city": "Paris"},
	})
	require.NotNil(t, geminiCall)
	assert.Equal(t, "lookup", geminiCall.Name)
	assert.Equal(t, "Paris", geminiCall.Arguments["city"])
	require.NotNil(t, geminiCall.Function)

	stringArgs := transformer.ParseToolCallFromMap(map[string]any{
		"name":      "lookup",
		"arguments": `{"city":"Rome"}`,
	})
	require.NotNil(t, stringArgs)
	assert.Equal(t, "Rome", stringArgs.Arguments["city"])
}

func TestResponseTransformParsingHelpers(t *testing.T) {
	t.Parallel()
	transformer := NewResponseTransform()
	response := map[string]any{
		"id":      "resp-1",
		"model":   "model-1",
		"created": float64(100),
		"usage": map[string]any{
			"prompt_tokens":     float64(1),
			"completion_tokens": float64(2),
			"total_tokens":      float64(3),
		},
	}

	assert.Equal(t, "resp-1", transformer.ParseResponseID(response))
	assert.Equal(t, "model-1", transformer.ParseResponseModel(response))
	assert.Equal(t, time.Unix(100, 0), transformer.ParseTimestampFromMap(response))
	usage := transformer.ParseUsageFromMap(response)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	assert.Nil(t, transformer.ParseUsageFromMap(map[string]any{}))

	assert.Contains(t, transformer.ParseResponseID(map[string]any{}), "resp-")
	assert.Empty(t, transformer.ParseResponseModel(map[string]any{}))
	assert.WithinDuration(t, time.Now(), transformer.ParseTimestampFromMap(map[string]any{}), time.Second)
	assert.Equal(t, time.Unix(200, 0), transformer.ParseTimestampFromMap(map[string]any{"created": int64(200)}))

	var decoded map[string]any
	require.NoError(t, transformer.LenientUnmarshal([]byte(`{"ok":true}`), &decoded))
	require.Error(t, transformer.LenientUnmarshal([]byte(`{`), &decoded))
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

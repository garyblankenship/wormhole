package transform

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamingTransformer(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	// Test OpenAI-style streaming chunk
	data := []byte(`{
		"id": "chatcmpl-123",
		"model": "gpt-4",
		"choices": [{
			"delta": {
				"content": "Hello",
				"tool_calls": [{
					"id": "tool_123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"city\": \"London\"}"
					}
				}]
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`)

	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, "chatcmpl-123", chunk.ID)
	assert.Equal(t, "gpt-4", chunk.Model)
	assert.Equal(t, "Hello", chunk.Text)
	assert.NotNil(t, chunk.Delta)
	assert.Equal(t, "Hello", chunk.Delta.Content)

	require.NotNil(t, chunk.FinishReason)
	assert.Equal(t, types.FinishReasonStop, *chunk.FinishReason)

	require.NotNil(t, chunk.Usage)
	assert.Equal(t, 10, chunk.Usage.PromptTokens)
	assert.Equal(t, 5, chunk.Usage.CompletionTokens)
	assert.Equal(t, 15, chunk.Usage.TotalTokens)

	// Tool calls should be parsed
	assert.Len(t, chunk.ToolCalls, 1)
	if len(chunk.ToolCalls) > 0 {
		toolCall := chunk.ToolCalls[0]
		assert.Equal(t, "tool_123", toolCall.ID)
		assert.Equal(t, "function", toolCall.Type)
		assert.Equal(t, "get_weather", toolCall.Name)
		require.NotNil(t, toolCall.Function)
		assert.Equal(t, "get_weather", toolCall.Function.Name)
		assert.Equal(t, "{\"city\": \"London\"}", toolCall.Function.Arguments)
	}
}

func TestOpenAIStreamingTransformer_UsageIncludesCachedTokens(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	data := []byte(`{"id":"c1","model":"gpt-4o-mini","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":40}}}`)

	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	require.NotNil(t, chunk.Usage)
	assert.Equal(t, 100, chunk.Usage.PromptTokens)
	assert.Equal(t, 150, chunk.Usage.TotalTokens)
	assert.Equal(t, 40, chunk.Usage.CacheReadTokens)
}

func TestOpenAIStreamingTransformer_SimpleText(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	// Simple text chunk without tool calls or usage
	data := []byte(`{
		"id": "chatcmpl-456",
		"model": "gpt-3.5-turbo",
		"choices": [{
			"delta": {
				"content": " world"
			}
		}]
	}`)

	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, "chatcmpl-456", chunk.ID)
	assert.Equal(t, "gpt-3.5-turbo", chunk.Model)
	assert.Equal(t, " world", chunk.Text)
	assert.NotNil(t, chunk.Delta)
	assert.Equal(t, " world", chunk.Delta.Content)
	assert.Nil(t, chunk.FinishReason)
	assert.Nil(t, chunk.Usage)
	assert.Empty(t, chunk.ToolCalls)
}

func TestOpenAIStreamingTransformer_Refusal(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	chunk, err := transformer.ParseChunk([]byte(`{
		"id":"chatcmpl-refusal",
		"model":"gpt-test",
		"choices":[{"delta":{"refusal":"I cannot help with that."}}]
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, "I cannot help with that.", chunk.Refusal)
	require.NotNil(t, chunk.Delta)
	assert.Equal(t, "I cannot help with that.", chunk.Delta.Refusal)
	assert.Empty(t, chunk.Text)
	assert.Empty(t, chunk.Content())
}

func TestOpenAIStreamingTransformer_FinishReasonMapping(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	testCases := []struct {
		name     string
		reason   string
		expected types.FinishReason
	}{
		{"stop", "stop", types.FinishReasonStop},
		{"length", "length", types.FinishReasonLength},
		{"tool_calls", "tool_calls", types.FinishReasonToolCalls},
		{"function_call", "function_call", types.FinishReasonToolCalls},
		{"content_filter", "content_filter", types.FinishReasonContentFilter},
		{"unknown", "unknown", types.FinishReasonOther},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := []byte(`{
				"id": "test",
				"model": "gpt-4",
				"choices": [{
					"delta": {},
					"finish_reason": "` + tc.reason + `"
				}]
			}`)

			chunk, err := transformer.ParseChunk(data)
			require.NoError(t, err)
			require.NotNil(t, chunk)
			require.NotNil(t, chunk.FinishReason)
			assert.Equal(t, tc.expected, *chunk.FinishReason)
		})
	}
}

func TestAnthropicStreamingTransformer(t *testing.T) {
	t.Parallel()
	transformer := NewAnthropicStreamingTransformer()

	// Note: Anthropic uses event-based streaming which is more complex
	// This is a simplified test for basic text extraction
	data := []byte(`{
		"type": "content_block_delta",
		"delta": {
			"type": "text_delta",
			"text": "Hello"
		}
	}`)

	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, "Hello", chunk.Text)
	assert.NotNil(t, chunk.Delta)
	assert.Equal(t, "Hello", chunk.Delta.Content)
}

func TestOllamaStreamingTransformer(t *testing.T) {
	t.Parallel()
	transformer := NewOllamaStreamingTransformer()

	// Test Ollama-style streaming chunk
	data := []byte(`{
		"model": "llama2",
		"message": {
			"content": "Hello world"
		},
		"done": true
	}`)

	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, "llama2", chunk.Model)
	assert.Equal(t, "Hello world", chunk.Text)
	assert.NotNil(t, chunk.Delta)
	assert.Equal(t, "Hello world", chunk.Delta.Content)
	require.NotNil(t, chunk.FinishReason)
	assert.Equal(t, types.FinishReasonOther, *chunk.FinishReason)
}

func TestStreamingTransformer_EmptyData(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	// Empty data should error
	_, err := transformer.ParseChunk([]byte{})
	assert.Error(t, err)

	// Invalid JSON should error
	_, err = transformer.ParseChunk([]byte("invalid json"))
	assert.Error(t, err)
}

func TestStreamingTransformer_MissingFields(t *testing.T) {
	t.Parallel()
	transformer := NewOpenAIStreamingTransformer()

	// Data without required fields should still parse
	data := []byte(`{}`)
	chunk, err := transformer.ParseChunk(data)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	// Fields should be empty/default
	assert.Empty(t, chunk.ID)
	assert.Empty(t, chunk.Model)
	assert.Empty(t, chunk.Text)
	assert.Nil(t, chunk.FinishReason)
	assert.Nil(t, chunk.Usage)
	assert.Empty(t, chunk.ToolCalls)
}

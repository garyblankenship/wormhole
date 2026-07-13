package openai

import (
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanJSONResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON - no change",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON in markdown code block with json tag",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON in generic markdown code block",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON array in markdown code block",
			input:    "```json\n[{\"key\": \"value\"}]\n```",
			expected: `[{"key": "value"}]`,
		},
		{
			name:     "non-JSON in code block - no change",
			input:    "```\nThis is not JSON\n```",
			expected: "```\nThis is not JSON\n```",
		},
		{
			name:     "mixed content with JSON block",
			input:    "Here's the response:\n```json\n{\"result\": \"success\"}\n```\nThat's it!",
			expected: `{"result": "success"}`,
		},
		{
			name:     "no code blocks - no change",
			input:    "Just plain text without code blocks",
			expected: "Just plain text without code blocks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cleanJSONResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformTextResponseWithJSONCleaning(t *testing.T) {
	t.Parallel()
	provider := &Provider{}

	// Test with Anthropic model that returns JSON in code blocks
	response := &chatCompletionResponse{
		ID:      "test-id",
		Model:   "claude-opus-4.1",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message: message{
					Content: "```json\n{\"variations\": [{\"strategy\": \"test\"}]}\n```",
				},
				FinishReason: "stop",
			},
		},
	}

	result := provider.transformTextResponse(response)

	// Should have cleaned the JSON
	expected := `{"variations": [{"strategy": "test"}]}`
	assert.Equal(t, expected, result.Text)
	assert.Equal(t, "claude-opus-4.1", result.Model)
}

func TestTransformTextResponseModelAgnosticCleaning(t *testing.T) {
	t.Parallel()
	provider := &Provider{}

	// Non-Anthropic model with JSON wrapped in markdown: stripping is now
	// applied unconditionally (no model-name sniff), so it must be cleaned.
	response := &chatCompletionResponse{
		ID:      "test-id",
		Model:   "gpt-4",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message: message{
					Content: "```json\n{\"key\": \"value\"}\n```",
				},
				FinishReason: "stop",
			},
		},
	}

	result := provider.transformTextResponse(response)

	// Cleaning is model-agnostic now: the gpt-4 response is stripped too.
	expected := `{"key": "value"}`
	assert.Equal(t, expected, result.Text)
}

func TestConvertUsageCacheTokenMapping(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	t.Run("cache token mapping", func(t *testing.T) {
		t.Parallel()
		u := usage{
			PromptTokens:        100,
			CompletionTokens:    50,
			TotalTokens:         150,
			PromptTokensDetails: &promptTokensDetail{CachedTokens: 40},
		}
		result := p.convertUsage(u)
		assert.Equal(t, 40, result.CacheReadTokens)
		assert.Equal(t, 0, result.CacheWriteTokens)
	})

	t.Run("nil details yields zero cache read", func(t *testing.T) {
		t.Parallel()
		u := usage{
			PromptTokens:        100,
			CompletionTokens:    50,
			TotalTokens:         150,
			PromptTokensDetails: nil,
		}
		result := p.convertUsage(u)
		assert.Equal(t, 0, result.CacheReadTokens)
		assert.Equal(t, 0, result.CacheWriteTokens)
	})
}

func TestTransformTextResponsePlainTextUnchanged(t *testing.T) {
	t.Parallel()
	provider := &Provider{}

	// No code fences: cleanJSONResponse is a no-op, content passes through.
	response := &chatCompletionResponse{
		ID:      "test-id",
		Model:   "gpt-4",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message: message{
					Content: "Just plain text, no JSON here.",
				},
				FinishReason: "stop",
			},
		},
	}

	result := provider.transformTextResponse(response)

	expected := "Just plain text, no JSON here."
	assert.Equal(t, expected, result.Text)
}

func TestBuildChatPayloadKeepsTextOnlyUserContentString(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	payload := provider.buildChatPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages: []types.Message{
			types.NewUserMessage("plain text"),
		},
	})

	messages := payload["messages"].([]map[string]any)
	require.Len(t, messages, 1)
	assert.Equal(t, "plain text", messages[0]["content"])
}

func TestBuildChatPayloadSerializesUserMediaAsImageURLParts(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	payload := provider.buildChatPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages: []types.Message{
			&types.UserMessage{
				Content: "compare these",
				Media: []types.Media{
					&types.ImageMedia{MimeType: "image/png", Base64Data: "aW1hZ2U="},
					&types.ImageMedia{URL: "https://example.test/image.jpg"},
				},
			},
		},
	})

	messages := payload["messages"].([]map[string]any)
	require.Len(t, messages, 1)
	parts := messages[0]["content"].([]map[string]any)
	require.Len(t, parts, 3)
	assert.Equal(t, map[string]any{"type": "text", "text": "compare these"}, parts[0])
	assert.Equal(t, "image_url", parts[1]["type"])
	assert.Equal(t, map[string]any{"url": "data:image/png;base64,aW1hZ2U="}, parts[1]["image_url"])
	assert.Equal(t, "image_url", parts[2]["type"])
	assert.Equal(t, map[string]any{"url": "https://example.test/image.jpg"}, parts[2]["image_url"])
}

func TestTransform_MalformedToolCallArgs_FlaggedNotSwallowed(t *testing.T) {
	t.Parallel()

	provider := &Provider{}
	truncatedArgs := `{"path":"/tmp/file`

	response := &chatCompletionResponse{
		ID:      "malformed-tool-args",
		Model:   "gpt-4o-mini",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message: message{
					Role: "assistant",
					ToolCalls: []toolCall{{
						ID:   "call-1",
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      "read_file",
							Arguments: truncatedArgs,
						},
					}},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	result := provider.transformTextResponse(response)
	require.Len(t, result.ToolCalls, 1)
	call := result.ToolCalls[0]

	assert.True(t, call.ArgsInvalid)
	assert.NotEmpty(t, call.ArgsParseError)
	assert.Equal(t, truncatedArgs, call.Function.Arguments)
	assert.Empty(t, call.Arguments)
}

func TestTransformEmbeddingsResponseBackfillsModel(t *testing.T) {
	t.Parallel()

	p := &Provider{}

	t.Run("empty response model uses request model", func(t *testing.T) {
		t.Parallel()
		response := &embeddingsResponse{
			Model: "",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float32{0.1, 0.2}},
			},
		}
		result := p.transformEmbeddingsResponse(response, "req-x")
		assert.Equal(t, "req-x", result.Model)
	})

	t.Run("provider model is preserved", func(t *testing.T) {
		t.Parallel()
		response := &embeddingsResponse{
			Model: "prov-y",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float32{0.3, 0.4}},
			},
		}
		result := p.transformEmbeddingsResponse(response, "req-x")
		assert.Equal(t, "prov-y", result.Model)
	})
}

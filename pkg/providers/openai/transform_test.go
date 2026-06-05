package openai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

package openai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCleanJSONResponse(t *testing.T) {
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
			result := cleanJSONResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformTextResponseWithJSONCleaning(t *testing.T) {
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

func TestTransformTextResponseWithoutCleaning(t *testing.T) {
	provider := &Provider{}

	// Test with non-Anthropic model - should not clean
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
	
	// Should NOT have cleaned the JSON for non-Anthropic models
	expected := "```json\n{\"key\": \"value\"}\n```"
	assert.Equal(t, expected, result.Text)
}
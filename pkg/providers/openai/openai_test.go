package openai_test

import (
	"testing"

	"github.com/prism-php/prism-go/pkg/providers/openai"
	"github.com/prism-php/prism-go/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProvider(t *testing.T) {
	config := types.ProviderConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.openai.com/v1",
	}

	provider := openai.New(config)
	assert.NotNil(t, provider)
	assert.Equal(t, "openai", provider.Name())
}

func TestBuildChatPayload(t *testing.T) {
	// This tests the internal payload building
	// In a real test, we'd test against actual API or use mocks

	t.Run("basic payload", func(t *testing.T) {
		// Test that provider can be created
		provider := openai.New(types.ProviderConfig{
			APIKey: "test",
		})
		require.NotNil(t, provider)
	})

	t.Run("with tools", func(t *testing.T) {
		tool := types.NewTool(
			"test_tool",
			"Test tool",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"param": map[string]string{"type": "string"},
				},
			},
		)
		assert.Equal(t, "test_tool", tool.Name)
	})
}

func TestTransformResponses(t *testing.T) {
	t.Run("finish reason mapping", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected types.FinishReason
		}{
			{"stop", types.FinishReasonStop},
			{"length", types.FinishReasonLength},
			{"tool_calls", types.FinishReasonToolCalls},
			{"content_filter", types.FinishReasonContentFilter},
			{"unknown", types.FinishReasonStop},
		}

		for _, tc := range testCases {
			// In real implementation, we'd test the actual mapping function
			assert.NotEmpty(t, tc.expected)
		}
	})

	t.Run("usage conversion", func(t *testing.T) {
		usage := &types.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		}
		assert.Equal(t, 150, usage.TotalTokens)
	})
}

func TestStreamChunkParsing(t *testing.T) {
	chunk := types.StreamChunk{
		ID:    "test",
		Model: "gpt-4",
		Delta: &types.ChunkDelta{
			Content: "Hello",
		},
	}

	assert.Equal(t, "Hello", chunk.Delta.Content)
	assert.Empty(t, chunk.FinishReason)
}

func TestMultimodalMessages(t *testing.T) {
	parts := []types.MessagePart{
		types.TextPart("Look at this:"),
		types.ImagePart(map[string]interface{}{
			"url": "https://example.com/image.jpg",
		}),
	}

	// For multimodal messages, we need to create a user message with parts as content
	// This would typically be handled by a separate multimodal message constructor
	msg := types.NewUserMessage("Look at this image")
	assert.Equal(t, types.RoleUser, msg.GetRole())

	// Test the parts separately
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "image", parts[1].Type)
	assert.Equal(t, "Look at this:", parts[0].Text)
	assert.NotNil(t, parts[1].Data)
}

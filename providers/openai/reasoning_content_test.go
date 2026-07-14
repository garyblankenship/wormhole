package openai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestTransformTextResponseReasoningContent(t *testing.T) {
	t.Parallel()
	provider := &Provider{}

	withReasoning := &chatCompletionResponse{
		ID:      "rc-1",
		Model:   "deepseek-v4-pro",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message:      message{Content: "the answer", ReasoningContent: "chain of thought"},
				FinishReason: "stop",
			},
		},
	}
	result := provider.transformTextResponse(withReasoning)
	require.NotNil(t, result.Thinking)
	assert.Equal(t, "chain of thought", result.Thinking.Content)
	assert.Equal(t, "the answer", result.Text)

	withoutReasoning := &chatCompletionResponse{
		ID:      "rc-2",
		Model:   "deepseek-v4-pro",
		Created: time.Now().Unix(),
		Choices: []struct {
			Index        int     `json:"index"`
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			{
				Message:      message{Content: "the answer"},
				FinishReason: "stop",
			},
		},
	}
	result = provider.transformTextResponse(withoutReasoning)
	assert.Nil(t, result.Thinking)
}

func TestParseStreamChunkReasoningContent(t *testing.T) {
	t.Parallel()
	provider := New(types.ProviderConfig{APIKey: "test-key"})
	provider.streamingTransformer = nil

	chunk, err := provider.parseStreamChunk([]byte(`{
		"id":"chunk-rc","model":"deepseek-v4-pro",
		"choices":[{"delta":{"reasoning_content":"thinking step"}}]
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.NotNil(t, chunk.Thinking)
	assert.Equal(t, "thinking step", chunk.Thinking.Content)
	require.NotNil(t, chunk.Delta)
	require.NotNil(t, chunk.Delta.Thinking)
	assert.Equal(t, "thinking step", chunk.Delta.Thinking.Content)

	chunk, err = provider.parseStreamChunk([]byte(`{
		"id":"chunk-c","model":"deepseek-v4-pro",
		"choices":[{"delta":{"content":"hi"}}]
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Nil(t, chunk.Thinking)
}

func TestConvertUsagePromptCacheHitTokens(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	result := p.convertUsage(usage{
		PromptTokens:         100,
		CompletionTokens:     50,
		TotalTokens:          150,
		PromptCacheHitTokens: 40,
	})
	assert.Equal(t, 40, result.CacheReadTokens)

	// OpenAI cached_tokens takes precedence over the DeepSeek hit-token fallback
	result = p.convertUsage(usage{
		PromptTokens:         100,
		CompletionTokens:     50,
		TotalTokens:          150,
		PromptCacheHitTokens: 40,
		PromptTokensDetails:  &promptTokensDetail{CachedTokens: 25},
	})
	assert.Equal(t, 25, result.CacheReadTokens)
}

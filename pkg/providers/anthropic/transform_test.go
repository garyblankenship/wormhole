package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertUsageCacheTokenMapping(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	t.Run("cache tokens populated", func(t *testing.T) {
		t.Parallel()
		u := messageUsage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheReadInputTokens:     30,
			CacheCreationInputTokens: 20,
		}
		result := p.convertUsage(u)
		assert.Equal(t, 100, result.PromptTokens)
		assert.Equal(t, 50, result.CompletionTokens)
		assert.Equal(t, 150, result.TotalTokens)
		assert.Equal(t, 30, result.CacheReadTokens)
		assert.Equal(t, 20, result.CacheWriteTokens)
	})

	t.Run("zero cache fields yield zeros", func(t *testing.T) {
		t.Parallel()
		u := messageUsage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheReadInputTokens:     0,
			CacheCreationInputTokens: 0,
		}
		result := p.convertUsage(u)
		assert.Equal(t, 0, result.CacheReadTokens)
		assert.Equal(t, 0, result.CacheWriteTokens)
	})
}

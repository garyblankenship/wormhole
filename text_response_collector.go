package wormhole

import "github.com/garyblankenship/wormhole/v3/types"

type textResponseCollector struct {
	response     types.TextResponse
	pending      []types.ToolCall
	toolSnapshot bool
}

func (c *textResponseCollector) add(chunk types.TextChunk) {
	if chunk.ID != "" {
		c.response.ID = chunk.ID
	}
	if chunk.Provider != "" {
		c.response.Provider = chunk.Provider
	}
	if chunk.Model != "" {
		c.response.Model = chunk.Model
	}
	c.response.Text += chunk.Content()
	refusal, thinking := chunk.Refusal, chunk.Thinking
	if chunk.Delta != nil {
		if refusal == "" {
			refusal = chunk.Delta.Refusal
		}
		if thinking == nil {
			thinking = chunk.Delta.Thinking
		}
	}
	c.response.Refusal += refusal
	c.addThinking(thinking)
	if chunk.FinishReason != nil && *chunk.FinishReason != "" {
		c.response.FinishReason = *chunk.FinishReason
	}
	c.addUsage(chunk.Usage)
	c.addTools(chunk)
}

func (c *textResponseCollector) addThinking(thinking *types.Thinking) {
	if thinking == nil {
		return
	}
	if c.response.Thinking == nil {
		c.response.Thinking = &types.Thinking{}
	}
	c.response.Thinking.Content += thinking.Content
	if thinking.Signature != "" {
		c.response.Thinking.Signature = thinking.Signature
	}
	if thinking.Provider != "" {
		c.response.Thinking.Provider = thinking.Provider
	}
}

func (c *textResponseCollector) addUsage(usage *types.Usage) {
	if usage == nil {
		return
	}
	if c.response.Usage == nil {
		c.response.Usage = &types.Usage{}
	}
	current := c.response.Usage
	current.PromptTokens = max(current.PromptTokens, usage.PromptTokens)
	current.CompletionTokens = max(current.CompletionTokens, usage.CompletionTokens)
	current.TotalTokens = max(current.TotalTokens, usage.TotalTokens)
	current.CacheReadTokens = max(current.CacheReadTokens, usage.CacheReadTokens)
	current.CacheWriteTokens = max(current.CacheWriteTokens, usage.CacheWriteTokens)
	current.ReasoningTokens = max(current.ReasoningTokens, usage.ReasoningTokens)
}

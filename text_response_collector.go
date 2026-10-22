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
	c.addFidelity(chunk)
	c.addTools(chunk)
}

func (c *textResponseCollector) addFidelity(chunk types.TextChunk) {
	if chunk.ResponseMetadata != nil {
		c.response.ResponseMetadata = types.CloneResponseMetadata(chunk.ResponseMetadata)
	}
	if chunk.Metadata != nil {
		if c.response.Metadata == nil {
			c.response.Metadata = make(map[string]any)
		}
		for key, value := range chunk.Metadata {
			c.response.Metadata[key] = types.CloneValue(value)
		}
	}
	if chunk.IsDone() {
		// Terminal reference snapshots replace additions already collected.
		if chunk.Sources != nil {
			c.response.Sources = types.CloneSources(chunk.Sources)
		}
		if chunk.Citations != nil {
			c.response.Citations = types.CloneCitations(chunk.Citations)
		}
		if chunk.Warnings != nil {
			c.response.Warnings = types.CloneWarnings(chunk.Warnings)
		}
		return
	}
	c.response.Sources = append(c.response.Sources, types.CloneSources(chunk.Sources)...)
	c.response.Citations = append(c.response.Citations, types.CloneCitations(chunk.Citations)...)
	c.response.Warnings = append(c.response.Warnings, types.CloneWarnings(chunk.Warnings)...)
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
	if usage.CacheWriteTokensByTTL != nil {
		if current.CacheWriteTokensByTTL == nil {
			current.CacheWriteTokensByTTL = make(map[string]int)
		}
		for ttl, tokens := range usage.CacheWriteTokensByTTL {
			current.CacheWriteTokensByTTL[ttl] = max(current.CacheWriteTokensByTTL[ttl], tokens)
		}
	}
}

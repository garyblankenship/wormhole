package openai

import (
	"encoding/json"

	providerTransform "github.com/garyblankenship/wormhole/v2/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v2/types"
)

// parseStreamChunk parses a streaming chunk
func (p *Provider) parseStreamChunk(data []byte) (*types.TextChunk, error) {
	// Try to use unified streaming transformer if available
	if p.streamingTransformer != nil {
		return p.streamingTransformer.ParseChunk(data)
	}

	// Fall back to original implementation
	var response streamResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.Choices) == 0 {
		return nil, nil
	}

	choice := response.Choices[0]
	chunk := &types.StreamChunk{
		ID:    response.ID,
		Model: response.Model,
		Text:  choice.Delta.Content, // Set Text field for backward compatibility
		Delta: &types.ChunkDelta{
			Content: choice.Delta.Content,
		},
	}

	if choice.Delta.Refusal != "" {
		chunk.Refusal = choice.Delta.Refusal
		chunk.Delta.Refusal = choice.Delta.Refusal
	}

	if choice.Delta.ReasoningContent != "" {
		thinking := &types.Thinking{Content: choice.Delta.ReasoningContent}
		chunk.Thinking = thinking
		chunk.Delta.Thinking = thinking
	}

	if len(choice.Delta.ToolCalls) > 0 {
		chunk.ToolCalls = p.convertToolCalls(choice.Delta.ToolCalls)
	}

	if choice.FinishReason != "" {
		reason := p.mapFinishReason(choice.FinishReason)
		chunk.FinishReason = &reason
	}

	if response.Usage != nil {
		chunk.Usage = p.convertUsage(*response.Usage)
	}

	return chunk, nil
}

// Helper functions

func (p *Provider) convertToolCalls(toolCalls []toolCall) []types.ToolCall {
	result := make([]types.ToolCall, len(toolCalls))

	for i, tc := range toolCalls {
		// Parse arguments from JSON string to map[string]any. For streaming
		// fragments tc.Function.Arguments is partial JSON that will not parse;
		// the accumulator (stream_accumulator.go) stitches fragments by index
		// and parses once. We always carry the raw fragment string in
		// Function.Arguments so the accumulator can reassemble it.
		// Empty default nil: an absent arg set stays a nil map here (the streaming
		// accumulator carries the raw fragment in Function.Arguments and reparses).
		argsMap, parseErrMsg := types.ParseToolArgs(tc.Function.Arguments, nil)

		toolCall := types.ToolCall{
			Index:     i,
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name,
			Arguments: argsMap,
			Function: &types.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
		if tc.Index != nil {
			toolCall.Index = *tc.Index
		}
		toolCall.MarkArgsError(parseErrMsg)
		result[i] = toolCall
	}

	return result
}

func (p *Provider) convertUsage(u usage) *types.Usage {
	usage := &types.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		usage.CacheReadTokens = u.PromptTokensDetails.CachedTokens
	}
	if usage.CacheReadTokens == 0 && u.PromptCacheHitTokens > 0 {
		usage.CacheReadTokens = u.PromptCacheHitTokens
	}
	if u.CompletionTokensDetails != nil {
		usage.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return usage
}

func (p *Provider) mapFinishReason(reason string) types.FinishReason {
	return providerTransform.MapFinishReason(reason)
}

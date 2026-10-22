package anthropic

import (
	"encoding/json"
	"time"

	providerTransform "github.com/garyblankenship/wormhole/v3/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v3/types"
)

// transformTextResponse converts Anthropic response to internal format
func (p *Provider) transformTextResponse(response *messageResponse) *types.TextResponse {
	text := ""
	var thinking *types.Thinking
	var toolCalls []types.ToolCall

	// Extract content from response
	for _, content := range response.Content {
		switch content.Type {
		case contentTypeText:
			text += content.Text
		case contentTypeThinking:
			thinking = &types.Thinking{Content: content.Thinking, Signature: content.Signature, Provider: "anthropic"}
		case contentTypeToolUse:
			args, _ := json.Marshal(content.Input)
			toolCalls = append(toolCalls, types.ToolCall{
				ID:        content.ID,
				Type:      "function",
				Name:      content.Name,
				Arguments: content.Input,
				Function: &types.ToolCallFunction{
					Name:      content.Name,
					Arguments: string(args),
				},
			})
		}
	}

	result := &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         text,
		Thinking:     thinking,
		ToolCalls:    toolCalls,
		FinishReason: p.mapStopReason(response.StopReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Now(),
	}
	if response.Usage.Raw != nil {
		result.Metadata = map[string]any{"provider_usage": response.Usage.Raw}
	}
	refs := newCitationAccumulator()
	for _, content := range response.Content {
		for _, raw := range content.Citations {
			refs.add(raw)
		}
	}
	result.Sources, result.Citations, result.Warnings = refs.sources, refs.citations, refs.warnings
	return result
}

func (p *Provider) convertUsage(u messageUsage) *types.Usage {
	return &types.Usage{
		PromptTokens:          u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens,
		CompletionTokens:      u.OutputTokens,
		TotalTokens:           u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens + u.OutputTokens,
		CacheReadTokens:       u.CacheReadInputTokens,
		CacheWriteTokens:      u.CacheCreationInputTokens,
		CacheWriteTokensByTTL: cloneTTL(u.CacheCreation),
	}
}

func (p *Provider) mapStopReason(reason string) types.FinishReason {
	return providerTransform.MapFinishReason(reason)
}

func cloneTTL(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

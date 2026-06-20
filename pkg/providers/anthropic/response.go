package anthropic

import (
	"encoding/json"
	"time"

	providerTransform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
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
				ID:   content.ID,
				Type: "function",
				Function: &types.ToolCallFunction{
					Name:      content.Name,
					Arguments: string(args),
				},
			})
		}
	}

	return &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         text,
		Thinking:     thinking,
		ToolCalls:    toolCalls,
		FinishReason: p.mapStopReason(response.StopReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Now(),
	}
}

func (p *Provider) convertUsage(u messageUsage) *types.Usage {
	return &types.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

func (p *Provider) mapStopReason(reason string) types.FinishReason {
	return providerTransform.MapFinishReason(reason)
}

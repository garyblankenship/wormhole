package anthropic

import (
	"fmt"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Content type constants
const (
	contentTypeText     = "text"
	contentTypeThinking = "thinking"
	contentTypeToolUse  = "tool_use"
)

// Role constant
const roleUser = "user"

// buildMessagePayload builds the Anthropic messages API payload
func (p *Provider) buildMessagePayload(request *types.TextRequest) map[string]any {
	prepared, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		prepared = request.Messages
	}
	payload := map[string]any{
		"model":    request.Model,
		"messages": p.transformMessages(prepared),
	}

	// Add system prompt if present. Anthropic requires system content in the
	// top-level field, while OpenAI-compatible callers often send it as a
	// normal system message.
	if system := mergeSystemMessages(request.SystemPrompt, request.Messages); system != "" {
		payload["system"] = system
	}

	// Handle max tokens - Anthropic requires this field
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_tokens"] = *request.MaxTokens
	} else {
		payload["max_tokens"] = config.GetDefaultAnthropicMaxTokens()
	}

	// Optional parameters - use shared utility
	// Pass nil for maxTokens since Anthropic handles max_tokens separately
	p.requestBuilder.AddGenerationParams(payload, request.Temperature, request.TopP, nil, request.Stop)
	// Anthropic uses "stop_sequences" instead of "stop", so rename if present
	if stop, ok := payload["stop"]; ok {
		payload["stop_sequences"] = stop
		delete(payload, "stop")
	}

	if thinking := anthropicThinkingPayload(request.Reasoning); len(thinking) > 0 {
		payload["thinking"] = thinking
	}

	// Tools
	if len(request.Tools) > 0 {
		payload["tools"] = p.transformTools(request.Tools)
		if request.ToolChoice != nil {
			payload["tool_choice"] = request.ToolChoice
		}
	}

	// Provider options
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	return payload
}

func anthropicThinkingPayload(reasoning *types.Reasoning) map[string]any {
	if reasoning == nil {
		return nil
	}
	out := make(map[string]any, 2)
	if reasoning.Enabled != nil && !*reasoning.Enabled {
		out["type"] = "disabled"
		return out
	}
	if reasoning.MaxTokens > 0 {
		out["type"] = "enabled"
		out["budget_tokens"] = reasoning.MaxTokens
	}
	return out
}

// transformMessages converts internal messages to Anthropic format
func (p *Provider) transformMessages(messages []types.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))

	for _, msg := range messages {
		// Skip system messages as they go in a separate field
		if msg.GetRole() == types.RoleSystem {
			continue
		}

		anthropicMsg := map[string]any{
			"role": p.mapRole(msg.GetRole()),
		}

		// Build content array
		content := p.buildContent(msg)
		anthropicMsg["content"] = content

		result = append(result, anthropicMsg)
	}

	return result
}

// buildContent builds the content array for a message
func (p *Provider) buildContent(msg types.Message) []map[string]any {
	var contentParts []map[string]any

	content := msg.GetContent()

	switch c := content.(type) {
	case string:
		contentParts = append(contentParts, map[string]any{
			"type": contentTypeText,
			"text": c,
		})
	case []types.MessagePart:
		for _, part := range c {
			switch part.Type {
			case contentTypeText:
				contentParts = append(contentParts, map[string]any{
					"type": contentTypeText,
					"text": part.Text,
				})
			case "image":
				contentParts = append(contentParts, map[string]any{
					"type":   "image",
					"source": part.Data,
				})
			}
		}
	default:
		// Try to convert to string
		contentParts = append(contentParts, map[string]any{
			"type": contentTypeText,
			"text": fmt.Sprintf("%v", content),
		})
	}

	// Handle tool messages
	if toolMsg, ok := msg.(*types.ToolMessage); ok {
		// Tool results are text content with tool_use_id
		if len(contentParts) > 0 {
			contentParts[0]["tool_use_id"] = toolMsg.ToolCallID
		}
	}

	// Handle assistant messages with tool calls
	if assistantMsg, ok := msg.(*types.AssistantMessage); ok && len(assistantMsg.ToolCalls) > 0 {
		for _, toolCall := range assistantMsg.ToolCalls {
			var input map[string]any
			_ = utils.UnmarshalAnthropicToolArgs(toolCall.Function.Arguments, &input)
			contentParts = append(contentParts, map[string]any{
				"type":  "tool_use",
				"id":    toolCall.ID,
				"name":  toolCall.Function.Name,
				"input": input,
			})
		}
	}

	return contentParts
}

// mapRole maps internal roles to Anthropic roles
func (p *Provider) mapRole(role types.Role) string {
	switch role {
	case types.RoleUser:
		return roleUser
	case types.RoleAssistant:
		return "assistant"
	case types.RoleTool:
		return roleUser // Anthropic uses 'user' role for tool results
	default:
		return string(role)
	}
}

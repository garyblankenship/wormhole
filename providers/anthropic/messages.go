package anthropic

import (
	"fmt"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
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
	prepared, _, err := providers.PrepareMessages(request.Messages)
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
		var toolChoice map[string]any
		if request.ToolChoice != nil {
			toolChoice = p.transformToolChoice(request.ToolChoice)
		}
		if request.ParallelToolCalls != nil && (request.ToolChoice == nil || request.ToolChoice.Type != types.ToolChoiceTypeNone) {
			if toolChoice == nil {
				toolChoice = map[string]any{"type": "auto"}
			}
			toolChoice["disable_parallel_tool_use"] = !*request.ParallelToolCalls
		}
		if toolChoice != nil {
			payload["tool_choice"] = toolChoice
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

	return coalesceSameRole(result)
}

// coalesceSameRole merges consecutive messages that share the same mapped role
// into a single message whose "content" block array is the concatenation of the
// merged entries' blocks. Anthropic requires strict user/assistant alternation
// and allows one role-turn to carry multiple content blocks, so a tool_result
// turn (mapped to "user") followed by a real user turn becomes a single "user"
// message holding both blocks. Order is preserved; nothing is dropped.
func coalesceSameRole(messages []map[string]any) []map[string]any {
	if len(messages) <= 1 {
		return messages
	}
	merged := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if n := len(merged); n > 0 && merged[n-1]["role"] == msg["role"] {
			prevBlocks, _ := merged[n-1]["content"].([]map[string]any)
			curBlocks, _ := msg["content"].([]map[string]any)
			merged[n-1]["content"] = append(prevBlocks, curBlocks...)
			continue
		}
		merged = append(merged, msg)
	}
	return merged
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

	// Handle tool messages: Anthropic requires a distinct tool_result block,
	// not a text block with tool_use_id bolted on.
	if toolMsg, ok := msg.(*types.ToolMessage); ok {
		result := []map[string]any{
			{
				"type":        "tool_result",
				"tool_use_id": toolMsg.ToolCallID,
				"content":     toolMsg.Content,
			},
		}
		// Anthropic requires is_error to distinguish a failed tool execution;
		// otherwise the error text in content is treated as a successful result.
		if toolMsg.Error != "" {
			result[0]["is_error"] = true
		}
		return result
	}

	// Handle assistant messages with tool calls
	if assistantMsg, ok := msg.(*types.AssistantMessage); ok && len(assistantMsg.ToolCalls) > 0 {
		for _, toolCall := range assistantMsg.ToolCalls {
			// types.ToolCall carries top-level Name/Arguments (Gemini-origin and
			// public map-form calls) OR an optional *ToolCallFunction (OpenAI-form).
			// Never deref a nil Function.
			name := toolCall.Name
			if name == "" && toolCall.Function != nil {
				name = toolCall.Function.Name
			}

			input := toolCall.Arguments
			if input == nil {
				input = map[string]any{}
				if toolCall.Function != nil {
					_ = unmarshalToolArgs(toolCall.Function.Arguments, &input)
				}
			}

			contentParts = append(contentParts, map[string]any{
				"type":  "tool_use",
				"id":    toolCall.ID,
				"name":  name,
				"input": input,
			})
		}
	}

	// Anthropic requires the prior turn's signed thinking block echoed back
	// (and it MUST be the first block) when extended thinking is interleaved
	// with tool_use. Prepend it when present.
	if assistantMsg, ok := msg.(*types.AssistantMessage); ok &&
		assistantMsg.Thinking != nil && assistantMsg.Thinking.Signature != "" &&
		(assistantMsg.Thinking.Provider == "" || assistantMsg.Thinking.Provider == "anthropic") {
		thinkingBlock := map[string]any{
			"type":      contentTypeThinking,
			"thinking":  assistantMsg.Thinking.Content,
			"signature": assistantMsg.Thinking.Signature,
		}
		contentParts = append([]map[string]any{thinkingBlock}, contentParts...)
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

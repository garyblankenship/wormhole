package providers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// RequestBuilder provides common request building utilities
// that can be used across different provider implementations
type RequestBuilder struct{}

// BuildTextPayload creates a base text request payload with common fields
func (b *RequestBuilder) BuildTextPayload(
	model string,
	messages []any,
	systemPrompt string,
) map[string]any {
	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}

	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	return payload
}

// AddGenerationParams adds generation parameters to a payload
func (b *RequestBuilder) AddGenerationParams(
	payload map[string]any,
	temperature, topP *float32,
	maxTokens *int,
	stop []string,
) {
	if temperature != nil {
		payload["temperature"] = *temperature
	}
	if topP != nil {
		payload["top_p"] = *topP
	}
	if maxTokens != nil && *maxTokens > 0 {
		payload["max_tokens"] = *maxTokens
	}
	if len(stop) > 0 {
		payload["stop"] = stop
	}
}

// TransformMessage converts a Wormhole message to provider format
func (b *RequestBuilder) TransformMessage(msg any) map[string]any {
	switch m := msg.(type) {
	case *types.UserMessage:
		return map[string]any{
			"role":    "user",
			"content": m.Content,
		}
	case *types.AssistantMessage:
		result := map[string]any{
			"role":    "assistant",
			"content": m.Content,
		}
		if len(m.ToolCalls) > 0 {
			result["tool_calls"] = b.transformToolCalls(m.ToolCalls)
		}
		return result
	case *types.SystemMessage:
		return map[string]any{
			"role":    "system",
			"content": m.Content,
		}
	case *types.ToolMessage:
		return map[string]any{
			"role":       "tool",
			"content":    m.Content,
			"tool_call_id": m.ToolCallID,
		}
	default:
		return map[string]any{
			"role":    "user",
			"content": fmt.Sprintf("%v", msg),
		}
	}
}

// TransformMessages converts a slice of Wormhole messages to provider format
func (b *RequestBuilder) TransformMessages(messages []any) []map[string]any {
	result := make([]map[string]any, len(messages))
	for i, msg := range messages {
		result[i] = b.TransformMessage(msg)
	}
	return result
}

// TransformMessagesFromInterface converts a slice of Message interfaces to provider format
func (b *RequestBuilder) TransformMessagesFromInterface(messages []types.Message) []map[string]any {
	result := make([]map[string]any, len(messages))
	for i, msg := range messages {
		// types.Message is an interface, but TransformMessage expects any
		// The underlying concrete type will be properly handled by TransformMessage's type switch
		result[i] = b.TransformMessage(msg)
	}
	return result
}

// transformToolCalls converts Wormhole tool calls to provider format
func (b *RequestBuilder) transformToolCalls(toolCalls []types.ToolCall) []map[string]any {
	result := make([]map[string]any, len(toolCalls))
	for i, tc := range toolCalls {
		tcMap := map[string]any{
			"id":   tc.ID,
			"type": "function",
		}

		if tc.Function != nil {
			tcMap["function"] = map[string]any{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			}
		} else if len(tc.Arguments) > 0 {
			// Convert map[string]any to JSON string for arguments
			argsJSON, err := json.Marshal(tc.Arguments)
			if err == nil {
				tcMap["function"] = map[string]any{
					"name":      tc.Type,
					"arguments": string(argsJSON),
				}
			}
		}

		result[i] = tcMap
	}
	return result
}

// TransformTool converts a Wormhole tool to provider format
func (b *RequestBuilder) TransformTool(tool types.Tool) map[string]any {
	toolMap := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		},
	}

	// Add parameters if provided
	if tool.InputSchema != nil {
		toolMap["function"].(map[string]any)["parameters"] = tool.InputSchema
	}

	return toolMap
}

// TransformTools converts a slice of Wormhole tools to provider format
func (b *RequestBuilder) TransformTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, len(tools))
	for i, tool := range tools {
		result[i] = b.TransformTool(tool)
	}
	return result
}

// TransformToolChoice converts a Wormhole tool choice to provider format
func (b *RequestBuilder) TransformToolChoice(toolChoice *types.ToolChoice) any {
	if toolChoice == nil {
		return nil
	}

	switch toolChoice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return "auto"
	case types.ToolChoiceTypeSpecific:
		if toolChoice.ToolName != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": toolChoice.ToolName,
				},
			}
		}
	}

	return "auto"
}

// BuildEmbeddingsPayload creates a base embeddings request payload
func (b *RequestBuilder) BuildEmbeddingsPayload(
	model string,
	input []string,
) map[string]any {
	// Handle single input
	if len(input) == 1 {
		return map[string]any{
			"model": model,
			"input": input[0],
		}
	}

	// Handle multiple inputs
	return map[string]any{
		"model": model,
		"input": input,
	}
}

// ValidateModelName validates and formats model names
func (b *RequestBuilder) ValidateModelName(model string, expectedPrefixes ...string) (string, error) {
	if model == "" {
		return "", types.NewWormholeError(types.ErrorCodeValidation, "model name is required", false)
	}

	// Check if model matches any expected prefix
	for _, prefix := range expectedPrefixes {
		if strings.HasPrefix(model, prefix) {
			return model, nil
		}
	}

	// If no prefixes provided or model doesn't match, return as-is
	return model, nil
}

// NewRequestBuilder creates a new RequestBuilder instance
func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{}
}
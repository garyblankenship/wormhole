package transform

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// MapFinishReason maps a provider's finish reason string to the canonical FinishReason.
// It handles all known provider-specific aliases (e.g., "end_turn" for Anthropic,
// "STOP" for Gemini) in addition to the standard values.
func MapFinishReason(reason string) types.FinishReason {
	switch strings.ToLower(reason) {
	case "stop", "end_turn":
		return types.FinishReasonStop
	case "length", "max_tokens":
		return types.FinishReasonLength
	case "tool_calls", "function_call", "tool_use":
		return types.FinishReasonToolCalls
	case "content_filter", "safety", "recitation":
		return types.FinishReasonContentFilter
	case "other", "finish_reason_unspecified":
		return types.FinishReasonOther
	default:
		return types.FinishReasonStop
	}
}

// ResponseTransform provides common response transformation utilities
// that can be used across different provider implementations
type ResponseTransform struct{}

// TransformTextResponse transforms a basic text response from provider format
// to Wormhole format. This can be used as a base for provider-specific implementations.
func (t *ResponseTransform) TransformTextResponse(
	id, model string,
	text string,
	usage *types.Usage,
	created time.Time,
	toolCalls []types.ToolCall,
) *types.TextResponse {
	return &types.TextResponse{
		ID:        id,
		Model:     model,
		Text:      text,
		ToolCalls: toolCalls,
		Usage:     usage,
		Created:   created,
	}
}

// ExtractTextFromChoices extracts text from OpenAI-style choices array
func (t *ResponseTransform) ExtractTextFromChoices(choices []map[string]any) string {
	for _, choice := range choices {
		if message, ok := choice["message"].(map[string]any); ok {
			if content, ok := message["content"].(string); ok && content != "" {
				return content
			}
		}
	}
	return ""
}

// ExtractToolCallsFromChoices extracts tool calls from OpenAI-style choices
func (t *ResponseTransform) ExtractToolCallsFromChoices(choices []map[string]any) []types.ToolCall {
	toolCalls := []types.ToolCall{}
	for _, choice := range choices {
		if message, ok := choice["message"].(map[string]any); ok {
			if toolCallSlice, ok := message["tool_calls"].([]any); ok {
				for _, tc := range toolCallSlice {
					if toolCallMap, ok := tc.(map[string]any); ok {
						toolCall := t.ParseToolCallFromMap(toolCallMap)
						if toolCall != nil {
							toolCalls = append(toolCalls, *toolCall)
						}
					}
				}
			}
		}
	}
	return toolCalls
}

// ParseToolCallFromMap parses a tool call from a generic map
func (t *ResponseTransform) ParseToolCallFromMap(toolCallMap map[string]any) *types.ToolCall {
	tc := &types.ToolCall{}

	if id, ok := toolCallMap["id"].(string); ok {
		tc.ID = id
	}
	if typ, ok := toolCallMap["type"].(string); ok {
		tc.Type = typ
	}

	// Parse function call
	if functionMap, ok := toolCallMap["function"].(map[string]any); ok {
		tc.Function = &types.ToolCallFunction{}
		if name, ok := functionMap["name"].(string); ok {
			tc.Function.Name = name
		}
		if arguments, ok := functionMap["arguments"].(string); ok {
			tc.Function.Arguments = arguments
		}
	}

	// Parse raw arguments if function is not present
	if tc.Function == nil {
		if argumentsStr, ok := toolCallMap["arguments"].(string); ok {
			// Try to parse JSON string into map[string]any
			var argsMap map[string]any
			if err := json.Unmarshal([]byte(argumentsStr), &argsMap); err == nil {
				tc.Arguments = argsMap
			}
		}
	}

	return tc
}

// BuildUsageFromTokens creates a Usage object from token counts
func (t *ResponseTransform) BuildUsageFromTokens(promptTokens, completionTokens, totalTokens int) *types.Usage {
	return &types.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

// ParseResponseID extracts an ID from a response map
func (t *ResponseTransform) ParseResponseID(responseMap map[string]any) string {
	if id, ok := responseMap["id"].(string); ok {
		return id
	}
	return fmt.Sprintf("resp-%d", time.Now().UnixNano())
}

// ParseResponseModel extracts a model name from a response map
func (t *ResponseTransform) ParseResponseModel(responseMap map[string]any) string {
	if model, ok := responseMap["model"].(string); ok {
		return model
	}
	return ""
}

// ParseUsageFromMap extracts usage information from a response map
func (t *ResponseTransform) ParseUsageFromMap(responseMap map[string]any) *types.Usage {
	usageMap, ok := responseMap["usage"].(map[string]any)
	if !ok {
		return nil
	}

	usage := &types.Usage{}
	if promptTokens, ok := usageMap["prompt_tokens"].(float64); ok {
		usage.PromptTokens = int(promptTokens)
	}
	if completionTokens, ok := usageMap["completion_tokens"].(float64); ok {
		usage.CompletionTokens = int(completionTokens)
	}
	if totalTokens, ok := usageMap["total_tokens"].(float64); ok {
		usage.TotalTokens = int(totalTokens)
	}
	return usage
}

// ParseTimestampFromMap extracts a timestamp from a response map
func (t *ResponseTransform) ParseTimestampFromMap(responseMap map[string]any) time.Time {
	if created, ok := responseMap["created"].(float64); ok {
		return time.Unix(int64(created), 0)
	}
	if created, ok := responseMap["created"].(int64); ok {
		return time.Unix(created, 0)
	}
	return time.Now()
}

// TransformStructuredResponse creates a structured response from parsed data
func (t *ResponseTransform) TransformStructuredResponse(
	id, model string,
	data any,
	usage *types.Usage,
	created time.Time,
) *types.StructuredResponse {
	return &types.StructuredResponse{
		ID:      id,
		Model:   model,
		Data:    data,
		Usage:   usage,
		Created: created,
	}
}

// TransformEmbeddingsResponse creates an embeddings response
func (t *ResponseTransform) TransformEmbeddingsResponse(
	model string,
	embeddings []types.Embedding,
	usage *types.Usage,
	created time.Time,
) *types.EmbeddingsResponse {
	return &types.EmbeddingsResponse{
		Model:      model,
		Embeddings: embeddings,
		Usage:      usage,
		Created:    created,
	}
}

// BuildEmbeddingFromVector creates an Embedding object from a vector and index
func (t *ResponseTransform) BuildEmbeddingFromVector(index int, vector []float64) types.Embedding {
	return types.Embedding{
		Index:     index,
		Embedding: vector,
	}
}

// LenientUnmarshal attempts to unmarshal JSON, ignoring unknown fields and type mismatches
func (t *ResponseTransform) LenientUnmarshal(data []byte, v any) error {
	// Try standard unmarshal first
	if err := json.Unmarshal(data, v); err != nil {
		// For structured output, we may want to be more lenient
		// For now, just return the error
		return types.NewWormholeError(types.ErrorCodeRequest, "failed to unmarshal", true).WithCause(err)
	}
	return nil
}

// NewResponseTransform creates a new ResponseTransform instance
func NewResponseTransform() *ResponseTransform {
	return &ResponseTransform{}
}

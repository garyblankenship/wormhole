package providers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
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
			"role":         "tool",
			"content":      m.Content,
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
					"name":      tc.Name,
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
		toolMap["function"].(map[string]any)["parameters"] = types.CloneMap(tool.InputSchema)
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

// ToolCallIDSafePattern matches any character outside the charset accepted by
// all supported providers. Such characters are replaced with '_' during ID
// normalization. The accepted charset is [a-zA-Z0-9_-].
var ToolCallIDSafePattern = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)

// ToolCallIDMaxLen is the maximum tool-call ID length accepted by all providers.
const ToolCallIDMaxLen = 64

// normalizeToolCallID replaces every character outside the shared safe charset
// with '_' and truncates the result to ToolCallIDMaxLen. It is a no-op for IDs
// that are already valid.
func normalizeToolCallID(id string) string {
	normalized := ToolCallIDSafePattern.ReplaceAllString(id, "_")
	if len(normalized) > ToolCallIDMaxLen {
		normalized = normalized[:ToolCallIDMaxLen]
	}
	return normalized
}

// PrepareMessages validates and repairs tool-call conversation history before
// provider-specific serialization. It returns a copied slice — the caller-owned
// input is never mutated.
//
// Repair rules:
//   - Text content is sanitized to valid UTF-8.
//   - Missing tool-call IDs on assistant messages are synthesized.
//   - Tool-call IDs are normalized to the shared safe charset; tool-result IDs
//     are updated to match.
//   - Duplicate normalized tool-call IDs within an assistant message produce an error.
//   - Orphaned tool calls (no matching tool result) are dropped from the
//     assistant message; a warning is returned. The assistant message itself is kept.
//   - Stranded tool results (no matching tool call) are dropped from the slice;
//     a warning is returned.
//
// Returns the repaired slice, a list of human-readable warning strings for any
// dropped entities (nil if none), and an error for hard constraint violations.
func PrepareMessages(messages []types.Message) ([]types.Message, []string, error) {
	if len(messages) == 0 {
		return nil, nil, nil
	}

	prepared, normalizedIDs, err := prepareMessageCopies(messages)
	if err != nil {
		return nil, nil, err
	}
	normalizeToolResultIDs(prepared, normalizedIDs)
	callIDs, resultIDs := collectToolMessageIDs(prepared)
	return filterUnmatchedToolMessages(prepared, callIDs, resultIDs)
}

func prepareMessageCopies(messages []types.Message) ([]types.Message, map[string]string, error) {
	prepared := types.CloneMessages(messages)
	normalizedIDs := make(map[string]string)
	for i, message := range prepared {
		switch message := message.(type) {
		case *types.AssistantMessage:
			if err := prepareAssistantMessage(message, i, normalizedIDs); err != nil {
				return nil, nil, err
			}
		case *types.ToolResultMessage:
			message.Content = strings.ToValidUTF8(message.Content, "")
		case *types.UserMessage:
			message.Content = strings.ToValidUTF8(message.Content, "")
		case *types.SystemMessage:
			message.Content = strings.ToValidUTF8(message.Content, "")
		}
	}
	return prepared, normalizedIDs, nil
}

func prepareAssistantMessage(message *types.AssistantMessage, messageIndex int, normalizedIDs map[string]string) error {
	message.Content = strings.ToValidUTF8(message.Content, "")
	messageIDs := make(map[string]struct{}, len(message.ToolCalls))
	for i := range message.ToolCalls {
		toolCall := message.ToolCalls[i]
		if toolCall.ID == "" {
			toolCall.ID = fmt.Sprintf("synth_%d_%d", messageIndex, i)
		}
		originalID := toolCall.ID
		toolCall.ID = normalizeToolCallID(toolCall.ID)
		normalized, err := types.NormalizeToolCall(toolCall)
		if err != nil {
			return fmt.Errorf("assistant message at index %d: %w", messageIndex, err)
		}
		if _, duplicate := messageIDs[normalized.ID]; duplicate {
			return fmt.Errorf("duplicate tool-call ID %q in assistant message at index %d", normalized.ID, messageIndex)
		}
		messageIDs[normalized.ID] = struct{}{}
		normalizedIDs[originalID] = normalized.ID
		message.ToolCalls[i] = normalized
	}
	return nil
}

func normalizeToolResultIDs(messages []types.Message, normalizedIDs map[string]string) {
	for _, message := range messages {
		result, ok := message.(*types.ToolResultMessage)
		if !ok {
			continue
		}
		if normalized, found := normalizedIDs[result.ToolCallID]; found {
			result.ToolCallID = normalized
			continue
		}
		result.ToolCallID = normalizeToolCallID(result.ToolCallID)
	}
}

func collectToolMessageIDs(messages []types.Message) (map[string]struct{}, map[string]struct{}) {
	callIDs := make(map[string]struct{})
	resultIDs := make(map[string]struct{})
	for _, message := range messages {
		switch message := message.(type) {
		case *types.AssistantMessage:
			for _, toolCall := range message.ToolCalls {
				callIDs[toolCall.ID] = struct{}{}
			}
		case *types.ToolResultMessage:
			resultIDs[message.ToolCallID] = struct{}{}
		}
	}
	return callIDs, resultIDs
}

func filterUnmatchedToolMessages(messages []types.Message, callIDs, resultIDs map[string]struct{}) ([]types.Message, []string, error) {
	warnings := make([]string, 0)
	repaired := make([]types.Message, 0, len(messages))
	for i, message := range messages {
		switch message := message.(type) {
		case *types.AssistantMessage:
			kept, droppedWarnings := matchedToolCalls(message.ToolCalls, resultIDs, i)
			message.ToolCalls = kept
			warnings = append(warnings, droppedWarnings...)
			repaired = append(repaired, message)
		case *types.ToolResultMessage:
			if _, matched := callIDs[message.ToolCallID]; matched {
				repaired = append(repaired, message)
				continue
			}
			warnings = append(warnings, fmt.Sprintf("dropped stranded tool result %s at index %d", message.ToolCallID, i))
		default:
			repaired = append(repaired, message)
		}
	}
	return repaired, warnings, nil
}

func matchedToolCalls(toolCalls []types.ToolCall, resultIDs map[string]struct{}, messageIndex int) ([]types.ToolCall, []string) {
	if len(toolCalls) == 0 {
		return toolCalls, nil
	}
	kept := make([]types.ToolCall, 0, len(toolCalls))
	var warnings []string
	for _, toolCall := range toolCalls {
		if _, matched := resultIDs[toolCall.ID]; matched {
			kept = append(kept, toolCall)
			continue
		}
		warnings = append(warnings, fmt.Sprintf("dropped orphaned tool call %s at assistant message index %d", toolCall.ID, messageIndex))
	}
	return kept, warnings
}

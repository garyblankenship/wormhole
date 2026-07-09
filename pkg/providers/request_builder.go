package providers

import (
	"encoding/json"
	"fmt"
	"regexp"
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

	// Build a copy.
	out := make([]types.Message, len(messages))

	// First pass: repair assistant messages and collect normalized IDs.
	normalizedIDs := make(map[string]string) // original → normalized

	for i, msg := range messages {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			repaired := *m
			repaired.Content = strings.ToValidUTF8(repaired.Content, "")

			if len(repaired.ToolCalls) > 0 {
				repaired.ToolCalls = make([]types.ToolCall, len(m.ToolCalls))
				copy(repaired.ToolCalls, m.ToolCalls)

				// Create a fresh set for each assistant message.
				msgIDSet := make(map[string]struct{}, len(repaired.ToolCalls))

				for j := range repaired.ToolCalls {
					tc := &repaired.ToolCalls[j]
					if tc.Function != nil {
						fc := *tc.Function
						tc.Function = &fc
					}

					// Synthesize missing ID.
					if tc.ID == "" {
						tc.ID = fmt.Sprintf("synth_%d_%d", i, j)
					}

					// Normalize to the shared safe charset before any matching.
					original := tc.ID
					tc.ID = normalizeToolCallID(tc.ID)

					// Check duplicates within this assistant message (post-normalization;
					// two distinct originals that normalize to the same ID collide).
					if _, dup := msgIDSet[tc.ID]; dup {
						return nil, nil, fmt.Errorf("duplicate tool-call ID %q in assistant message at index %d", tc.ID, i)
					}
					msgIDSet[tc.ID] = struct{}{}
					normalizedIDs[original] = tc.ID
				}
			}

			out[i] = &repaired

		case *types.ToolResultMessage:
			repaired := *m
			repaired.Content = strings.ToValidUTF8(repaired.Content, "")
			out[i] = &repaired

		case *types.UserMessage:
			repaired := *m
			repaired.Content = strings.ToValidUTF8(repaired.Content, "")
			out[i] = &repaired

		case *types.SystemMessage:
			repaired := *m
			repaired.Content = strings.ToValidUTF8(repaired.Content, "")
			out[i] = &repaired

		default:
			out[i] = msg
		}
	}

	// Second pass: normalize tool-result IDs so they match the normalized
	// tool-call IDs produced in the first pass.
	for i, msg := range out {
		if tr, ok := msg.(*types.ToolResultMessage); ok {
			repaired := *tr
			if normalized, found := normalizedIDs[repaired.ToolCallID]; found {
				repaired.ToolCallID = normalized
			} else {
				repaired.ToolCallID = normalizeToolCallID(repaired.ToolCallID)
			}
			out[i] = &repaired
		}
	}

	// Third pass: drop orphaned tool calls and stranded tool results.
	// Matching is on the already-normalized IDs in `out`.
	resultIDs := make(map[string]struct{}) // normalized tool-result IDs present
	callIDs := make(map[string]struct{})   // normalized tool-call IDs present
	for _, msg := range out {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			for _, tc := range m.ToolCalls {
				callIDs[tc.ID] = struct{}{}
			}
		case *types.ToolResultMessage:
			resultIDs[m.ToolCallID] = struct{}{}
		}
	}

	var warnings []string
	repaired := make([]types.Message, 0, len(out))
	for i, msg := range out {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			if len(m.ToolCalls) == 0 {
				repaired = append(repaired, msg)
				continue
			}
			kept := make([]types.ToolCall, 0, len(m.ToolCalls))
			dropped := false
			for _, tc := range m.ToolCalls {
				if _, matched := resultIDs[tc.ID]; matched {
					kept = append(kept, tc)
					continue
				}
				dropped = true
				warnings = append(warnings, fmt.Sprintf("dropped orphaned tool call %s at assistant message index %d", tc.ID, i))
			}
			if !dropped {
				repaired = append(repaired, msg)
				continue
			}
			am := *m
			am.ToolCalls = kept
			repaired = append(repaired, &am)
		case *types.ToolResultMessage:
			if _, matched := callIDs[m.ToolCallID]; matched {
				repaired = append(repaired, msg)
				continue
			}
			warnings = append(warnings, fmt.Sprintf("dropped stranded tool result %s at index %d", m.ToolCallID, i))
		default:
			repaired = append(repaired, msg)
		}
	}

	return repaired, warnings, nil
}

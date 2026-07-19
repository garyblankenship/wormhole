package providers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

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

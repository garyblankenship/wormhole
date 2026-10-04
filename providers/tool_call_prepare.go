package providers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
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
	return filterUnmatchedToolMessages(prepared)
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

// filterUnmatchedToolMessages pairs each result with one earlier unmatched call.
// Occurrences, rather than global IDs, make later reuse and ordering explicit.
func filterUnmatchedToolMessages(messages []types.Message) ([]types.Message, []string, error) {
	type occurrence struct{ message, call int }
	pending := make(map[string][]occurrence)
	matched := make(map[occurrence]bool)
	keptResults := make(map[int]bool)
	for i, message := range messages {
		switch m := message.(type) {
		case *types.AssistantMessage:
			for j, call := range m.ToolCalls {
				pending[call.ID] = append(pending[call.ID], occurrence{i, j})
			}
		case *types.ToolResultMessage:
			queue := pending[m.ToolCallID]
			if len(queue) > 0 {
				matched[queue[0]] = true
				keptResults[i] = true
				pending[m.ToolCallID] = queue[1:]
			}
		}
	}
	var warnings []string
	repaired := make([]types.Message, 0, len(messages))
	for i, message := range messages {
		switch m := message.(type) {
		case *types.AssistantMessage:
			kept := make([]types.ToolCall, 0, len(m.ToolCalls))
			for j, call := range m.ToolCalls {
				if matched[occurrence{i, j}] {
					kept = append(kept, call)
				} else {
					warnings = append(warnings, fmt.Sprintf("dropped orphaned tool call %s at assistant message index %d", call.ID, i))
				}
			}
			if len(m.ToolCalls) > 0 {
				m.ToolCalls = kept
			}
			repaired = append(repaired, m)
		case *types.ToolResultMessage:
			if keptResults[i] {
				repaired = append(repaired, m)
			} else {
				warnings = append(warnings, fmt.Sprintf("dropped stranded tool result %s at index %d", m.ToolCallID, i))
			}
		default:
			repaired = append(repaired, m)
		}
	}
	return repaired, warnings, nil
}

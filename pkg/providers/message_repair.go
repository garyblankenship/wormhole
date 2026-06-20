package providers

import (
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ValidateMessageSequence runs the same orphan/stranded detection as
// PrepareMessages but returns the defects as warning strings rather than
// repairing them. Use it to surface sequence problems (for logging or metrics)
// before — or instead of — calling PrepareMessages. It does not copy or mutate
// the input slice.
//
// The returned warnings use the same wording PrepareMessages emits for the
// corresponding repairs. The error return mirrors PrepareMessages' hard
// constraint: a duplicate normalized tool-call ID within one assistant message.
func ValidateMessageSequence(messages []types.Message) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Collect normalized tool-call IDs (per-assistant duplicate check) and the
	// sets of all normalized call/result IDs present.
	callIDs := make(map[string]struct{})
	resultIDs := make(map[string]struct{})

	for i, msg := range messages {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			seen := make(map[string]struct{}, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				id := tc.ID
				if id == "" {
					id = fmt.Sprintf("synth_%d_%d", i, j)
				}
				id = normalizeToolCallID(id)
				if _, dup := seen[id]; dup {
					return nil, fmt.Errorf("duplicate tool-call ID %q in assistant message at index %d", id, i)
				}
				seen[id] = struct{}{}
				callIDs[id] = struct{}{}
			}
		case *types.ToolResultMessage:
			resultIDs[normalizeToolCallID(m.ToolCallID)] = struct{}{}
		}
	}

	var warnings []string
	for i, msg := range messages {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			for j, tc := range m.ToolCalls {
				id := tc.ID
				if id == "" {
					id = fmt.Sprintf("synth_%d_%d", i, j)
				}
				id = normalizeToolCallID(id)
				if _, matched := resultIDs[id]; !matched {
					warnings = append(warnings, fmt.Sprintf("dropped orphaned tool call %s at assistant message index %d", id, i))
				}
			}
		case *types.ToolResultMessage:
			id := normalizeToolCallID(m.ToolCallID)
			if _, matched := callIDs[id]; !matched {
				warnings = append(warnings, fmt.Sprintf("dropped stranded tool result %s at index %d", id, i))
			}
		}
	}

	return warnings, nil
}

package providers

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMessageSequence_Empty(t *testing.T) {
	t.Parallel()
	warnings, err := ValidateMessageSequence(nil)
	require.NoError(t, err)
	assert.Nil(t, warnings)
}

func TestValidateMessageSequence_CleanSequence(t *testing.T) {
	t.Parallel()
	input := []types.Message{
		&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "call_1", Name: "search"}}},
		types.NewToolResultMessage("call_1", "ok"),
	}
	warnings, err := ValidateMessageSequence(input)
	require.NoError(t, err)
	assert.Empty(t, warnings, "clean sequence reports no defects")
}

func TestValidateMessageSequence_ReportsOrphanAndStranded(t *testing.T) {
	t.Parallel()
	input := []types.Message{
		&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "orphan_call", Name: "a"}}},
		types.NewToolResultMessage("ghost_result", "x"),
	}
	warnings, err := ValidateMessageSequence(input)
	require.NoError(t, err)
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "orphaned tool call")
	assert.Contains(t, warnings[0], "orphan_call")
	assert.Contains(t, warnings[1], "stranded tool result")
	assert.Contains(t, warnings[1], "ghost_result")
}

func TestValidateMessageSequence_DoesNotMutate(t *testing.T) {
	t.Parallel()
	am := &types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "orphan_call", Name: "a"}}}
	input := []types.Message{am}
	_, err := ValidateMessageSequence(input)
	require.NoError(t, err)
	// Input slice and message untouched (read-only).
	require.Len(t, am.ToolCalls, 1)
	assert.Equal(t, "orphan_call", am.ToolCalls[0].ID)
}

func TestValidateMessageSequence_DuplicateIDError(t *testing.T) {
	t.Parallel()
	input := []types.Message{
		&types.AssistantMessage{ToolCalls: []types.ToolCall{
			{ID: "a:b", Name: "x"},
			{ID: "a;b", Name: "y"},
		}},
	}
	_, err := ValidateMessageSequence(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool-call ID")
}

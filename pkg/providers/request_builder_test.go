package providers

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareMessages_EmptyInput(t *testing.T) {
	t.Parallel()

	result, _, err := PrepareMessages(nil)
	require.NoError(t, err)
	assert.Nil(t, result)

	result, _, err = PrepareMessages([]types.Message{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestPrepareMessages_SynthesizesMissingToolCallIDs(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		types.NewUserMessage("hi"),
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{
				{ID: "", Name: "get_weather", Function: &types.ToolCallFunction{Name: "get_weather", Arguments: "{}"}},
			},
		},
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 2)

	am, ok := result[1].(*types.AssistantMessage)
	require.True(t, ok)
	assert.NotEmpty(t, am.ToolCalls[0].ID, "missing tool-call ID should be synthesized")
	assert.Contains(t, am.ToolCalls[0].ID, "synth_")
}

func TestPrepareMessages_RejectsDuplicateToolCallIDs(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		types.NewUserMessage("hi"),
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{
				{ID: "call_1", Name: "a"},
				{ID: "call_1", Name: "b"},
			},
		},
	}

	_, _, err := PrepareMessages(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool-call ID")
}

func TestPrepareMessages_SanitizesInvalidUTF8(t *testing.T) {
	t.Parallel()

	invalidUTF8 := "hello\xffworld"

	input := []types.Message{
		&types.UserMessage{Content: invalidUTF8},
		&types.AssistantMessage{Content: invalidUTF8},
		&types.SystemMessage{Content: invalidUTF8},
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 3)

	um, ok := result[0].(*types.UserMessage)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(um.Content), "user message should be valid UTF-8")

	am, ok := result[1].(*types.AssistantMessage)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(am.Content), "assistant message should be valid UTF-8")

	sm, ok := result[2].(*types.SystemMessage)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(sm.Content), "system message should be valid UTF-8")
}

func TestPrepareMessages_ToolResultUTF8Sanitized(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		types.NewToolResultMessage("call_1", "result\xffdata"),
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 1)

	tr, ok := result[0].(*types.ToolResultMessage)
	require.True(t, ok)
	assert.True(t, utf8.ValidString(tr.Content), "tool result should be valid UTF-8")
}

func TestPrepareMessages_DoesNotMutateCallerSlice(t *testing.T) {
	t.Parallel()

	original := &types.AssistantMessage{
		Content:   "original",
		ToolCalls: []types.ToolCall{{ID: "", Name: "test"}},
	}

	input := []types.Message{
		types.NewUserMessage("hi"),
		original,
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)

	// Original should be untouched
	assert.Empty(t, original.ToolCalls[0].ID, "caller-owned message should not be mutated")
	assert.Equal(t, "original", original.Content)

	// Result should have repaired version
	am, ok := result[1].(*types.AssistantMessage)
	require.True(t, ok)
	assert.NotEmpty(t, am.ToolCalls[0].ID, "prepared copy should have synthesized ID")
}

func TestPrepareMessages_PreservesValidMessages(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		types.NewSystemMessage("You are helpful."),
		types.NewUserMessage("Hello"),
		&types.AssistantMessage{
			Content:   "Hi there",
			ToolCalls: []types.ToolCall{{ID: "call_abc", Name: "search"}},
		},
		types.NewToolResultMessage("call_abc", "results"),
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 4)

	// Types preserved
	assert.IsType(t, &types.SystemMessage{}, result[0])
	assert.IsType(t, &types.UserMessage{}, result[1])
	assert.IsType(t, &types.AssistantMessage{}, result[2])
	assert.IsType(t, &types.ToolResultMessage{}, result[3])

	// Values preserved
	am, _ := result[2].(*types.AssistantMessage)
	assert.Equal(t, "call_abc", am.ToolCalls[0].ID)
}

func TestPrepareMessages_AllowsUniqueIDsAcrossAssistantMessages(t *testing.T) {
	t.Parallel()

	// Same ID in different assistant messages is allowed (they're separate tool calls)
	input := []types.Message{
		types.NewUserMessage("hi"),
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{{ID: "call_1", Name: "a"}},
		},
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{{ID: "call_1", Name: "b"}},
		},
	}

	// This should succeed — duplicates are only rejected within the same assistant message
	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 3)
}

func TestNormalizeToolCallID_ValidPassthrough(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "call_abc-123", normalizeToolCallID("call_abc-123"))
}

func TestNormalizeToolCallID_ColonReplaced(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "toolu_01_foo", normalizeToolCallID("toolu_01:foo"))
}

func TestNormalizeToolCallID_TruncatedAt64(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100)
	got := normalizeToolCallID(long)
	assert.Len(t, got, 64)
	assert.Equal(t, strings.Repeat("a", 64), got)
}

func TestPrepareMessages_NormalizesIDsBeforeMatching(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{{ID: "toolu_01:x", Name: "search"}},
		},
		types.NewToolResultMessage("toolu_01:x", "result"),
	}

	result, _, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, result, 2)

	am, ok := result[0].(*types.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "toolu_01_x", am.ToolCalls[0].ID, "tool-call ID should be normalized")

	tr, ok := result[1].(*types.ToolResultMessage)
	require.True(t, ok)
	assert.Equal(t, "toolu_01_x", tr.ToolCallID, "tool-result ID should be normalized to match")
}

func TestPrepareMessages_CollisionAfterNormalization(t *testing.T) {
	t.Parallel()

	// "a:b" and "a-b" both normalize to "a_b"... actually "a-b" stays "a-b" (hyphen is safe).
	// Use two originals that BOTH normalize to the same value: "a:b" -> "a_b" and "a;b" -> "a_b".
	input := []types.Message{
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{
				{ID: "a:b", Name: "x"},
				{ID: "a;b", Name: "y"},
			},
		},
	}

	_, _, err := PrepareMessages(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool-call ID")
}

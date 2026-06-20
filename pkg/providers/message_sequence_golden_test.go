package providers

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareMessages_AnthropicOriginThroughOpenAIPath is the headline golden
// test: a conversation born on Anthropic (tool-call IDs containing a colon, plus
// one orphaned tool call with no matching result) is routed to an OpenAI target.
// PrepareMessages must normalize the IDs to the OpenAI-safe charset, drop the
// orphaned call, keep the matched call + its result, and report the repair in
// warnings — producing a slice OpenAI accepts.
func TestPrepareMessages_AnthropicOriginThroughOpenAIPath(t *testing.T) {
	t.Parallel()

	input := []types.Message{
		types.NewUserMessage("what's the weather and the time?"),
		&types.AssistantMessage{
			Content: "let me check both",
			ToolCalls: []types.ToolCall{
				{ID: "toolu_01:weather", Name: "get_weather"},
				{ID: "toolu_01:orphan", Name: "get_time"},
			},
		},
		// Only the weather call gets a result; get_time is orphaned.
		types.NewToolResultMessage("toolu_01:weather", "sunny, 72F"),
	}

	repaired, warnings, err := PrepareMessages(input)
	require.NoError(t, err)
	require.Len(t, repaired, 3, "user, assistant, and the matched tool result remain")

	// IDs normalized to the OpenAI-safe charset (colon -> underscore).
	am, ok := repaired[1].(*types.AssistantMessage)
	require.True(t, ok)
	require.Len(t, am.ToolCalls, 1, "orphaned get_time call dropped, weather call kept")
	assert.Equal(t, "toolu_01_weather", am.ToolCalls[0].ID)
	for _, tc := range am.ToolCalls {
		assert.Regexp(t, `^[a-zA-Z0-9_-]+$`, tc.ID, "kept tool-call ID must be OpenAI-safe")
		assert.LessOrEqual(t, len(tc.ID), 64)
	}
	assert.Equal(t, "let me check both", am.Content, "assistant text preserved")

	// The tool result ID is normalized to match the kept call.
	tr, ok := repaired[2].(*types.ToolResultMessage)
	require.True(t, ok)
	assert.Equal(t, "toolu_01_weather", tr.ToolCallID)
	assert.Regexp(t, `^[a-zA-Z0-9_-]+$`, tr.ToolCallID)

	// The orphaned call is reported in warnings (normalized ID form).
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "orphaned tool call")
	assert.Contains(t, warnings[0], "toolu_01_orphan")
}

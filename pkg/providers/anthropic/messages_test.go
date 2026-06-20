package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FIX 1: buildContent must not panic when a ToolCall has a nil Function
// (Gemini-origin and public map-form calls populate only the top-level
// Name/Arguments fields).
func TestBuildContent_ToolCall_NilFunction(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.AssistantMessage{
		Content: "",
		ToolCalls: []types.ToolCall{
			{ID: "x", Name: "get_weather", Arguments: map[string]any{"city": "NYC"}},
		},
	}

	var parts []map[string]any
	require.NotPanics(t, func() {
		parts = p.buildContent(msg)
	})

	var toolUse map[string]any
	for _, part := range parts {
		if part["type"] == "tool_use" {
			toolUse = part
			break
		}
	}
	require.NotNil(t, toolUse, "expected a tool_use block")
	assert.Equal(t, "get_weather", toolUse["name"])
	assert.Equal(t, "x", toolUse["id"])
	input, ok := toolUse["input"].(map[string]any)
	require.True(t, ok, "input should be a map")
	assert.Equal(t, "NYC", input["city"])
}

// FIX 1 regression guard: the OpenAI-form ToolCall (populated *Function with
// JSON-string Arguments) must still build a correct tool_use block.
func TestBuildContent_ToolCall_PopulatedFunction(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.AssistantMessage{
		Content: "",
		ToolCalls: []types.ToolCall{
			{
				ID: "y",
				Function: &types.ToolCallFunction{
					Name:      "get_weather",
					Arguments: `{"city":"NYC"}`,
				},
			},
		},
	}

	var parts []map[string]any
	require.NotPanics(t, func() {
		parts = p.buildContent(msg)
	})

	var toolUse map[string]any
	for _, part := range parts {
		if part["type"] == "tool_use" {
			toolUse = part
			break
		}
	}
	require.NotNil(t, toolUse, "expected a tool_use block")
	assert.Equal(t, "get_weather", toolUse["name"])
	assert.Equal(t, "y", toolUse["id"])
	input, ok := toolUse["input"].(map[string]any)
	require.True(t, ok, "input should be a map")
	assert.Equal(t, "NYC", input["city"])
}

// FIX 2: a tool-result message must build exactly one tool_result block
// (NOT a text block with tool_use_id), per Anthropic's wire format.
func TestBuildContent_ToolResult(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.ToolMessage{
		Content:    "72F and sunny",
		ToolCallID: "call_abc",
	}

	parts := p.buildContent(msg)
	require.Len(t, parts, 1, "expected exactly one block")
	block := parts[0]
	assert.Equal(t, "tool_result", block["type"])
	assert.Equal(t, "call_abc", block["tool_use_id"])
	assert.Equal(t, "72F and sunny", block["content"])
	_, hasText := block["text"]
	assert.False(t, hasText, "tool_result block must not carry a text field")
}

// FIX (coalesce): consecutive messages mapping to the same Anthropic role must
// merge into ONE role-turn carrying both content blocks. A tool-result message
// (RoleTool -> "user") followed by a real user message must NOT produce two
// adjacent "user" entries (Anthropic 400s on non-alternating roles).
func TestTransformMessages_CoalescesConsecutiveUserRole(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msgs := []types.Message{
		&types.AssistantMessage{
			Content: "",
			ToolCalls: []types.ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "NYC"}},
			},
		},
		&types.ToolResultMessage{ToolCallID: "call_1", Content: "72F and sunny"},
		types.NewUserMessage("next question"),
	}

	out := p.transformMessages(msgs)

	require.Len(t, out, 2, "expected one assistant turn + one merged user turn")
	assert.Equal(t, "assistant", out[0]["role"])
	assert.Equal(t, roleUser, out[1]["role"])

	userBlocks, ok := out[1]["content"].([]map[string]any)
	require.True(t, ok, "merged user content must be []map[string]any")
	require.Len(t, userBlocks, 2, "tool_result block + user text block must both be present in the single user turn")
	assert.Equal(t, "tool_result", userBlocks[0]["type"])
	assert.Equal(t, "call_1", userBlocks[0]["tool_use_id"])
	assert.Equal(t, contentTypeText, userBlocks[1]["type"])
	assert.Equal(t, "next question", userBlocks[1]["text"])

	// No two adjacent entries share a role.
	for i := 1; i < len(out); i++ {
		assert.NotEqual(t, out[i-1]["role"], out[i]["role"], "no two adjacent messages may share a role")
	}
}

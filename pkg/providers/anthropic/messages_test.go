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

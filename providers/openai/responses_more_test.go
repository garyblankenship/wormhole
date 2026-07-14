package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

// responseFunctionCallToToolCall must flag a malformed tool-call arguments JSON
// (ArgsInvalid + nil Arguments + parse error) on the Responses-API parse path,
// matching the Chat-transform contract enforced via types.MarkArgsError.
func TestResponseFunctionCallToToolCall_MalformedArgs(t *testing.T) {
	t.Parallel()

	tc := responseFunctionCallToToolCall(responsesOutputItem{
		CallID:    "call_1",
		Name:      "do_thing",
		Arguments: `{"x": `, // truncated -> invalid JSON
	})

	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "do_thing", tc.Name)
	assert.True(t, tc.ArgsInvalid, "malformed args must set ArgsInvalid")
	assert.Nil(t, tc.Arguments, "malformed args must clear Arguments to nil")
	assert.NotEmpty(t, tc.ArgsParseError, "malformed args must record parse error")
	require.NotNil(t, tc.Function)
	assert.Equal(t, `{"x": `, tc.Function.Arguments, "raw fragment retained on Function.Arguments")
}

func TestResponseFunctionCallToToolCall_ValidArgs(t *testing.T) {
	t.Parallel()

	tc := responseFunctionCallToToolCall(responsesOutputItem{
		CallID:    "call_2",
		Name:      "do_thing",
		Arguments: `{"x":1}`,
	})

	assert.Equal(t, "call_2", tc.ID)
	assert.False(t, tc.ArgsInvalid)
	assert.Empty(t, tc.ArgsParseError)
	require.NotNil(t, tc.Arguments)
	assert.Equal(t, float64(1), tc.Arguments["x"])
	_ = types.ToolCall{} // keep types import explicit alongside ToolCall fields under test
}

func TestResponseFunctionCallToToolCall_EmptyArgsAndIDFallback(t *testing.T) {
	t.Parallel()

	tc := responseFunctionCallToToolCall(responsesOutputItem{
		ID:        "id_fallback",
		Name:      "do_thing",
		Arguments: "",
	})

	assert.Equal(t, "id_fallback", tc.ID, "CallID empty -> falls back to ID")
	assert.False(t, tc.ArgsInvalid)
	assert.Empty(t, tc.ArgsParseError)
	assert.Equal(t, map[string]any{}, tc.Arguments, "empty args -> empty map (emptyVal)")
}

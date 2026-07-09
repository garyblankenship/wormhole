package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformTextResponse_ToolUsePopulatesArgumentsMap(t *testing.T) {
	t.Parallel()
	resp := &messageResponse{
		Content: []contentPart{
			{Type: contentTypeToolUse, ID: "tu_1", Name: "get_weather", Input: toolInput{"city": "sf"}},
		},
	}
	out := (&Provider{}).transformTextResponse(resp)
	require.Len(t, out.ToolCalls, 1)
	require.NotNil(t, out.ToolCalls[0].Arguments, "non-streaming tool_use must populate the Arguments map (parity with streaming)")
	assert.Equal(t, "sf", out.ToolCalls[0].Arguments["city"])
}

package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestGemini() *Gemini {
	return New("test-key", types.ProviderConfig{})
}

func TestTransformMessages_SkipsSystemRole(t *testing.T) {
	t.Parallel()

	g := newTestGemini()
	contents, err := g.transformMessages([]types.Message{
		types.NewSystemMessage("you are helpful"),
		types.NewUserMessage("hello"),
	}, "")
	require.NoError(t, err)

	require.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0]["role"])
}

func TestMergeSystemInstruction_MergesMessagesAndPrompt(t *testing.T) {
	t.Parallel()

	msgs := []types.Message{
		types.NewSystemMessage("from messages"),
		types.NewUserMessage("hi"),
	}

	assert.Equal(t, "from messages", mergeSystemInstruction("", msgs))
	assert.Equal(t, "base prompt\n\nfrom messages", mergeSystemInstruction("base prompt", msgs))
	assert.Equal(t, "base prompt", mergeSystemInstruction("base prompt", []types.Message{types.NewUserMessage("hi")}))
	assert.Equal(t, "", mergeSystemInstruction("", []types.Message{types.NewUserMessage("hi")}))
}

func TestBuildTextPayload_SystemMessageBecomesSystemInstruction(t *testing.T) {
	t.Parallel()

	g := newTestGemini()
	payload, err := g.buildTextPayload(types.TextRequest{
		Messages: []types.Message{
			types.NewSystemMessage("you are helpful"),
			types.NewUserMessage("hello"),
		},
	})
	require.NoError(t, err)

	si, ok := payload["systemInstruction"].(map[string]any)
	require.True(t, ok)
	parts, ok := si["parts"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	assert.Equal(t, "you are helpful", parts[0]["text"])

	contents, ok := payload["contents"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0]["role"])
}

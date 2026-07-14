package ollama

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestTransformMessagesIncludesUserMediaImages(t *testing.T) {
	t.Parallel()

	p := &Provider{}
	const b64 = "aGVsbG8="

	msgs := []types.Message{
		&types.UserMessage{
			Content: "describe this",
			Media: []types.Media{
				&types.ImageMedia{Base64Data: b64, MimeType: "image/png"},
			},
		},
	}

	out := p.transformMessages(msgs, "")
	require.Len(t, out, 1)
	require.Len(t, out[0].Images, 1, "UserMessage.Media image must reach Ollama Images")
	assert.Equal(t, b64, out[0].Images[0])
	assert.Equal(t, "describe this", out[0].Content)
}

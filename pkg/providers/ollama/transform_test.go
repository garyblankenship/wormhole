package ollama

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaTransformMessages_CallsPrepareMessages(t *testing.T) {
	t.Parallel()

	provider, err := New(types.ProviderConfig{BaseURL: "http://localhost:11434"})
	require.NoError(t, err)

	// A stranded tool result (no preceding matching tool call) must be dropped
	// by PrepareMessages before the payload is built. If PrepareMessages were
	// not wired in, the stranded result would survive and the payload would
	// carry both messages.
	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "llama2"},
		Messages: []types.Message{
			types.NewUserMessage("hello"),
			types.NewToolResultMessage("ghost-id", "stranded result"),
		},
	}

	payload := provider.buildChatPayload(request)

	// No system prompt, so payload messages == prepared messages.
	// The stranded tool result is dropped → only the user message remains.
	require.Len(t, payload.Messages, 1, "stranded tool result should be dropped by PrepareMessages")
	assert.Equal(t, roleUser, payload.Messages[0].Role)
}

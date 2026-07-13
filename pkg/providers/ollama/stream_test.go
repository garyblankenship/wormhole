package ollama

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamChunkFallback(t *testing.T) {
	t.Parallel()

	provider, err := New(types.ProviderConfig{BaseURL: "http://127.0.0.1:11434"})
	require.NoError(t, err)
	provider.streamingTransformer = nil

	_, err = provider.parseStreamChunk([]byte("{"))
	require.Error(t, err)

	chunk, err := provider.parseStreamChunk([]byte(`{
		"model":"llama3",
		"message":{"role":"assistant","content":"hello"},
		"done":false
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "llama3", chunk.Model)
	require.NotNil(t, chunk.Delta)
	assert.Equal(t, "hello", chunk.Delta.Content)
	assert.Nil(t, chunk.FinishReason)
	assert.Nil(t, chunk.Usage)

	chunk, err = provider.parseStreamChunk([]byte(`{
		"model":"llama3",
		"message":{"role":"assistant","content":{"part":"done"}},
		"done":true,
		"prompt_eval_count":3,
		"eval_count":4
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.NotNil(t, chunk.Delta)
	assert.Equal(t, "map[part:done]", chunk.Delta.Content)
	require.NotNil(t, chunk.FinishReason)
	assert.Equal(t, types.FinishReasonOther, *chunk.FinishReason)
	require.NotNil(t, chunk.Usage)
	assert.Equal(t, 7, chunk.Usage.TotalTokens)
}

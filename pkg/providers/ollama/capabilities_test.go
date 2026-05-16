package ollama

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSupportedCapabilities(t *testing.T) {
	t.Parallel()

	provider, err := New(types.ProviderConfig{BaseURL: "http://127.0.0.1:11434"})
	require.NoError(t, err)

	capabilities := provider.SupportedCapabilities()
	require.Len(t, capabilities, 5)
	assert.Contains(t, capabilities, types.CapabilityText)
	assert.Contains(t, capabilities, types.CapabilityChat)
	assert.Contains(t, capabilities, types.CapabilityStructured)
	assert.Contains(t, capabilities, types.CapabilityEmbeddings)
	assert.Contains(t, capabilities, types.CapabilityStream)
	assert.NotContains(t, capabilities, types.CapabilityImages)
}

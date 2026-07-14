package gemini_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/providers/gemini"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderSupportedCapabilities(t *testing.T) {
	t.Parallel()

	provider := gemini.New("test-key", types.ProviderConfig{})
	capabilities := provider.SupportedCapabilities()

	require.Len(t, capabilities, 7)
	assert.Contains(t, capabilities, types.CapabilityText)
	assert.Contains(t, capabilities, types.CapabilityChat)
	assert.Contains(t, capabilities, types.CapabilityStructured)
	assert.Contains(t, capabilities, types.CapabilityEmbeddings)
	assert.Contains(t, capabilities, types.CapabilityImages)
	assert.Contains(t, capabilities, types.CapabilityStream)
	assert.Contains(t, capabilities, types.CapabilityFunctions)
}

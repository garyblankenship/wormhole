package gemini_test

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSupportedCapabilities(t *testing.T) {
	t.Parallel()

	provider := gemini.New("test-key", types.ProviderConfig{})
	capabilities := provider.SupportedCapabilities()

	require.Len(t, capabilities, 6)
	assert.Contains(t, capabilities, types.CapabilityText)
	assert.Contains(t, capabilities, types.CapabilityChat)
	assert.Contains(t, capabilities, types.CapabilityStructured)
	assert.Contains(t, capabilities, types.CapabilityEmbeddings)
	assert.Contains(t, capabilities, types.CapabilityStream)
	assert.Contains(t, capabilities, types.CapabilityFunctions)
	assert.NotContains(t, capabilities, types.CapabilityImages)
}

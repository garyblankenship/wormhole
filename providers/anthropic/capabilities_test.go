package anthropic_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/providers/anthropic"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderSupportedCapabilities(t *testing.T) {
	t.Parallel()

	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key"})
	capabilities := provider.SupportedCapabilities()

	require.Len(t, capabilities, 5)
	assert.Contains(t, capabilities, types.CapabilityText)
	assert.Contains(t, capabilities, types.CapabilityChat)
	assert.Contains(t, capabilities, types.CapabilityStructured)
	assert.Contains(t, capabilities, types.CapabilityStream)
	assert.Contains(t, capabilities, types.CapabilityFunctions)
	assert.NotContains(t, capabilities, types.CapabilityImages)
}

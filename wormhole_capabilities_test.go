package wormhole

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

func TestCapabilitiesFromConfiguredProvider(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock")
	client := New(
		WithCustomProvider("mock", wmtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDefaultProvider("mock"),
		WithDiscovery(false),
	)

	caps := client.ProviderCapabilities("mock")
	assert.True(t, caps.SupportsText())
	assert.True(t, caps.SupportsStructured())
	assert.True(t, caps.SupportsEmbeddings())
	assert.True(t, caps.SupportsImages())
	assert.True(t, caps.SupportsAudio())
	assert.True(t, caps.SupportsStreaming())
	assert.False(t, caps.SupportsToolCalling())
	assert.NotEmpty(t, caps.All())
}

func TestConservativeProviderCapabilities(t *testing.T) {
	t.Parallel()
	client := New(WithDiscovery(false))

	openaiCaps := client.ProviderCapabilities("openai")
	assert.True(t, openaiCaps.SupportsText())
	assert.False(t, openaiCaps.SupportsImages())

	unknownCaps := client.ProviderCapabilities("unknown")
	assert.False(t, unknownCaps.SupportsText())
	assert.Empty(t, unknownCaps.All())
}

func TestModelCapabilitiesValidationAndFallback(t *testing.T) {
	t.Parallel()
	client := New(WithDiscovery(false))

	_, err := client.ModelCapabilities("", "model")
	require.Error(t, err)

	_, err = client.ModelCapabilities("openai", "")
	require.Error(t, err)

	caps, err := client.ModelCapabilities("openai", "missing-model")
	require.NoError(t, err)
	assert.True(t, caps.SupportsText())
}

func TestCapabilitiesNilReceiver(t *testing.T) {
	t.Parallel()
	var caps *Capabilities
	assert.False(t, caps.Has(CapabilityText))
	assert.Nil(t, caps.All())
	assert.False(t, caps.SupportsText())
}

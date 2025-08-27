package wormhole

import (
	"testing"
	"time"

	mockpkg "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnlimitedTimeoutConfiguration(t *testing.T) {
	t.Run("WithUnlimitedTimeout sets DefaultTimeout to 0", func(t *testing.T) {
		wormhole := New(
			WithOpenAI("test-key"),
			WithUnlimitedTimeout(),
		)

		// Verify the config has unlimited timeout
		assert.Equal(t, time.Duration(0), wormhole.config.DefaultTimeout)
	})

	t.Run("WithTimeout with 0 enables unlimited timeout", func(t *testing.T) {
		wormhole := New(
			WithOpenAI("test-key"),
			WithTimeout(0),
		)

		// Verify the config has unlimited timeout
		assert.Equal(t, time.Duration(0), wormhole.config.DefaultTimeout)
	})

	t.Run("Provider gets unlimited timeout when DefaultTimeout is 0", func(t *testing.T) {
		// Register a test provider factory that captures config
		var capturedConfig types.ProviderConfig
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			capturedConfig = config
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithUnlimitedTimeout(),
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		// Get the provider to trigger factory call
		provider, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Verify the provider config received Timeout=0 (unlimited)
		assert.Equal(t, 0, capturedConfig.Timeout)
	})

	t.Run("Normal timeout still works", func(t *testing.T) {
		var capturedConfig types.ProviderConfig
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			capturedConfig = config
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithTimeout(30*time.Second),
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		// Get the provider to trigger factory call
		provider, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Verify the provider config received Timeout=30
		assert.Equal(t, 30, capturedConfig.Timeout)
	})
}

// Note: mockProvider is already defined in provider_registration_test.go

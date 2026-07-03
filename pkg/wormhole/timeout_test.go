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
	t.Parallel()
	t.Run("WithUnlimitedTimeout sets DefaultTimeout to 0", func(t *testing.T) {
		t.Parallel()
		wormhole := New(
			WithOpenAI("test-key"),
			WithUnlimitedTimeout(),
		)

		// Verify the config has unlimited timeout
		assert.Equal(t, time.Duration(0), wormhole.config.DefaultTimeout)
	})

	t.Run("WithTimeout with 0 enables unlimited timeout", func(t *testing.T) {
		t.Parallel()
		wormhole := New(
			WithOpenAI("test-key"),
			WithTimeout(0),
		)

		// Verify the config has unlimited timeout
		assert.Equal(t, time.Duration(0), wormhole.config.DefaultTimeout)
	})

	t.Run("Provider gets unlimited timeout when DefaultTimeout is 0", func(t *testing.T) {
		t.Parallel()
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

		require.NotNil(t, capturedConfig.HTTPTimeout)
		assert.Equal(t, time.Duration(0), *capturedConfig.HTTPTimeout)
	})

	t.Run("Normal timeout still works", func(t *testing.T) {
		t.Parallel()
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

		require.NotNil(t, capturedConfig.HTTPTimeout)
		assert.Equal(t, 30*time.Second, *capturedConfig.HTTPTimeout)
		assert.Equal(t, 30, capturedConfig.Timeout)
	})

	t.Run("Subsecond timeout remains precise", func(t *testing.T) {
		t.Parallel()
		var capturedConfig types.ProviderConfig
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			capturedConfig = config
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithTimeout(500*time.Millisecond),
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		provider, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		require.NotNil(t, capturedConfig.HTTPTimeout)
		assert.Equal(t, 500*time.Millisecond, *capturedConfig.HTTPTimeout)
		assert.Equal(t, 0, capturedConfig.Timeout)
	})
}

func TestDefaultRetryConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("Wormhole defaults propagate to provider config", func(t *testing.T) {
		t.Parallel()
		var capturedConfig types.ProviderConfig
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			capturedConfig = config
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithRetries(0, 25*time.Millisecond),
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		provider, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		require.NotNil(t, capturedConfig.MaxRetries)
		assert.Equal(t, 0, *capturedConfig.MaxRetries)
		require.NotNil(t, capturedConfig.RetryDelay)
		assert.Equal(t, 25*time.Millisecond, *capturedConfig.RetryDelay)
	})

	t.Run("Provider retry config wins over Wormhole defaults", func(t *testing.T) {
		t.Parallel()
		var capturedConfig types.ProviderConfig
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			capturedConfig = config
			return mockpkg.NewMockProvider("test"), nil
		}
		providerRetries := 2
		providerDelay := 75 * time.Millisecond

		wormhole := New(
			WithRetries(0, 25*time.Millisecond),
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{
				APIKey:     "test-key",
				MaxRetries: &providerRetries,
				RetryDelay: &providerDelay,
			}),
		)

		provider, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		require.NotNil(t, capturedConfig.MaxRetries)
		assert.Equal(t, providerRetries, *capturedConfig.MaxRetries)
		require.NotNil(t, capturedConfig.RetryDelay)
		assert.Equal(t, providerDelay, *capturedConfig.RetryDelay)
	})
}

// Note: mockProvider is already defined in provider_registration_test.go

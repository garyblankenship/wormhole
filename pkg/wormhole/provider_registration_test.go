package wormhole

import (
	"testing"

	mockpkg "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistration(t *testing.T) {
	t.Run("built-in providers are registered", func(t *testing.T) {
		wormhole := New(WithOpenAI("test-key"))

		// Verify that core built-in providers are registered
		assert.Contains(t, wormhole.providerFactories, "openai")
		assert.Contains(t, wormhole.providerFactories, "anthropic")
		assert.Contains(t, wormhole.providerFactories, "gemini")
		assert.Contains(t, wormhole.providerFactories, "ollama")

		// groq and mistral are no longer built-in factories - they use WithOpenAICompatible()
		assert.NotContains(t, wormhole.providerFactories, "groq")
		assert.NotContains(t, wormhole.providerFactories, "mistral")
	})

	t.Run("custom provider registration", func(t *testing.T) {
		// Register a custom provider via functional options
		customFactory := func(config types.ProviderConfig) (types.Provider, error) {
			return mockpkg.NewMockProvider("custom"), nil
		}

		wormhole := New(
			WithCustomProvider("custom", customFactory),
			WithProviderConfig("custom", types.ProviderConfig{APIKey: "test-key"}),
		)

		// Verify the custom provider is registered
		assert.Contains(t, wormhole.providerFactories, "custom")

		// Test that we can get the custom provider
		provider, err := wormhole.Provider("custom")
		require.NoError(t, err)
		assert.Equal(t, "custom", provider.Name())
	})

	t.Run("provider factory creates instances", func(t *testing.T) {
		// Register a test provider factory with call counting
		callCount := 0
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			callCount++
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		// First call should create the provider
		provider1, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.Equal(t, "test", provider1.Name())
		assert.Equal(t, 1, callCount)

		// Second call should return cached provider (factory not called again)
		provider2, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.Equal(t, provider1, provider2) // Same instance
		assert.Equal(t, 1, callCount)         // Factory not called again
	})

	t.Run("unregistered provider returns error", func(t *testing.T) {
		wormhole := New() // Empty client with no providers

		_, err := wormhole.Provider("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown or unregistered provider")
	})

	t.Run("custom provider with auto-config works", func(t *testing.T) {
		// WithCustomProvider automatically creates a config placeholder
		wormhole := New(
			WithCustomProvider("autoconfigured", func(config types.ProviderConfig) (types.Provider, error) {
				return mockpkg.NewMockProvider("autoconfigured"), nil
			}),
			// Note: WithCustomProvider auto-creates empty config
		)

		provider, err := wormhole.Provider("autoconfigured")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "autoconfigured", provider.Name())
	})
}

func TestWithOpenAICompatibleOption(t *testing.T) {
	t.Run("WithOpenAICompatible option registers provider", func(t *testing.T) {
		// Use WithOpenAICompatible option to add a provider during initialization
		wormhole := New(
			WithOpenAICompatible("custom-openai", "https://api.example.com", types.ProviderConfig{
				APIKey: "test-key",
			}),
		)

		// Verify the provider is registered and configured
		assert.Contains(t, wormhole.providerFactories, "custom-openai")
		assert.Contains(t, wormhole.config.Providers, "custom-openai")
		assert.Equal(t, "https://api.example.com", wormhole.config.Providers["custom-openai"].BaseURL)
	})

	t.Run("WithGemini option stores config correctly", func(t *testing.T) {
		// Use WithGemini option to add provider during initialization
		wormhole := New(
			WithGemini("test-api-key", types.ProviderConfig{
				BaseURL: "custom-base-url",
			}),
		)

		// Verify config is stored with API key
		assert.Contains(t, wormhole.config.Providers, "gemini")
		config := wormhole.config.Providers["gemini"]
		assert.Equal(t, "test-api-key", config.APIKey)
		assert.Equal(t, "custom-base-url", config.BaseURL)
	})

}

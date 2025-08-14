package wormhole

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a simple mock provider for testing
type mockProvider struct {
	name string
}

func (m *mockProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	return &types.TextResponse{Text: "mock response"}, nil
}

func (m *mockProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	ch := make(chan types.TextChunk, 1)
	ch <- types.TextChunk{Text: "mock chunk"}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	return &types.StructuredResponse{Data: map[string]interface{}{"mock": "data"}}, nil
}

func (m *mockProvider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return &types.EmbeddingsResponse{}, nil
}

func (m *mockProvider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return &types.AudioResponse{}, nil
}

func (m *mockProvider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return &types.ImagesResponse{}, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestProviderRegistration(t *testing.T) {
	t.Run("built-in providers are registered", func(t *testing.T) {
		wormhole := New(WithOpenAI("test-key"))

		// Verify that built-in providers are registered
		assert.Contains(t, wormhole.providerFactories, "openai")
		assert.Contains(t, wormhole.providerFactories, "anthropic")
		assert.Contains(t, wormhole.providerFactories, "gemini")
		assert.Contains(t, wormhole.providerFactories, "groq")
		assert.Contains(t, wormhole.providerFactories, "mistral")
		assert.Contains(t, wormhole.providerFactories, "ollama")
	})

	t.Run("custom provider registration", func(t *testing.T) {
		// Register a custom provider via functional options
		customFactory := func(config types.ProviderConfig) (types.Provider, error) {
			return &mockProvider{name: "custom"}, nil
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
			return &mockProvider{name: "test"}, nil
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
				return &mockProvider{name: "autoconfigured"}, nil
			}),
			// Note: WithCustomProvider auto-creates empty config
		)

		provider, err := wormhole.Provider("autoconfigured")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "autoconfigured", provider.(*mockProvider).name)
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

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
		wormhole := New(Config{
			Providers: map[string]types.ProviderConfig{
				"openai": {APIKey: "test-key"},
			},
		})

		// Verify that built-in providers are registered
		assert.Contains(t, wormhole.providerFactories, "openai")
		assert.Contains(t, wormhole.providerFactories, "anthropic")
		assert.Contains(t, wormhole.providerFactories, "gemini")
		assert.Contains(t, wormhole.providerFactories, "groq")
		assert.Contains(t, wormhole.providerFactories, "mistral")
		assert.Contains(t, wormhole.providerFactories, "ollama")
	})

	t.Run("custom provider registration", func(t *testing.T) {
		wormhole := New(Config{
			Providers: map[string]types.ProviderConfig{
				"custom": {APIKey: "test-key"},
			},
		})

		// Register a custom provider
		customFactory := func(config types.ProviderConfig) (types.Provider, error) {
			return &mockProvider{name: "custom"}, nil
		}
		wormhole.RegisterProvider("custom", customFactory)

		// Verify the custom provider is registered
		assert.Contains(t, wormhole.providerFactories, "custom")

		// Test that we can get the custom provider
		provider, err := wormhole.Provider("custom")
		require.NoError(t, err)
		assert.Equal(t, "custom", provider.Name())
	})

	t.Run("provider factory creates instances", func(t *testing.T) {
		wormhole := New(Config{
			Providers: map[string]types.ProviderConfig{
				"test": {APIKey: "test-key"},
			},
		})

		// Register a test provider factory
		callCount := 0
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			callCount++
			return &mockProvider{name: "test"}, nil
		}
		wormhole.RegisterProvider("test", testFactory)

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
		wormhole := New(Config{})

		_, err := wormhole.Provider("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown or unregistered provider")
	})

	t.Run("unconfigured provider returns error", func(t *testing.T) {
		wormhole := New(Config{})
		
		// Register provider but don't configure it
		wormhole.RegisterProvider("unconfigured", func(config types.ProviderConfig) (types.Provider, error) {
			return &mockProvider{name: "unconfigured"}, nil
		})

		_, err := wormhole.Provider("unconfigured")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})
}

func TestWithMethodsUseRegistration(t *testing.T) {
	t.Run("WithOpenAICompatible registers provider", func(t *testing.T) {
		wormhole := New(Config{})

		// Use WithOpenAICompatible to add a provider
		wormhole.WithOpenAICompatible("custom-openai", "https://api.example.com", types.ProviderConfig{
			APIKey: "test-key",
		})

		// Verify the provider is registered and configured
		assert.Contains(t, wormhole.providerFactories, "custom-openai")
		assert.Contains(t, wormhole.config.Providers, "custom-openai")
		assert.Equal(t, "https://api.example.com", wormhole.config.Providers["custom-openai"].BaseURL)
	})

	t.Run("WithGemini stores config correctly", func(t *testing.T) {
		wormhole := New(Config{})

		// Use WithGemini to add provider
		wormhole.WithGemini("test-api-key", types.ProviderConfig{
			BaseURL: "custom-base-url",
		})

		// Verify config is stored with API key
		assert.Contains(t, wormhole.config.Providers, "gemini")
		config := wormhole.config.Providers["gemini"]
		assert.Equal(t, "test-api-key", config.APIKey)
		assert.Equal(t, "custom-base-url", config.BaseURL)
	})
}
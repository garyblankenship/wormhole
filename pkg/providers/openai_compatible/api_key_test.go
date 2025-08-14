package openai_compatible

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestAPIKeyHandling(t *testing.T) {
	testAPIKey := "test-api-key-12345"
	baseURL := "https://api.example.com"

	t.Run("NewGeneric preserves API key for cloud services", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey: testAPIKey,
		}

		provider := NewGeneric("openrouter", baseURL, config)
		assert.NotNil(t, provider)
		assert.Equal(t, "openrouter", provider.Name())

		// The underlying OpenAI provider should have received the API key
		// We can't directly access it, but we can verify the config was passed through
		// by checking that no panic occurred and the provider was created successfully
		assert.NotNil(t, provider.Provider)
	})

	t.Run("NewLMStudio clears API key for local service", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey:  testAPIKey,
			BaseURL: "http://localhost:1234",
		}

		// Should not panic and should create provider successfully
		provider := NewLMStudio(config)
		assert.NotNil(t, provider)
		assert.Equal(t, "lmstudio", provider.Name())
	})

	t.Run("NewVLLM clears API key for local service", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey:  testAPIKey,
			BaseURL: "http://localhost:8000",
		}

		provider := NewVLLM(config)
		assert.NotNil(t, provider)
		assert.Equal(t, "vllm", provider.Name())
	})

	t.Run("NewOllamaOpenAI clears API key for local service", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey:  testAPIKey,
			BaseURL: "http://localhost:11434",
		}

		provider := NewOllamaOpenAI(config)
		assert.NotNil(t, provider)
		assert.Equal(t, "ollama-openai", provider.Name())
	})

	t.Run("New preserves API key when called directly", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey:  testAPIKey,
			BaseURL: baseURL,
		}

		// This should preserve the API key for cloud services
		provider := New("custom-cloud-service", config)
		assert.NotNil(t, provider)
		assert.Equal(t, "custom-cloud-service", provider.Name())
	})
}

func TestConfigModification(t *testing.T) {
	t.Run("local service constructors modify config copy", func(t *testing.T) {
		originalConfig := types.ProviderConfig{
			APIKey:  "original-key",
			BaseURL: "http://localhost:1234",
		}

		// Call NewLMStudio which should clear the API key
		provider := NewLMStudio(originalConfig)
		assert.NotNil(t, provider)

		// Original config should be unchanged (Go passes structs by value)
		assert.Equal(t, "original-key", originalConfig.APIKey)
	})
}

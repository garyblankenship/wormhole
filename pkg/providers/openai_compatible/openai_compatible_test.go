package openai_compatible

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderImplementsInterface(t *testing.T) {
	config := types.ProviderConfig{
		BaseURL: "http://localhost:1234/v1",
	}

	provider := New("test", config)

	// Verify that our provider implements the Provider interface
	var _ types.Provider = provider

	// Verify specific interface implementations
	var _ types.TextProvider = provider
	var _ types.StructuredProvider = provider
	var _ types.EmbeddingsProvider = provider
	var _ types.AudioProvider = provider
	var _ types.ImageProvider = provider
}

func TestLMStudioProvider(t *testing.T) {
	config := types.ProviderConfig{
		BaseURL: "http://localhost:1234/v1",
	}
	provider := NewLMStudio(config)

	if provider.Name() != "lmstudio" {
		t.Errorf("Expected provider name to be 'lmstudio', got '%s'", provider.Name())
	}

	if provider.GetBaseURL() != "http://localhost:1234/v1" {
		t.Errorf("Expected base URL to be 'http://localhost:1234/v1', got '%s'", provider.GetBaseURL())
	}
}

func TestVLLMProvider(t *testing.T) {
	config := types.ProviderConfig{
		BaseURL: "http://localhost:8000/v1",
	}
	provider := NewVLLM(config)

	if provider.Name() != "vllm" {
		t.Errorf("Expected provider name to be 'vllm', got '%s'", provider.Name())
	}

	if provider.GetBaseURL() != "http://localhost:8000/v1" {
		t.Errorf("Expected base URL to be 'http://localhost:8000/v1', got '%s'", provider.GetBaseURL())
	}
}

func TestOllamaOpenAIProvider(t *testing.T) {
	config := types.ProviderConfig{
		BaseURL: "http://localhost:11434/v1",
	}
	provider := NewOllamaOpenAI(config)

	if provider.Name() != "ollama-openai" {
		t.Errorf("Expected provider name to be 'ollama-openai', got '%s'", provider.Name())
	}

	if provider.GetBaseURL() != "http://localhost:11434/v1" {
		t.Errorf("Expected base URL to be 'http://localhost:11434/v1', got '%s'", provider.GetBaseURL())
	}
}

func TestGenericProvider(t *testing.T) {
	customURL := "https://custom.example.com/v1"
	config := types.ProviderConfig{}
	provider := NewGeneric("custom", customURL, config)

	if provider.Name() != "custom" {
		t.Errorf("Expected provider name to be 'custom', got '%s'", provider.Name())
	}

	if provider.GetBaseURL() != customURL {
		t.Errorf("Expected base URL to be '%s', got '%s'", customURL, provider.GetBaseURL())
	}
}

func TestCustomBaseURL(t *testing.T) {
	customURL := "https://my-lmstudio.com/v1"
	config := types.ProviderConfig{
		BaseURL: customURL,
	}
	provider := NewLMStudio(config)

	if provider.GetBaseURL() != customURL {
		t.Errorf("Expected base URL to be '%s', got '%s'", customURL, provider.GetBaseURL())
	}
}

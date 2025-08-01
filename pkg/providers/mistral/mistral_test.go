package mistral

import (
	"testing"

	"github.com/prism-php/prism-go/pkg/types"
)

func TestProviderImplementsInterface(t *testing.T) {
	config := types.ProviderConfig{
		APIKey: "test-key",
	}

	provider := New(config)

	// Verify that our provider implements the Provider interface
	var _ types.Provider = provider

	// Verify specific interface implementations
	var _ types.TextProvider = provider
	var _ types.StructuredProvider = provider
	var _ types.EmbeddingsProvider = provider
	var _ types.AudioProvider = provider
	var _ types.ImageProvider = provider
}

func TestProviderName(t *testing.T) {
	config := types.ProviderConfig{
		APIKey: "test-key",
	}

	provider := New(config)

	if provider.Name() != "mistral" {
		t.Errorf("Expected provider name to be 'mistral', got '%s'", provider.Name())
	}
}

func TestProviderBaseURL(t *testing.T) {
	// Test default base URL
	config := types.ProviderConfig{
		APIKey: "test-key",
	}

	provider := New(config)

	if provider.GetBaseURL() != defaultBaseURL {
		t.Errorf("Expected base URL to be '%s', got '%s'", defaultBaseURL, provider.GetBaseURL())
	}

	// Test custom base URL
	customURL := "https://custom.mistral.ai/v1"
	config.BaseURL = customURL

	provider = New(config)

	if provider.GetBaseURL() != customURL {
		t.Errorf("Expected base URL to be '%s', got '%s'", customURL, provider.GetBaseURL())
	}
}

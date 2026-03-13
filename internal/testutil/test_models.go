package testutil

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// SetupTestModels registers test models in the global model registry for testing
func SetupTestModels(t *testing.T) {
	// Save original registry for cleanup
	originalRegistry := types.DefaultModelRegistry

	// Reset to empty registry for testing
	types.DefaultModelRegistry = types.NewModelRegistry()

	// Cleanup after test
	t.Cleanup(func() {
		types.DefaultModelRegistry = originalRegistry
	})

	// Register test models
	testModels := []*types.ModelInfo{
		{
			ID:          "gpt-5",
			Name:        "GPT-5",
			Provider:    "openai",
			Description: "Test GPT-5 model",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
			ContextLength: 128000,
			MaxTokens:     4096,
		},
		{
			ID:          "claude-3-opus",
			Name:        "Claude 3 Opus",
			Provider:    "anthropic",
			Description: "Test Claude 3 Opus model",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
			ContextLength: 200000,
			MaxTokens:     4096,
		},
	}

	for _, model := range testModels {
		types.DefaultModelRegistry.Register(model)
	}
}

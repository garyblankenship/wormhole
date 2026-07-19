package discovery

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// getFallbackModels returns minimal hardcoded models for offline mode
func getFallbackModels() map[string][]*types.ModelInfo {
	return map[string][]*types.ModelInfo{
		"openai": {
			{
				ID:       "gpt-5",
				Name:     "GPT-5",
				Provider: "openai",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
				},
				MaxTokens: 128000,
			},
			{
				ID:       "gpt-5-mini",
				Name:     "GPT-5 Mini",
				Provider: "openai",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
				},
				MaxTokens: 128000,
			},
		},
		"anthropic": {
			{
				ID:       "claude-sonnet-4-5",
				Name:     "Claude Sonnet 4.5",
				Provider: "anthropic",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
					types.CapabilityVision,
				},
				MaxTokens: 200000,
			},
		},
		"openrouter": {
			// OpenRouter is fully dynamic, no fallback needed
		},
		"ollama": {
			// Ollama models are user-specific, no fallback possible
		},
	}
}

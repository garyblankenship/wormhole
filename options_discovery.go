package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/discovery"
	"github.com/garyblankenship/wormhole/v2/types"
)

// WithDiscoveryConfig configures the dynamic model discovery service.
// This allows customization of caching behavior, refresh intervals, and offline mode.
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
//	        CacheTTL:        12 * time.Hour,  // Cache models for 12 hours
//	        RefreshInterval: 6 * time.Hour,   // Refresh every 6 hours
//	        OfflineMode:     false,           // Allow network fetching
//	    }),
//	)
func WithDiscoveryConfig(config discovery.DiscoveryConfig) Option {
	return func(c *Config) {
		c.DiscoveryConfig = discovery.MergeConfig(c.DiscoveryConfig, config)
	}
}

// WithOfflineMode enables offline mode for model discovery.
// When enabled, only cached and fallback models will be available (no network fetching).
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithOfflineMode(true),
//	)
func WithOfflineMode(enabled bool) Option {
	return func(c *Config) {
		c.DiscoveryConfig.OfflineMode = enabled
	}
}

// WithDiscovery enables or disables the dynamic model discovery system.
// When disabled, only hardcoded fallback models will be available.
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithDiscovery(false), // Disable discovery, use only fallback models
//	)
func WithDiscovery(enabled bool) Option {
	return func(c *Config) {
		c.EnableDiscovery = enabled
	}
}

// WithProviderFromEnv configures a provider using environment variables from
// the built-in provider profile registry.
//
// Supported provider names:
//   - "openai" -> OPENAI_API_KEY, OPENAI_BASE_URL
//   - "anthropic" -> ANTHROPIC_API_KEY, ANTHROPIC_BASE_URL
//   - "gemini" -> GEMINI_API_KEY, GEMINI_BASE_URL
//   - "groq" -> GROQ_API_KEY
//   - "openrouter" -> OPENROUTER_API_KEY
//
// Example:
//
//	// Set environment variables:
//	// export OPENAI_API_KEY=sk-...
//	// export ANTHROPIC_API_KEY=sk-ant-...
//
//	client := wormhole.New(
//	    wormhole.WithProviderFromEnv("openai"),
//	    wormhole.WithProviderFromEnv("anthropic"),
//	    wormhole.WithDefaultProvider("openai"),
//	)
//
// Returns a no-op option if the API key environment variable is not set.
// This allows safe composition without runtime errors for unconfigured providers.
func WithProviderFromEnv(provider string) Option {
	return func(c *Config) {
		profile, known := providerProfile(provider)
		if !known {
			return
		}

		apiKey := configuredAPIKey(profile)
		if apiKey == "" && !profile.Local {
			return
		}

		cfg := types.ProviderConfig{
			APIKey:  apiKey,
			BaseURL: configuredBaseURL(profile),
		}

		switch provider {
		case "openai":
			WithOpenAI(apiKey, cfg)(c)
		case "anthropic":
			WithAnthropic(apiKey, cfg)(c)
		case "gemini":
			WithGemini(apiKey, cfg)(c)
		case "groq":
			WithGroq(apiKey, cfg)(c)
		case "mistral":
			WithMistral(cfg)(c)
		case "ollama":
			WithOllama(cfg)(c)
		case "openrouter":
			WithProfiledOpenAICompatible("openrouter", cfg)(c)
		default:
			if profile.Kind == providerKindOpenAICompatible && cfg.BaseURL != "" {
				WithProfiledOpenAICompatible(provider, cfg)(c)
			}
		}
	}
}

// WithAllProvidersFromEnv configures all known providers from environment variables.
// This is a convenience function for applications that want to auto-configure
// all available providers based on which API keys are present in the environment.
//
// Example:
//
//	// All providers with API keys in env will be configured
//	client := wormhole.New(
//	    wormhole.WithAllProvidersFromEnv(),
//	    wormhole.WithDefaultProvider("openai"),
//	)
func WithAllProvidersFromEnv() Option {
	return func(c *Config) {
		for _, profile := range envProviderProfiles() {
			WithProviderFromEnv(profile.Name)(c)
		}
	}
}

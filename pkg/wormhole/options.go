package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Option is a function that configures a Wormhole client.
type Option func(*Config)

// WithDefaultProvider sets the default provider for requests.
func WithDefaultProvider(name string) Option {
	return func(c *Config) {
		c.DefaultProvider = name
	}
}

// WithOpenAI configures the OpenAI provider.
func WithOpenAI(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}

		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.APIKey = apiKey
		c.Providers["openai"] = cfg

		// Models are now auto-registered globally in New() - no need to register here
	}
}

// WithAnthropic configures the Anthropic provider.
func WithAnthropic(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}

		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.APIKey = apiKey
		c.Providers["anthropic"] = cfg
	}
}

// WithGemini configures the Gemini provider.
func WithGemini(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}

		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.APIKey = apiKey
		c.Providers["gemini"] = cfg
	}
}

// WithGroq configures the Groq provider as an OpenAI-compatible endpoint.
func WithGroq(apiKey string, config ...types.ProviderConfig) Option {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	cfg.APIKey = apiKey

	// Use the generic OpenAI-compatible provider factory
	return WithOpenAICompatible("groq", "https://api.groq.com/openai/v1", cfg)
}

// WithMistral configures the Mistral provider as an OpenAI-compatible endpoint.
func WithMistral(config types.ProviderConfig) Option {
	// Use the generic OpenAI-compatible provider factory
	return WithOpenAICompatible("mistral", "https://api.mistral.ai/v1", config)
}

// WithOllama configures the Ollama provider.
func WithOllama(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["ollama"] = config
	}
}

// WithLMStudio configures the LMStudio provider.
func WithLMStudio(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		c.Providers["lmstudio"] = config
		c.CustomFactories["lmstudio"] = func(cfg types.ProviderConfig) (types.Provider, error) {
			return openai.New(cfg), nil
		}
	}
}

// WithVLLM configures the vLLM provider.
func WithVLLM(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		c.Providers["vllm"] = config
		c.CustomFactories["vllm"] = func(cfg types.ProviderConfig) (types.Provider, error) {
			return openai.New(cfg), nil
		}
	}
}

// WithOllamaOpenAI configures the Ollama OpenAI-compatible provider.
func WithOllamaOpenAI(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		c.Providers["ollama-openai"] = config
		c.CustomFactories["ollama-openai"] = func(cfg types.ProviderConfig) (types.Provider, error) {
			return openai.New(cfg), nil
		}
	}
}

// WithOpenAICompatible configures a generic OpenAI-compatible provider.
func WithOpenAICompatible(name, baseURL string, config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		// Set the baseURL in config
		config.BaseURL = baseURL
		c.Providers[name] = config

		// Register the factory for this custom provider
		c.CustomFactories[name] = func(cfg types.ProviderConfig) (types.Provider, error) {
			return openai.New(cfg), nil
		}

		// Models are now auto-registered globally in New() - no need to register here
		// OpenRouter models are automatically available
	}
}

// WithCustomProvider registers a custom provider with its factory function.
func WithCustomProvider(name string, factory types.ProviderFactory) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		// Ensure a config placeholder exists
		if _, ok := c.Providers[name]; !ok {
			c.Providers[name] = types.ProviderConfig{}
		}

		c.CustomFactories[name] = factory
	}
}

// WithProviderConfig sets the configuration for a specific provider.
func WithProviderConfig(name string, config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers[name] = config
	}
}

// WithMiddleware adds middleware to the client's execution chain.
func WithMiddleware(mw ...middleware.Middleware) Option {
	return func(c *Config) {
		c.Middleware = append(c.Middleware, mw...)
	}
}

// WithTimeout sets the default timeout for requests.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.DefaultTimeout = timeout
	}
}

// WithUnlimitedTimeout disables HTTP client timeouts for long-running AI processing.
// Use for heavy text processing that may take 3+ minutes.
func WithUnlimitedTimeout() Option {
	return func(c *Config) {
		c.DefaultTimeout = 0 // 0 = unlimited timeout
	}
}

// WithDebugLogging enables debug logging with an optional custom logger.
func WithDebugLogging(logger ...types.Logger) Option {
	return func(c *Config) {
		c.DebugLogging = true
		if len(logger) > 0 && logger[0] != nil {
			c.Logger = logger[0]
		}
	}
}

// WithLogger sets a custom logger for the client.
func WithLogger(logger types.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithModelValidation enables or disables model validation against the registry.
func WithModelValidation(enabled bool) Option {
	return func(c *Config) {
		c.ModelValidation = enabled
	}
}

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
		c.DiscoveryConfig = config
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

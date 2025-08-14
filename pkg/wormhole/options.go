package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/providers/openai_compatible"
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
func WithOpenAI(apiKey string) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["openai"] = types.ProviderConfig{APIKey: apiKey}
	}
}

// WithAnthropic configures the Anthropic provider.
func WithAnthropic(apiKey string) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["anthropic"] = types.ProviderConfig{APIKey: apiKey}
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

// WithGroq configures the Groq provider.
func WithGroq(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}

		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.APIKey = apiKey
		c.Providers["groq"] = cfg
	}
}

// WithMistral configures the Mistral provider.
func WithMistral(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["mistral"] = config
	}
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
			return openai_compatible.NewLMStudio(cfg), nil
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
			return openai_compatible.NewVLLM(cfg), nil
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
			return openai_compatible.NewOllamaOpenAI(cfg), nil
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
			return openai_compatible.NewGeneric(name, cfg.BaseURL, cfg), nil
		}

		// Auto-register common models for OpenRouter
		if name == "openrouter" {
			registerOpenRouterModels()
			
			// OpenRouter can be much slower due to:
			// 1. Routing overhead through their infrastructure  
			// 2. Heavy models like Claude Opus 4.1 can take 60-120s for complex responses
			// 3. Model cold starts and queue times during peak usage
			if c.DefaultTimeout == 0 {
				c.DefaultTimeout = 2 * time.Minute // Realistic for heavy models like Opus 4.1
			}
		}
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

// WithRetries configures the default retry behavior.
func WithRetries(maxRetries int, baseDelay time.Duration) Option {
	return func(c *Config) {
		c.DefaultRetries = maxRetries
		c.DefaultRetryDelay = baseDelay
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

// registerOpenRouterModels auto-registers popular OpenRouter models to the DefaultModelRegistry
func registerOpenRouterModels() {
	// Top 10 OpenRouter models (weekly rankings)
	openRouterModels := []*types.ModelInfo{
		// TOP 10 WEEKLY MODELS

		// #1 - GPT-5 Series (Latest OpenAI)
		{
			ID:       "openai/gpt-5",
			Name:     "GPT-5",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
				types.CapabilityVision,
			},
		},
		{
			ID:       "openai/gpt-5-mini",
			Name:     "GPT-5 Mini",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},
		{
			ID:       "openai/gpt-5-nano",
			Name:     "GPT-5 Nano",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityStream,
			},
		},

		// #2 - Claude Opus 4 (Top coding model)
		{
			ID:       "anthropic/claude-opus-4",
			Name:     "Claude Opus 4",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// #3 - Claude Sonnet 4 (Balanced performance)
		{
			ID:       "anthropic/claude-sonnet-4",
			Name:     "Claude Sonnet 4",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// #4 - Gemini 2.5 Pro (Google's advanced reasoning)
		{
			ID:       "google/gemini-2.5-pro",
			Name:     "Gemini 2.5 Pro",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
				types.CapabilityVision,
			},
		},

		// #5 - Gemini 2.5 Flash (Workhorse model)
		{
			ID:       "google/gemini-2.5-flash",
			Name:     "Gemini 2.5 Flash",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// #6 - Mistral Medium 3.1 (Enterprise-grade)
		{
			ID:       "mistralai/mistral-medium-3.1",
			Name:     "Mistral Medium 3.1",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// #7 - Codestral 2508 (Specialized coding)
		{
			ID:       "mistralai/codestral-2508",
			Name:     "Codestral 2508",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityStream,
			},
		},

		// #8 - GPT-4o (Still popular for vision)
		{
			ID:       "openai/gpt-4o",
			Name:     "GPT-4o",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
				types.CapabilityVision,
			},
		},

		// EXISTING MODELS (CONFIRMED WORKING)

		// Claude 4.1 - Already confirmed by user
		{
			ID:       "anthropic/claude-opus-4.1",
			Name:     "Claude Opus 4.1",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// Claude 3.5 Series - Popular fallbacks
		{
			ID:       "anthropic/claude-3-5-sonnet",
			Name:     "Claude 3.5 Sonnet",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},
		{
			ID:       "anthropic/claude-3-5-haiku",
			Name:     "Claude 3.5 Haiku",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// Meta Llama - Popular open source
		{
			ID:       "meta-llama/llama-3.3-70b-instruct",
			Name:     "Llama 3.3 70B Instruct",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityStream,
			},
		},
	}

	// Register all models with the DefaultModelRegistry
	for _, model := range openRouterModels {
		types.DefaultModelRegistry.Register(model)
	}
}

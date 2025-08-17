package wormhole

import (
	"time"

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
func WithOpenAI(apiKey string) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["openai"] = types.ProviderConfig{APIKey: apiKey}

		// Models are now auto-registered globally in New() - no need to register here
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

// registerOpenAIModels registers OpenAI models for validation and discovery
func registerOpenAIModels() {
	openAIModels := []*types.ModelInfo{
		// GPT-5 Series
		{ID: "gpt-5", Name: "GPT-5", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0050, OutputTokens: 0.0150, Currency: "USD"}},
		{ID: "gpt-5-mini", Name: "GPT-5 Mini", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0015, OutputTokens: 0.0060, Currency: "USD"}},
		{ID: "gpt-5-turbo", Name: "GPT-5 Turbo", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0100, OutputTokens: 0.0300, Currency: "USD"}},

		// GPT-4 Series
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0025, OutputTokens: 0.0100, Currency: "USD"}},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0001, OutputTokens: 0.0006, Currency: "USD"}},
		{ID: "gpt-4.1", Name: "GPT-4.1", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityFunctions, types.CapabilityStructured}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0025, OutputTokens: 0.0100, Currency: "USD"}},
		{ID: "gpt-4.1-mini", Name: "GPT-4.1 Mini", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityFunctions, types.CapabilityStructured}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0001, OutputTokens: 0.0006, Currency: "USD"}},
		{ID: "gpt-4", Name: "GPT-4", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 8192, Cost: &types.ModelCost{InputTokens: 0.0300, OutputTokens: 0.0600, Currency: "USD"}},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 128000, Cost: &types.ModelCost{InputTokens: 0.0100, OutputTokens: 0.0300, Currency: "USD"}},

		// GPT-3.5 Series
		{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStructured, types.CapabilityFunctions}, MaxTokens: 16384, Cost: &types.ModelCost{InputTokens: 0.0005, OutputTokens: 0.0015, Currency: "USD"}},

		// Embeddings
		{ID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityEmbeddings}, MaxTokens: 8191, Cost: &types.ModelCost{InputTokens: 0.0001, OutputTokens: 0.0000, Currency: "USD"}},
		{ID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityEmbeddings}, MaxTokens: 8191, Cost: &types.ModelCost{InputTokens: 0.00002, OutputTokens: 0.0000, Currency: "USD"}},
		{ID: "text-embedding-ada-002", Name: "Text Embedding Ada 002", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityEmbeddings}, MaxTokens: 8191, Cost: &types.ModelCost{InputTokens: 0.0001, OutputTokens: 0.0000, Currency: "USD"}},

		// Audio Models
		{ID: "whisper-1", Name: "Whisper 1", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityAudio}, MaxTokens: 0, Cost: &types.ModelCost{InputTokens: 0.0060, OutputTokens: 0.0000, Currency: "USD"}},
		{ID: "tts-1", Name: "TTS 1", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityAudio}, MaxTokens: 4096, Cost: &types.ModelCost{InputTokens: 0.0150, OutputTokens: 0.0000, Currency: "USD"}},
		{ID: "tts-1-hd", Name: "TTS 1 HD", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityAudio}, MaxTokens: 4096, Cost: &types.ModelCost{InputTokens: 0.0300, OutputTokens: 0.0000, Currency: "USD"}},

		// Image Models
		{ID: "dall-e-3", Name: "DALL-E 3", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityImages}, MaxTokens: 0, Cost: &types.ModelCost{InputTokens: 0.0400, OutputTokens: 0.0800, Currency: "USD"}},
		{ID: "dall-e-2", Name: "DALL-E 2", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityImages}, MaxTokens: 0, Cost: &types.ModelCost{InputTokens: 0.0200, OutputTokens: 0.0000, Currency: "USD"}},
	}

	for _, model := range openAIModels {
		types.DefaultModelRegistry.Register(model)
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

		// REQUESTED MODELS FOR TESTING

		// GPT-OSS-120B (User requested test model)
		{
			ID:       "openai/gpt-oss-120b",
			Name:     "GPT-OSS-120B",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// GPT-4.1 Series
		{
			ID:       "openai/gpt-4.1",
			Name:     "GPT-4.1",
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
			ID:       "openai/gpt-4.1-mini",
			Name:     "GPT-4.1 Mini",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},

		// O Series Models
		{
			ID:       "openai/o3",
			Name:     "O3",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
		},
		{
			ID:       "openai/o1-mini",
			Name:     "O1 Mini",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityStream,
			},
		},

		// GPT-3.5 for OpenRouter
		{
			ID:       "openai/gpt-3.5-turbo",
			Name:     "GPT-3.5 Turbo",
			Provider: "openrouter",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
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

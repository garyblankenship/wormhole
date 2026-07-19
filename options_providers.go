package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// Option is a function that configures a Wormhole client.
type Option func(*Config)

func registerProvider(c *Config, name, apiKey string, cfgs ...types.ProviderConfig) {
	if c.Providers == nil {
		c.Providers = make(map[string]types.ProviderConfig)
	}
	var cfg types.ProviderConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	applyProviderProfileConfig(name, &cfg)
	cfg.APIKey = apiKey
	c.Providers[name] = cfg
}

func registerOpenAICompatible(c *Config, name string, cfg types.ProviderConfig) {
	if c.Providers == nil {
		c.Providers = make(map[string]types.ProviderConfig)
	}
	if c.CustomFactories == nil {
		c.CustomFactories = make(map[string]types.ProviderFactory)
	}
	applyProviderProfileConfig(name, &cfg)
	c.Providers[name] = cfg
	c.CustomFactories[name] = namedOpenAICompatibleFactory(name)
}

func disableProviderRetries(config *types.ProviderConfig) {
	maxRetries := 0
	config.MaxRetries = &maxRetries
}

// WithDefaultProvider sets the default provider for requests.
func WithDefaultProvider(name string) Option {
	return func(c *Config) {
		c.DefaultProvider = name
	}
}

// WithOpenAI configures the OpenAI provider.
func WithOpenAI(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		registerProvider(c, "openai", apiKey, config...)
	}
}

// WithOpenAIResponses configures the OpenAI provider to use the Responses API
// for text generation instead of Chat Completions.
func WithOpenAIResponses(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.UseResponsesAPI = true
		registerProvider(c, "openai", apiKey, cfg)
	}
}

// WithAnthropic configures the Anthropic provider.
func WithAnthropic(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		registerProvider(c, "anthropic", apiKey, config...)
	}
}

// WithGemini configures the Gemini provider.
func WithGemini(apiKey string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		registerProvider(c, "gemini", apiKey, config...)
	}
}

// WithGroq configures the Groq provider as an OpenAI-compatible endpoint.
func WithGroq(apiKey string, config ...types.ProviderConfig) Option {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	cfg.APIKey = apiKey

	return WithProfiledOpenAICompatible("groq", cfg)
}

// WithMistral configures the Mistral provider as an OpenAI-compatible endpoint.
func WithMistral(config types.ProviderConfig) Option {
	return WithProfiledOpenAICompatible("mistral", config)
}

// WithOllama configures the Ollama provider.
func WithOllama(config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers["ollama"] = config // no APIKey override; caller sets it in config
	}
}

// WithLMStudio configures the LMStudio provider.
func WithLMStudio(config types.ProviderConfig) Option {
	return WithProfiledOpenAICompatible("lmstudio", config)
}

// WithVLLM configures the vLLM provider.
func WithVLLM(config types.ProviderConfig) Option {
	return WithProfiledOpenAICompatible("vllm", config)
}

// WithOllamaOpenAI configures the Ollama OpenAI-compatible provider.
func WithOllamaOpenAI(config types.ProviderConfig) Option {
	return WithProfiledOpenAICompatible("ollama-openai", config)
}

// WithOpenAICompatible configures a generic OpenAI-compatible provider.
func WithOpenAICompatible(name, baseURL string, config types.ProviderConfig) Option {
	return func(c *Config) {
		config.BaseURL = baseURL
		registerOpenAICompatible(c, name, config)
	}
}

// WithLocalOpenAI configures a no-auth local OpenAI-compatible endpoint under
// provider name "local". Pass the OpenAI-compatible API root, usually
// "http://host:port/v1"; Wormhole appends "/chat/completions".
func WithLocalOpenAI(baseURL string, config ...types.ProviderConfig) Option {
	return func(c *Config) {
		var cfg types.ProviderConfig
		if len(config) > 0 {
			cfg = config[0]
		}
		cfg.BaseURL = baseURL
		cfg.NoAuth = true
		cfg.DynamicModels = true
		if cfg.MaxRetries == nil {
			disableProviderRetries(&cfg)
		}
		registerOpenAICompatible(c, "local", cfg)
		if c.DefaultProvider == "" {
			c.DefaultProvider = "local"
		}
	}
}

// WithProfiledOpenAICompatible configures a known OpenAI-compatible provider
// using the provider profile's default or environment-provided base URL.
func WithProfiledOpenAICompatible(name string, config types.ProviderConfig) Option {
	return func(c *Config) {
		if profile, ok := providerProfile(name); ok {
			if config.BaseURL == "" {
				config.BaseURL = configuredBaseURL(profile)
			}
			applyProviderProfile(profile, &config)
		}
		registerOpenAICompatible(c, name, config)
	}
}

func applyProviderProfileConfig(name string, config *types.ProviderConfig) {
	if profile, ok := providerProfile(name); ok {
		applyProviderProfile(profile, config)
	}
}

func applyProviderProfile(profile ProviderProfile, config *types.ProviderConfig) {
	if config.RequestPolicy.MaxTokensParam == "" {
		config.RequestPolicy.MaxTokensParam = profile.RequestPolicy.MaxTokensParam
	}
	if len(config.RequestPolicy.MaxTokensParamRules) == 0 && len(profile.RequestPolicy.MaxTokensParamRules) > 0 {
		config.RequestPolicy.MaxTokensParamRules = make([]types.MaxTokensParamRule, 0, len(profile.RequestPolicy.MaxTokensParamRules))
		for _, rule := range profile.RequestPolicy.MaxTokensParamRules {
			config.RequestPolicy.MaxTokensParamRules = append(config.RequestPolicy.MaxTokensParamRules, types.MaxTokensParamRule{
				ModelContains: rule.ModelContains,
				Param:         rule.Param,
			})
		}
	}
	if config.RequestPolicy.MaxTokensCap == 0 {
		config.RequestPolicy.MaxTokensCap = profile.RequestPolicy.MaxTokensCap
	}
	if config.ImagePath == "" {
		config.ImagePath = profile.ImagePath
	}
	if config.ResponsesPath == "" {
		config.ResponsesPath = profile.ResponsesPath
	}
	// A profile may opt a provider into the Responses transport; a caller who already
	// enabled it stays enabled. No bundled profile sets this on (OpenRouter /responses is beta).
	if !config.UseResponsesAPI {
		config.UseResponsesAPI = profile.UseResponsesAPI
	}
	if len(profile.DefaultProviderOptions) > 0 {
		config.DefaultProviderOptions = mergeProfileProviderOptions(profile.DefaultProviderOptions, config.DefaultProviderOptions)
	}
}

func mergeProfileProviderOptions(profileDefaults, configDefaults map[string]any) map[string]any {
	merged := make(map[string]any, len(profileDefaults)+len(configDefaults))
	for k, v := range profileDefaults {
		merged[k] = types.CloneValue(v)
	}
	for k, v := range configDefaults {
		merged[k] = types.CloneValue(v)
	}
	return merged
}

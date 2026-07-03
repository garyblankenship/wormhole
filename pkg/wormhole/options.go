package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
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
		merged[k] = v
	}
	for k, v := range configDefaults {
		merged[k] = v
	}
	return merged
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
// DEPRECATED: Use WithProviderMiddleware for type-safe middleware instead.
// This function automatically converts legacy middleware to type-safe middleware
// using adapter pattern for backward compatibility.
func WithMiddleware(mw ...middleware.Middleware) Option {
	return func(c *Config) {
		// Store legacy middleware for backward compatibility
		c.Middleware = append(c.Middleware, mw...)

		// Convert legacy middleware to type-safe middleware using adapter
		for _, legacyMw := range mw {
			adapter := middleware.NewLegacyAdapter(legacyMw)
			c.ProviderMiddlewares = append(c.ProviderMiddlewares, adapter)
		}
	}
}

// WithProviderMiddleware adds type-safe middleware to the client's execution chain.
// Use this for compile-time type checking instead of the deprecated WithMiddleware.
func WithProviderMiddleware(mw ...types.ProviderMiddleware) Option {
	return func(c *Config) {
		c.ProviderMiddlewares = append(c.ProviderMiddlewares, mw...)
	}
}

// WithTimeout sets the default timeout for requests.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.DefaultTimeout = timeout
		c.DefaultTimeoutSet = true
	}
}

// WithUnlimitedTimeout disables HTTP client timeouts for long-running AI processing.
// Use for heavy text processing that may take 3+ minutes.
func WithUnlimitedTimeout() Option {
	return func(c *Config) {
		c.DefaultTimeout = 0 // 0 = unlimited timeout
		c.DefaultTimeoutSet = true
	}
}

// WithRetries sets default HTTP retry behavior for providers that do not set
// ProviderConfig.MaxRetries or RetryDelay. maxRetries may be zero to disable
// retries by default.
func WithRetries(maxRetries int, delay time.Duration) Option {
	return func(c *Config) {
		c.DefaultRetries = maxRetries
		c.DefaultRetriesSet = true
		c.DefaultRetryDelay = delay
		c.DefaultRetryDelaySet = true
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

// WithAttemptTrace configures a callback for provider/model attempts.
func WithAttemptTrace(trace AttemptTraceFunc) Option {
	return func(c *Config) {
		c.AttemptTrace = trace
	}
}

// WithStreamIdleTimeout configures a per-chunk idle timeout for streaming responses.
// A stream that stops emitting chunks for longer than this duration fails with
// a typed timeout error. Zero or negative disables the watchdog (default).
func WithStreamIdleTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.StreamIdleTimeout = d
	}
}

// WithStreamTrace configures a callback for stream lifecycle events.
// Terminal events (StreamEnded, StreamError) are emitted exactly once per stream.
func WithStreamTrace(trace StreamTraceFunc) Option {
	return func(c *Config) {
		c.StreamTrace = trace
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

// WithIdempotencyKey adds an idempotency key to prevent duplicate operations during retries.
// When provided, the SDK will simulate server-side deduplication by caching responses.
//
// Parameters:
//   - key: Unique identifier for the operation (e.g., UUID, request hash)
//   - ttl: Time-to-live for cached responses (default: 24 hours)
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithIdempotencyKey("req-123", 1*time.Hour),
//	)
func WithIdempotencyKey(key string, ttl ...time.Duration) Option {
	return func(c *Config) {
		if c.Idempotency == nil {
			c.Idempotency = &IdempotencyConfig{}
		}
		c.Idempotency.Key = key
		if len(ttl) > 0 {
			c.Idempotency.TTL = ttl[0]
		}
	}
}

// WithModels populates the opt-in model registry with the given models.
//
// The global model registry (types.DefaultModelRegistry) starts empty. When
// model validation is enabled (the default), validation helpers have nothing
// to check against until the registry is populated. WithModels loads the
// provided models into the registry at New() time, making the opt-in explicit.
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithModels([]*types.ModelInfo{
//	        {ID: "my-model", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityChat}},
//	    }),
//	)
func WithModels(models ...*types.ModelInfo) Option {
	return func(c *Config) {
		c.Models = append(c.Models, models...)
	}
}

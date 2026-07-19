package types

import (
	"time"
)

// ProviderConfig holds provider configuration
type ProviderConfig struct {
	APIKey        string            `json:"api_key"`
	BaseURL       string            `json:"base_url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	NoAuth        bool              `json:"no_auth,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
	HTTPTimeout   *time.Duration    `json:"-"`                        // precise transport request timeout; nil means use defaults
	DynamicModels bool              `json:"dynamic_models,omitempty"` // Skip local registry validation for providers with dynamic model catalogs
	Params        map[string]any    `json:"params,omitempty"`         // Provider-specific parameters for customization

	DefaultProviderOptions map[string]any            `json:"default_provider_options,omitempty"`
	ProviderOptionsByModel map[string]map[string]any `json:"provider_options_by_model,omitempty"`
	RequestPolicy          ProviderRequestPolicy     `json:"request_policy,omitempty"`

	// ChatPath overrides the chat-completions path appended to BaseURL.
	// Empty means the provider's default ("/chat/completions" for OpenAI).
	ChatPath string `json:"chat_path,omitempty"`

	// UseResponsesAPI makes OpenAI text generation use /responses instead of
	// /chat/completions. It is opt-in because many OpenAI-compatible providers
	// only implement the chat-completions wire format.
	UseResponsesAPI bool `json:"use_responses_api,omitempty"`

	// ResponsesPath overrides the Responses API path appended to BaseURL.
	// Empty means the provider's default ("/responses" for OpenAI).
	ResponsesPath string `json:"responses_path,omitempty"`

	// ImagePath overrides the image-generation path appended to BaseURL.
	// Empty means the provider's default ("/images/generations" for OpenAI).
	ImagePath string `json:"image_path,omitempty"`

	// APIKeys, when it holds more than one entry, enables round-robin key
	// rotation on HTTP 429 within the retry path. Requires MaxRetries > 0.
	// A single key here (or only APIKey set) behaves identically to before.
	APIKeys []string `json:"api_keys,omitempty"`

	// NEW: Per-provider retry configuration (pointers allow differentiation between not set vs explicitly set to 0)
	MaxRetries    *int           `json:"max_retries,omitempty"`
	RetryDelay    *time.Duration `json:"retry_delay,omitempty"`
	RetryMaxDelay *time.Duration `json:"retry_max_delay,omitempty"`
}

// EffectiveAPIKey returns the key used for the first provider request.
// APIKey takes precedence; APIKeys[0] is the fallback for rotation-only
// configurations.
func (c ProviderConfig) EffectiveAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if len(c.APIKeys) > 0 {
		return c.APIKeys[0]
	}
	return ""
}

// ProviderRequestPolicy describes provider/model request serialization quirks
// resolved before a provider adapter serializes a request.
type ProviderRequestPolicy struct {
	MaxTokensParam      string               `json:"max_tokens_param,omitempty"`
	MaxTokensParamRules []MaxTokensParamRule `json:"max_tokens_param_rules,omitempty"`
	MaxTokensCap        int                  `json:"max_tokens_cap,omitempty"`
}

// MaxTokensParamRule selects a request parameter name when ModelContains is
// found in the model name, case-insensitively.
type MaxTokensParamRule struct {
	ModelContains string `json:"model_contains"`
	Param         string `json:"param"`
}

// NewProviderConfig creates a new ProviderConfig with the given API key.
// Use the builder methods to add optional configuration.
//
// Example:
//
//	config := types.NewProviderConfig("sk-...").
//	    WithRetries(3, 500*time.Millisecond).
//	    WithTimeout(30)
func NewProviderConfig(apiKey string) ProviderConfig {
	return ProviderConfig{APIKey: apiKey}
}

// WithBaseURL sets a custom base URL for the provider.
// Use this for self-hosted models or alternative endpoints.
func (c ProviderConfig) WithBaseURL(url string) ProviderConfig {
	c.BaseURL = url
	return c
}

// WithNoAuth disables provider authentication. Use this for local
// OpenAI-compatible endpoints that do not expect an Authorization header.
func (c ProviderConfig) WithNoAuth() ProviderConfig {
	c.NoAuth = true
	return c
}

// WithHeaders adds custom HTTP headers to all requests.
// Headers are merged with any existing headers.
func (c ProviderConfig) WithHeaders(headers map[string]string) ProviderConfig {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	for k, v := range headers {
		c.Headers[k] = v
	}
	return c
}

// WithHeader adds a single custom HTTP header.
func (c ProviderConfig) WithHeader(key, value string) ProviderConfig {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers[key] = value
	return c
}

// WithTimeout sets the request timeout in seconds.
func (c ProviderConfig) WithTimeout(seconds int) ProviderConfig {
	c.Timeout = seconds
	return c
}

// WithTimeoutDuration sets the request timeout using a time.Duration.
func (c ProviderConfig) WithTimeoutDuration(d time.Duration) ProviderConfig {
	c.Timeout = int(d.Seconds())
	c.HTTPTimeout = &d
	return c
}

// WithRetries configures retry behavior for failed requests.
// maxRetries is the maximum number of retry attempts.
// delay is the initial delay between retries.
//
// Example:
//
//	config := types.NewProviderConfig(apiKey).
//	    WithRetries(3, 500*time.Millisecond)
func (c ProviderConfig) WithRetries(maxRetries int, delay time.Duration) ProviderConfig {
	c.MaxRetries = &maxRetries
	c.RetryDelay = &delay
	return c
}

// WithNoRetries disables automatic HTTP retries for this provider.
func (c ProviderConfig) WithNoRetries() ProviderConfig {
	maxRetries := 0
	c.MaxRetries = &maxRetries
	return c
}

// WithMaxRetryDelay sets the maximum delay between retries.
// This caps exponential backoff to prevent excessive wait times.
func (c ProviderConfig) WithMaxRetryDelay(maxDelay time.Duration) ProviderConfig {
	c.RetryMaxDelay = &maxDelay
	return c
}

// WithHTTPTimeout sets the precise per-request HTTP timeout. A zero duration
// explicitly disables request timeout enforcement.
func (c ProviderConfig) WithHTTPTimeout(timeout time.Duration) ProviderConfig {
	c.HTTPTimeout = &timeout
	return c
}

// WithDynamicModels enables dynamic model discovery for this provider.
// When enabled, the provider can use any model name without local validation.
func (c ProviderConfig) WithDynamicModels() ProviderConfig {
	c.DynamicModels = true
	return c
}

// WithParam adds a provider-specific parameter.
// These are passed through to the underlying provider implementation.
func (c ProviderConfig) WithParam(key string, value any) ProviderConfig {
	if c.Params == nil {
		c.Params = make(map[string]any)
	}
	c.Params[key] = value
	return c
}

// WithParams sets multiple provider-specific parameters at once.
func (c ProviderConfig) WithParams(params map[string]any) ProviderConfig {
	if c.Params == nil {
		c.Params = make(map[string]any)
	}
	for k, v := range params {
		c.Params[k] = v
	}
	return c
}

// WithDefaultProviderOptions sets provider-specific body fields included on
// every request for this provider unless overridden by model or request options.
func (c ProviderConfig) WithDefaultProviderOptions(options map[string]any) ProviderConfig {
	c.DefaultProviderOptions = cloneAnyMap(options)
	return c
}

// WithProviderOptionsForModel sets provider-specific body fields included when
// a request uses model. These override default provider options and are
// overridden by request.ProviderOptions.
func (c ProviderConfig) WithProviderOptionsForModel(model string, options map[string]any) ProviderConfig {
	c.ProviderOptionsByModel = cloneProviderOptionsByModel(c.ProviderOptionsByModel)
	if c.ProviderOptionsByModel == nil {
		c.ProviderOptionsByModel = make(map[string]map[string]any)
	}
	c.ProviderOptionsByModel[model] = cloneAnyMap(options)
	return c
}

// MergedProviderOptions returns provider body options with precedence:
// defaults < per-model < per-request. The returned map is detached from config
// and request maps so providers can safely copy it into payloads.
func (c ProviderConfig) MergedProviderOptions(model string, requestOptions map[string]any) map[string]any {
	total := len(c.DefaultProviderOptions) + len(requestOptions)
	if perModel := c.ProviderOptionsByModel[model]; perModel != nil {
		total += len(perModel)
	}
	if total == 0 {
		return nil
	}

	merged := make(map[string]any, total)
	copyAnyMap(merged, c.DefaultProviderOptions)
	if perModel := c.ProviderOptionsByModel[model]; perModel != nil {
		copyAnyMap(merged, perModel)
	}
	copyAnyMap(merged, requestOptions)
	return merged
}

func cloneAnyMap(src map[string]any) map[string]any {
	return CloneMap(src)
}

func copyAnyMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = CloneValue(v)
	}
}

func cloneProviderOptionsByModel(src map[string]map[string]any) map[string]map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]map[string]any, len(src))
	for model, options := range src {
		dst[model] = cloneAnyMap(options)
	}
	return dst
}

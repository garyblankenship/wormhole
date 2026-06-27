package types

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Provider represents the unified LLM provider interface
// All providers embed BaseProvider and override only the methods they support
type Provider interface {
	io.Closer

	// Core provider info
	Name() string
	SupportedCapabilities() []ModelCapability

	// Text generation
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)

	// Structured output
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)

	// Embeddings
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)

	// Rerank
	Rerank(ctx context.Context, request RerankRequest) (*RerankResponse, error)

	// Audio operations
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
	SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error)
	TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error)

	// Image operations
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
	GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error)
}

// BaseProvider provides default "not implemented" implementations for all methods
// Embed this in your provider and override only the methods you support
type BaseProvider struct {
	name string
}

// NewBaseProvider creates a new base provider
func NewBaseProvider(name string) *BaseProvider {
	return &BaseProvider{name: name}
}

// Name returns the provider name
func (bp *BaseProvider) Name() string {
	return bp.name
}

// SupportedCapabilities returns an empty slice of capabilities by default
func (bp *BaseProvider) SupportedCapabilities() []ModelCapability {
	return []ModelCapability{}
}

// NotImplementedError returns a standard not implemented error
func (bp *BaseProvider) NotImplementedError(method string) error {
	return NewWormholeError(ErrorCodeProvider, fmt.Sprintf("%s provider does not support %s", bp.name, method), false)
}

// Default implementations that return not implemented errors
func (bp *BaseProvider) Text(ctx context.Context, request TextRequest) (*TextResponse, error) {
	return nil, bp.NotImplementedError("Text")
}

func (bp *BaseProvider) Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error) {
	return nil, bp.NotImplementedError("Stream")
}

func (bp *BaseProvider) Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
	return nil, bp.NotImplementedError("Structured")
}

func (bp *BaseProvider) Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
	return nil, bp.NotImplementedError("Embeddings")
}

func (bp *BaseProvider) Rerank(ctx context.Context, request RerankRequest) (*RerankResponse, error) {
	return nil, bp.NotImplementedError("Rerank")
}

func (bp *BaseProvider) Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
	return nil, bp.NotImplementedError("Audio")
}

func (bp *BaseProvider) SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error) {
	return nil, bp.NotImplementedError("SpeechToText")
}

func (bp *BaseProvider) TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error) {
	return nil, bp.NotImplementedError("TextToSpeech")
}

func (bp *BaseProvider) Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error) {
	return nil, bp.NotImplementedError("Images")
}

func (bp *BaseProvider) GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
	return nil, bp.NotImplementedError("GenerateImage")
}

// Close implements io.Closer interface for BaseProvider
func (bp *BaseProvider) Close() error {
	// Base provider has no resources to clean up
	return nil
}

// Legacy interfaces for backward compatibility - now simplified
type LegacyProvider interface {
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
	SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error)
	TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error)
	Name() string
}

// Legacy interfaces kept for backward compatibility during transition
type LegacyTextProvider interface {
	Provider
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
}

type LegacyStreamProvider interface {
	Provider
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
}

type LegacyStructuredProvider interface {
	Provider
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
}

type LegacyEmbeddingsProvider interface {
	Provider
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
}

type LegacyAudioProvider interface {
	Provider
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
}

type LegacyImageProvider interface {
	Provider
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
	GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error)
}

// ProviderConfig holds provider configuration
type ProviderConfig struct {
	APIKey        string            `json:"api_key"`
	BaseURL       string            `json:"base_url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	NoAuth        bool              `json:"no_auth,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
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

// ==================== ProviderConfig Builder Methods ====================
// These methods provide a fluent API for constructing ProviderConfig,
// eliminating the need for pointer arithmetic with optional fields.

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
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	copyAnyMap(dst, src)
	return dst
}

func copyAnyMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
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

// ==================== TLS Configuration Support ====================
// TLS configuration can be stored in the Params map under the key "tls_config".
// These methods provide type-safe access to TLS configuration.

// TLSConfigParamKey is the key used to store TLS configuration in Params map.
const TLSConfigParamKey = "tls_config"

// WithTLSConfigParam adds TLS configuration parameters to the provider config.
// The TLS configuration is stored as a map[string]any in the Params field.
// This allows type-safe extraction by providers that understand TLS configuration.
//
// Example TLS parameters:
//   - "min_version": uint16 (e.g., tls.VersionTLS12)
//   - "cipher_suites": []uint16
//   - "insecure_skip_verify": bool
//   - "server_name": string
func (c ProviderConfig) WithTLSConfigParam(key string, value any) ProviderConfig {
	if c.Params == nil {
		c.Params = make(map[string]any)
	}

	// Get or create TLS config map
	var tlsConfig map[string]any
	if existing, ok := c.Params[TLSConfigParamKey]; ok {
		if configMap, ok := existing.(map[string]any); ok {
			tlsConfig = configMap
		} else {
			tlsConfig = make(map[string]any)
		}
	} else {
		tlsConfig = make(map[string]any)
	}

	// Set the TLS parameter
	tlsConfig[key] = value
	c.Params[TLSConfigParamKey] = tlsConfig

	return c
}

// WithInsecureTLS enables insecure TLS configuration for legacy compatibility.
// WARNING: This should only be used for testing or legacy servers.
// The configuration will allow TLS 1.0 and disable certificate verification.
func (c ProviderConfig) WithInsecureTLS(skipVerify bool) ProviderConfig {
	return c.WithTLSConfigParam("min_version", uint16(0x0301)). // TLS 1.0
									WithTLSConfigParam("insecure_skip_verify", skipVerify)
}

// HasTLSConfig returns true if the provider config contains TLS configuration.
func (c ProviderConfig) HasTLSConfig() bool {
	if c.Params == nil {
		return false
	}
	_, ok := c.Params[TLSConfigParamKey]
	return ok
}

// ProviderFactory defines the function signature for creating a new provider instance.
// This enables dynamic provider registration without modifying core code.
type ProviderFactory func(config ProviderConfig) (Provider, error)

// Utility functions for capability checking - simplified since all providers now implement Provider interface
// These functions check if a method call would return a NotImplementedError
func IsMethodSupported(provider Provider, method string) bool {
	// This is a runtime check - we could enhance this by having providers expose their capabilities
	// For now, we rely on the runtime error to determine support
	return true // All providers implement all methods, some just return NotImplementedError
}

// Error checking utility - determines if an error indicates unsupported functionality
func IsNotSupportedError(err error) bool {
	if err == nil {
		return false
	}
	// Check if it's a WormholeError with ErrorCodeProvider
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeProvider
	}
	// Fallback: check error message for backward compatibility
	return err.Error() != "" &&
		(len(err.Error()) > 20 &&
			err.Error()[len(err.Error())-20:] == "does not support")
}

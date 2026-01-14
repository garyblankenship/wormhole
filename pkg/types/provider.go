package types

import (
	"context"
	"errors"
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

	// Text generation
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)

	// Structured output
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)

	// Embeddings
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)

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

// NotImplementedError returns a standard not implemented error
func (bp *BaseProvider) NotImplementedError(method string) error {
	return fmt.Errorf("%s provider does not support %s", bp.name, method)
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
	Timeout       int               `json:"timeout,omitempty"`
	DynamicModels bool              `json:"dynamic_models,omitempty"` // Skip local registry validation for providers with dynamic model catalogs
	Params        map[string]any    `json:"params,omitempty"`         // Provider-specific parameters for customization

	// NEW: Per-provider retry configuration (pointers allow differentiation between not set vs explicitly set to 0)
	MaxRetries    *int           `json:"max_retries,omitempty"`
	RetryDelay    *time.Duration `json:"retry_delay,omitempty"`
	RetryMaxDelay *time.Duration `json:"retry_max_delay,omitempty"`
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
	return errors.Is(err, errors.New("not supported")) ||
		(err.Error() != "" &&
			(len(err.Error()) > 20 &&
				err.Error()[len(err.Error())-20:] == "does not support"))
}

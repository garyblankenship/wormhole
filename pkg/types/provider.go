package types

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Provider represents the unified LLM provider interface
// All providers embed BaseProvider and override only the methods they support
type Provider interface {
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

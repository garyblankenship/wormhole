package types

import (
	"context"
)

// Provider represents the base LLM provider interface
// All providers must implement at least the Name method
type Provider interface {
	// Name returns the provider name
	Name() string
}

// FullProvider represents the legacy monolithic interface
// Deprecated: Use capability-based interfaces instead
type FullProvider interface {
	Provider
	TextProvider
	StructuredProvider
	EmbeddingsProvider
	AudioProvider
	ImageProvider
}

// LegacyProvider maintains backward compatibility
// This is the old monolithic interface
type LegacyProvider interface {
	// Text generates a text response
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)

	// Stream generates a streaming text response
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)

	// Structured generates a structured response
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)

	// Embeddings generates embeddings for input
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)

	// Audio handles audio requests (TTS and STT)
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)

	// Images generates images from prompts
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)

	// SpeechToText converts speech to text
	SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error)

	// TextToSpeech converts text to speech
	TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error)

	// Name returns the provider name
	Name() string
}

// TextProvider supports text generation
type TextProvider interface {
	Provider
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
}

// StreamProvider supports streaming text generation
type StreamProvider interface {
	Provider
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
}

// TextStreamProvider supports both text and streaming
type TextStreamProvider interface {
	TextProvider
	StreamProvider
}

// StructuredProvider supports structured output
type StructuredProvider interface {
	Provider
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
}

// EmbeddingsProvider supports embeddings generation
type EmbeddingsProvider interface {
	Provider
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
}

// AudioProvider supports audio operations
type AudioProvider interface {
	Provider
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
}

// SpeechToTextProvider supports speech-to-text conversion
type SpeechToTextProvider interface {
	Provider
	SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error)
}

// TextToSpeechProvider supports text-to-speech conversion
type TextToSpeechProvider interface {
	Provider
	TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error)
}

// ImageProvider supports image generation
type ImageProvider interface {
	Provider
	GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error)
}

// ImagesProvider supports multiple image generation (legacy)
type ImagesProvider interface {
	Provider
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
}

// ProviderConfig holds provider configuration
type ProviderConfig struct {
	APIKey        string                 `json:"api_key"`
	BaseURL       string                 `json:"base_url,omitempty"`
	Headers       map[string]string      `json:"headers,omitempty"`
	Timeout       int                    `json:"timeout,omitempty"`
	MaxRetries    int                    `json:"max_retries,omitempty"`
	RetryDelay    int                    `json:"retry_delay,omitempty"`
	DynamicModels bool                   `json:"dynamic_models,omitempty"` // Skip local registry validation for providers with dynamic model catalogs
	Params        map[string]any `json:"params,omitempty"`         // Provider-specific parameters for customization
}

// ProviderFactory defines the function signature for creating a new provider instance.
// This enables dynamic provider registration without modifying core code.
type ProviderFactory func(config ProviderConfig) (Provider, error)

// CapabilityChecker provides runtime capability checking
type CapabilityChecker interface {
	// SupportsText returns true if the provider supports text generation
	SupportsText() bool
	// SupportsStream returns true if the provider supports streaming
	SupportsStream() bool
	// SupportsStructured returns true if the provider supports structured output
	SupportsStructured() bool
	// SupportsEmbeddings returns true if the provider supports embeddings
	SupportsEmbeddings() bool
	// SupportsAudio returns true if the provider supports audio processing
	SupportsAudio() bool
	// SupportsImages returns true if the provider supports image generation
	SupportsImages() bool
}

// ProviderCapabilities provides a default implementation of capability checking
type ProviderCapabilities struct {
	Text       bool
	Stream     bool
	Structured bool
	Embeddings bool
	Audio      bool
	Images     bool
}

func (pc ProviderCapabilities) SupportsText() bool       { return pc.Text }
func (pc ProviderCapabilities) SupportsStream() bool     { return pc.Stream }
func (pc ProviderCapabilities) SupportsStructured() bool { return pc.Structured }
func (pc ProviderCapabilities) SupportsEmbeddings() bool { return pc.Embeddings }
func (pc ProviderCapabilities) SupportsAudio() bool      { return pc.Audio }
func (pc ProviderCapabilities) SupportsImages() bool     { return pc.Images }

// HasCapability checks if a provider implements a specific capability interface
func HasCapability[T any](provider Provider) bool {
	_, ok := provider.(T)
	return ok
}

// AsCapability safely casts a provider to a capability interface
func AsCapability[T any](provider Provider) (T, bool) {
	cap, ok := provider.(T)
	return cap, ok
}

// GetTextCapability returns the text capability if available
func GetTextCapability(provider Provider) (TextProvider, bool) {
	// First try the new interface
	if textProvider, ok := AsCapability[TextProvider](provider); ok {
		return textProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyTextAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// GetStreamCapability returns the stream capability if available
func GetStreamCapability(provider Provider) (StreamProvider, bool) {
	// First try the new interface
	if streamProvider, ok := AsCapability[StreamProvider](provider); ok {
		return streamProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyStreamAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// GetStructuredCapability returns the structured capability if available
func GetStructuredCapability(provider Provider) (StructuredProvider, bool) {
	// First try the new interface
	if structuredProvider, ok := AsCapability[StructuredProvider](provider); ok {
		return structuredProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyStructuredAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// GetEmbeddingsCapability returns the embeddings capability if available
func GetEmbeddingsCapability(provider Provider) (EmbeddingsProvider, bool) {
	// First try the new interface
	if embeddingsProvider, ok := AsCapability[EmbeddingsProvider](provider); ok {
		return embeddingsProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyEmbeddingsAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// GetAudioCapability returns the audio capability if available
func GetAudioCapability(provider Provider) (AudioProvider, bool) {
	// First try the new interface
	if audioProvider, ok := AsCapability[AudioProvider](provider); ok {
		return audioProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyAudioAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// GetImageCapability returns the image capability if available
func GetImageCapability(provider Provider) (ImageProvider, bool) {
	// First try the new interface
	if imageProvider, ok := AsCapability[ImageProvider](provider); ok {
		return imageProvider, true
	}

	// Fall back to legacy interface for backward compatibility
	if legacyProvider, ok := AsCapability[LegacyProvider](provider); ok {
		return &LegacyImageAdapter{legacy: legacyProvider}, true
	}

	return nil, false
}

// Legacy adapters for backward compatibility
type LegacyTextAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyTextAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyTextAdapter) Text(ctx context.Context, request TextRequest) (*TextResponse, error) {
	return a.legacy.Text(ctx, request)
}

type LegacyStreamAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyStreamAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyStreamAdapter) Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error) {
	return a.legacy.Stream(ctx, request)
}

type LegacyStructuredAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyStructuredAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyStructuredAdapter) Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
	return a.legacy.Structured(ctx, request)
}

type LegacyEmbeddingsAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyEmbeddingsAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyEmbeddingsAdapter) Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
	return a.legacy.Embeddings(ctx, request)
}

type LegacyAudioAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyAudioAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyAudioAdapter) Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
	return a.legacy.Audio(ctx, request)
}

type LegacyImageAdapter struct {
	legacy LegacyProvider
}

func (a *LegacyImageAdapter) Name() string {
	return a.legacy.Name()
}

func (a *LegacyImageAdapter) GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
	return a.legacy.Images(ctx, ImagesRequest{
		Prompt:          request.Prompt,
		Model:           request.Model,
		N:               request.N,
		Size:            request.Size,
		Quality:         request.Quality,
		Style:           request.Style,
		ResponseFormat:  request.ResponseFormat,
		ProviderOptions: request.ProviderOptions,
	})
}

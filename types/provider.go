package types

import (
	"context"
	"fmt"
	"io"
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

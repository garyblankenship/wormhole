package types

import (
	"context"
)

// ProviderWrapper wraps a provider with middleware capabilities
type ProviderWrapper struct {
	provider Provider
	chain    *ProviderMiddlewareChain
}

// NewProviderWrapper creates a new provider wrapper with middleware
func NewProviderWrapper(provider Provider, middlewares ...ProviderMiddleware) *ProviderWrapper {
	return &ProviderWrapper{
		provider: provider,
		chain:    NewProviderChain(middlewares...),
	}
}

// Name returns the underlying provider name
func (w *ProviderWrapper) Name() string {
	return w.provider.Name()
}

// Text implements TextProvider with middleware
func (w *ProviderWrapper) Text(ctx context.Context, request TextRequest) (*TextResponse, error) {
	textProvider, ok := GetTextCapability(w.provider)
	if !ok {
		return nil, NewProviderError("text generation not supported", w.provider.Name())
	}

	handler := w.chain.ApplyText(textProvider.Text)
	return handler(ctx, request)
}

// Stream implements StreamProvider with middleware
func (w *ProviderWrapper) Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error) {
	streamProvider, ok := GetStreamCapability(w.provider)
	if !ok {
		return nil, NewProviderError("streaming not supported", w.provider.Name())
	}

	handler := w.chain.ApplyStream(streamProvider.Stream)
	return handler(ctx, request)
}

// Structured implements StructuredProvider with middleware
func (w *ProviderWrapper) Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
	structuredProvider, ok := GetStructuredCapability(w.provider)
	if !ok {
		return nil, NewProviderError("structured output not supported", w.provider.Name())
	}

	handler := w.chain.ApplyStructured(structuredProvider.Structured)
	return handler(ctx, request)
}

// Embeddings implements EmbeddingsProvider with middleware
func (w *ProviderWrapper) Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
	embeddingsProvider, ok := GetEmbeddingsCapability(w.provider)
	if !ok {
		return nil, NewProviderError("embeddings not supported", w.provider.Name())
	}

	handler := w.chain.ApplyEmbeddings(embeddingsProvider.Embeddings)
	return handler(ctx, request)
}

// Audio implements AudioProvider with middleware
func (w *ProviderWrapper) Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
	audioProvider, ok := GetAudioCapability(w.provider)
	if !ok {
		return nil, NewProviderError("audio not supported", w.provider.Name())
	}

	handler := w.chain.ApplyAudio(audioProvider.Audio)
	return handler(ctx, request)
}

// GenerateImage implements ImageProvider with middleware
func (w *ProviderWrapper) GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
	imageProvider, ok := GetImageCapability(w.provider)
	if !ok {
		return nil, NewProviderError("image generation not supported", w.provider.Name())
	}

	handler := w.chain.ApplyImage(imageProvider.GenerateImage)
	return handler(ctx, request)
}

// Unwrap returns the underlying provider
func (w *ProviderWrapper) Unwrap() Provider {
	return w.provider
}

// ProviderError represents provider capability errors
type ProviderError struct {
	Message      string
	ProviderName string
}

func (e *ProviderError) Error() string {
	return e.Message + " (provider: " + e.ProviderName + ")"
}

// NewProviderError creates a new provider error
func NewProviderError(message, providerName string) *ProviderError {
	return &ProviderError{
		Message:      message,
		ProviderName: providerName,
	}
}

// IsProviderError checks if an error is a provider error
func IsProviderError(err error) bool {
	_, ok := err.(*ProviderError)
	return ok
}

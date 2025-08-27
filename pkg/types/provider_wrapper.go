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

// Text implements text generation with middleware
func (w *ProviderWrapper) Text(ctx context.Context, request TextRequest) (*TextResponse, error) {
	handler := w.chain.ApplyText(w.provider.Text)
	return handler(ctx, request)
}

// Stream implements streaming with middleware
func (w *ProviderWrapper) Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error) {
	handler := w.chain.ApplyStream(w.provider.Stream)
	return handler(ctx, request)
}

// Structured implements structured output with middleware
func (w *ProviderWrapper) Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
	handler := w.chain.ApplyStructured(w.provider.Structured)
	return handler(ctx, request)
}

// Embeddings implements embeddings with middleware
func (w *ProviderWrapper) Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
	handler := w.chain.ApplyEmbeddings(w.provider.Embeddings)
	return handler(ctx, request)
}

// Audio implements audio with middleware
func (w *ProviderWrapper) Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
	handler := w.chain.ApplyAudio(w.provider.Audio)
	return handler(ctx, request)
}

// GenerateImage implements image generation with middleware
func (w *ProviderWrapper) GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
	handler := w.chain.ApplyImage(w.provider.GenerateImage)
	return handler(ctx, request)
}

// Images implements multiple image generation with middleware
func (w *ProviderWrapper) Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error) {
	// For now, delegate directly since we don't have middleware chain for Images yet
	return w.provider.Images(ctx, request)
}

// SpeechToText implements speech-to-text with middleware
func (w *ProviderWrapper) SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error) {
	// For now, delegate directly since we don't have middleware chain for SpeechToText yet
	return w.provider.SpeechToText(ctx, request)
}

// TextToSpeech implements text-to-speech with middleware
func (w *ProviderWrapper) TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error) {
	// For now, delegate directly since we don't have middleware chain for TextToSpeech yet
	return w.provider.TextToSpeech(ctx, request)
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

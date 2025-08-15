package types

import (
	"context"
)

// Provider represents an LLM provider interface
type Provider interface {
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

	// Name returns the provider name
	Name() string
}

// TextProvider supports text generation
type TextProvider interface {
	Text(ctx context.Context, request TextRequest) (*TextResponse, error)
	Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
}

// StructuredProvider supports structured output
type StructuredProvider interface {
	Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
}

// EmbeddingsProvider supports embeddings generation
type EmbeddingsProvider interface {
	Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
}

// AudioProvider supports audio operations
type AudioProvider interface {
	Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
	SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error)
	TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error)
}

// ImageProvider supports image generation
type ImageProvider interface {
	Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
	GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error)
}

// ProviderConfig holds provider configuration
type ProviderConfig struct {
	APIKey        string            `json:"api_key"`
	BaseURL       string            `json:"base_url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
	MaxRetries    int               `json:"max_retries,omitempty"`
	RetryDelay    int               `json:"retry_delay,omitempty"`
	DynamicModels bool              `json:"dynamic_models,omitempty"` // Skip local registry validation for providers with dynamic model catalogs
}

// ProviderFactory defines the function signature for creating a new provider instance.
// This enables dynamic provider registration without modifying core code.
type ProviderFactory func(config ProviderConfig) (Provider, error)

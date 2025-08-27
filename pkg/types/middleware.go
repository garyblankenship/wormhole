package types

import (
	"context"
)

// ProviderMiddleware represents middleware that can be applied at the provider level
type ProviderMiddleware interface {
	// ApplyText wraps text generation calls
	ApplyText(next TextHandler) TextHandler
	// ApplyStream wraps streaming calls
	ApplyStream(next StreamHandler) StreamHandler
	// ApplyStructured wraps structured output calls
	ApplyStructured(next StructuredHandler) StructuredHandler
	// ApplyEmbeddings wraps embeddings calls
	ApplyEmbeddings(next EmbeddingsHandler) EmbeddingsHandler
	// ApplyAudio wraps audio calls
	ApplyAudio(next AudioHandler) AudioHandler
	// ApplyImage wraps image generation calls
	ApplyImage(next ImageHandler) ImageHandler
}

// Handler function types for different capabilities
type TextHandler func(ctx context.Context, request TextRequest) (*TextResponse, error)
type StreamHandler func(ctx context.Context, request TextRequest) (<-chan StreamChunk, error)
type StructuredHandler func(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
type EmbeddingsHandler func(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
type AudioHandler func(ctx context.Context, request AudioRequest) (*AudioResponse, error)
type ImageHandler func(ctx context.Context, request ImageRequest) (*ImageResponse, error)

// ProviderMiddlewareChain manages provider-level middleware
type ProviderMiddlewareChain struct {
	middlewares []ProviderMiddleware
}

// NewProviderChain creates a new provider middleware chain
func NewProviderChain(middlewares ...ProviderMiddleware) *ProviderMiddlewareChain {
	return &ProviderMiddlewareChain{
		middlewares: middlewares,
	}
}

// ApplyText applies the middleware chain to a text handler
func (c *ProviderMiddlewareChain) ApplyText(handler TextHandler) TextHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyText(result)
	}
	return result
}

// ApplyStream applies the middleware chain to a stream handler
func (c *ProviderMiddlewareChain) ApplyStream(handler StreamHandler) StreamHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyStream(result)
	}
	return result
}

// ApplyStructured applies the middleware chain to a structured handler
func (c *ProviderMiddlewareChain) ApplyStructured(handler StructuredHandler) StructuredHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyStructured(result)
	}
	return result
}

// ApplyEmbeddings applies the middleware chain to an embeddings handler
func (c *ProviderMiddlewareChain) ApplyEmbeddings(handler EmbeddingsHandler) EmbeddingsHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyEmbeddings(result)
	}
	return result
}

// ApplyAudio applies the middleware chain to an audio handler
func (c *ProviderMiddlewareChain) ApplyAudio(handler AudioHandler) AudioHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyAudio(result)
	}
	return result
}

// ApplyImage applies the middleware chain to an image handler
func (c *ProviderMiddlewareChain) ApplyImage(handler ImageHandler) ImageHandler {
	if len(c.middlewares) == 0 {
		return handler
	}

	result := handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		result = c.middlewares[i].ApplyImage(result)
	}
	return result
}

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

// applyChain applies a slice of middlewares in reverse order to handler,
// using wrap to dispatch the per-middleware Apply* call. Early-returns when
// the slice is empty so callers pay no allocation cost for the common case.
func applyChain[H any](middlewares []ProviderMiddleware, handler H, wrap func(ProviderMiddleware, H) H) H {
	if len(middlewares) == 0 {
		return handler
	}
	result := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		result = wrap(middlewares[i], result)
	}
	return result
}

// ApplyText applies the middleware chain to a text handler.
func (c *ProviderMiddlewareChain) ApplyText(handler TextHandler) TextHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h TextHandler) TextHandler { return mw.ApplyText(h) })
}

// ApplyStream applies the middleware chain to a stream handler.
func (c *ProviderMiddlewareChain) ApplyStream(handler StreamHandler) StreamHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h StreamHandler) StreamHandler { return mw.ApplyStream(h) })
}

// ApplyStructured applies the middleware chain to a structured handler.
func (c *ProviderMiddlewareChain) ApplyStructured(handler StructuredHandler) StructuredHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h StructuredHandler) StructuredHandler { return mw.ApplyStructured(h) })
}

// ApplyEmbeddings applies the middleware chain to an embeddings handler.
func (c *ProviderMiddlewareChain) ApplyEmbeddings(handler EmbeddingsHandler) EmbeddingsHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h EmbeddingsHandler) EmbeddingsHandler { return mw.ApplyEmbeddings(h) })
}

// ApplyAudio applies the middleware chain to an audio handler.
func (c *ProviderMiddlewareChain) ApplyAudio(handler AudioHandler) AudioHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h AudioHandler) AudioHandler { return mw.ApplyAudio(h) })
}

// ApplyImage applies the middleware chain to an image handler.
func (c *ProviderMiddlewareChain) ApplyImage(handler ImageHandler) ImageHandler {
	return applyChain(c.middlewares, handler, func(mw ProviderMiddleware, h ImageHandler) ImageHandler { return mw.ApplyImage(h) })
}

package middleware

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// LegacyAdapter wraps a legacy Middleware function into a ProviderMiddleware interface
// This provides backward compatibility during migration from legacy Chain to type-safe ProviderMiddlewareChain
type LegacyAdapter struct {
	mw Middleware
}

// NewLegacyAdapter creates a new adapter that wraps legacy middleware into ProviderMiddleware
func NewLegacyAdapter(mw Middleware) *LegacyAdapter {
	return &LegacyAdapter{mw: mw}
}

// ApplyText wraps text generation calls using the legacy middleware
func (a *LegacyAdapter) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			textReq := req.(*types.TextRequest)
			return next(ctx, *textReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.TextResponse), nil
	}
}

// ApplyStream wraps streaming calls using the legacy middleware
func (a *LegacyAdapter) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			textReq := req.(*types.TextRequest)
			return next(ctx, *textReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(<-chan types.StreamChunk), nil
	}
}

// ApplyStructured wraps structured output calls using the legacy middleware
func (a *LegacyAdapter) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			structuredReq := req.(*types.StructuredRequest)
			return next(ctx, *structuredReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.StructuredResponse), nil
	}
}

// ApplyEmbeddings wraps embeddings calls using the legacy middleware
func (a *LegacyAdapter) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			embeddingsReq := req.(*types.EmbeddingsRequest)
			return next(ctx, *embeddingsReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.EmbeddingsResponse), nil
	}
}

// ApplyAudio wraps audio calls using the legacy middleware
func (a *LegacyAdapter) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			audioReq := req.(*types.AudioRequest)
			return next(ctx, *audioReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.AudioResponse), nil
	}
}

// ApplyImage wraps image generation calls using the legacy middleware
func (a *LegacyAdapter) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		// Wrap the typed handler in a legacy handler
		legacyHandler := func(ctx context.Context, req any) (any, error) {
			imageReq := req.(*types.ImageRequest)
			return next(ctx, *imageReq)
		}

		// Apply legacy middleware
		wrappedHandler := a.mw(legacyHandler)

		// Execute with type conversion
		resp, err := wrappedHandler(ctx, &request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.ImageResponse), nil
	}
}
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

// applyLegacy is the shared wrap-unwrap core used by all Apply* methods.
// It boxes req as a pointer through the legacy any-typed Middleware and unboxes the result.
func applyLegacy[Req any, Resp any](mw Middleware, ctx context.Context, req Req, next func(context.Context, Req) (Resp, error)) (Resp, error) {
	legacyHandler := func(ctx context.Context, r any) (any, error) {
		return next(ctx, *r.(*Req))
	}
	resp, err := mw(legacyHandler)(ctx, &req)
	if err != nil {
		var zero Resp
		return zero, err
	}
	return resp.(Resp), nil
}

// ApplyText wraps text generation calls using the legacy middleware
func (a *LegacyAdapter) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

// ApplyStream wraps streaming calls using the legacy middleware
func (a *LegacyAdapter) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, req types.TextRequest) (<-chan types.StreamChunk, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

// ApplyStructured wraps structured output calls using the legacy middleware
func (a *LegacyAdapter) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, req types.StructuredRequest) (*types.StructuredResponse, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

// ApplyEmbeddings wraps embeddings calls using the legacy middleware
func (a *LegacyAdapter) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

// ApplyAudio wraps audio calls using the legacy middleware
func (a *LegacyAdapter) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, req types.AudioRequest) (*types.AudioResponse, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

// ApplyImage wraps image generation calls using the legacy middleware
func (a *LegacyAdapter) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, req types.ImageRequest) (*types.ImageResponse, error) {
		return applyLegacy(a.mw, ctx, req, next)
	}
}

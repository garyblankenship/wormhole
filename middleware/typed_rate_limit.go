package middleware

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TypedRateLimitMiddleware applies one cancellation-aware token bucket across
// every provider operation that shares this middleware instance.
type TypedRateLimitMiddleware struct {
	limiter *RateLimiter
}

// NewTypedRateLimitMiddleware creates typed provider middleware with a shared
// request-per-second budget. Values below one use a one-request-per-second
// minimum.
func NewTypedRateLimitMiddleware(requestsPerSecond int) *TypedRateLimitMiddleware {
	return &TypedRateLimitMiddleware{limiter: NewRateLimiter(requestsPerSecond)}
}

func typedRateLimit[Req any, Resp any](m *TypedRateLimitMiddleware, next func(context.Context, Req) (Resp, error)) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, request Req) (Resp, error) {
		if err := m.limiter.Wait(ctx); err != nil {
			var zero Resp
			return zero, wrapMiddlewareError("rate_limiter", "wait", err)
		}
		return next(ctx, request)
	}
}

func (m *TypedRateLimitMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return typedRateLimit(m, next)
}

func (m *TypedRateLimitMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return typedRateLimit(m, next)
}

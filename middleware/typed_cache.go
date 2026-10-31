package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TypedCacheMiddleware caches detached non-stream responses. Cache keys include
// both operation and provider identity, so shared caches cannot cross routes.
type TypedCacheMiddleware struct {
	config CacheConfig
	fills  cacheFillGroup
}

// NewTypedCacheMiddleware creates typed response-caching middleware.
func NewTypedCacheMiddleware(config CacheConfig) *TypedCacheMiddleware {
	if config.KeyGenerator == nil {
		config.KeyGenerator = DefaultCacheKeyGenerator
	}
	return &TypedCacheMiddleware{config: config}
}

func typedCache[Req any, Resp any](m *TypedCacheMiddleware, operation string, ctx context.Context, request Req, next func(context.Context, Req) (Resp, error)) (Resp, error) {
	if types.IsCachePrewarm(request) || m.config.Cache == nil || (m.config.CacheableFunc != nil && !m.config.CacheableFunc(request)) {
		return next(ctx, request)
	}
	key, err := m.config.KeyGenerator(request)
	if err != nil {
		return next(ctx, request)
	}
	if endpoint, _ := ctx.Value(CtxKeyBaseURLOverride).(string); endpoint != "" {
		// Hash endpoint data rather than retaining credentials or query strings in keys.
		key = fmt.Sprintf("endpoint:%x:%s", sha256.Sum256([]byte(endpoint)), key)
	}
	key = fmt.Sprintf("%s:%s:%s", operation, providerIdentity(ctx), key)
	fillIdentity := ""
	if m.config.CoordinateFills && m.config.FillIdentity != nil {
		if identity := m.config.FillIdentity(ctx, request); identity != "" {
			fillIdentity = fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
			// The same isolation boundary must cover stored hits as well as
			// in-flight sharing, including when the flight limit is full.
			key = fmt.Sprintf("fill:%s:%s", fillIdentity, key)
		}
	}
	if cached, ok := m.config.Cache.Get(key); ok {
		if response, ok := clonedCacheResponse[Resp](cached); ok {
			return response, nil
		}
	}

	fill := func() (Resp, any, bool, error) {
		response, err := next(ctx, request)
		if err != nil {
			return response, nil, false, err
		}
		stored, cloneErr := cloneValue(response)
		if cloneErr == nil && (!m.config.CoordinateFills || ctx.Err() == nil) {
			m.config.Cache.Set(key, stored, m.config.TTL)
			return response, stored, true, nil
		}
		return response, nil, false, nil
	}
	if m.canCoordinateFill(ctx) {
		if identity := fillIdentity; identity != "" {
			flight, leader := m.fills.join(key, identity, m.config.MaxConcurrentFills)
			if flight != nil {
				if !leader {
					value, ok, waitErr := waitCacheFill(ctx, flight)
					if waitErr != nil {
						var zero Resp
						return zero, waitErr
					}
					if ok {
						if response, ok := clonedCacheResponse[Resp](value); ok {
							return response, nil
						}
					}
					// A failed/canceled leader cannot own another caller's outcome.
					// Retry with the waiter's original context and request.
					response, _, _, err := fill()
					return response, err
				}
				// Another leader may have finished between the first Get and join.
				// Recheck storage before issuing another downstream call.
				if cached, ok := m.config.Cache.Get(key); ok {
					if response, ok := clonedCacheResponse[Resp](cached); ok {
						m.fills.finish(key, identity, flight, cached, true)
						return response, nil
					}
				}
				var shared any
				var shareable bool
				defer func() { m.fills.finish(key, identity, flight, shared, shareable) }()
				response, stored, ok, err := fill()
				shared, shareable = stored, ok
				return response, err
			}
		}
	}
	response, _, _, err := fill()
	return response, err
}

func (m *TypedCacheMiddleware) canCoordinateFill(ctx context.Context) bool {
	return m.config.CoordinateFills && m.config.MaxConcurrentFills > 0 && ctx.Err() == nil
}

func clonedCacheResponse[Resp any](value any) (Resp, bool) {
	cloned, err := cloneValue(value)
	if err != nil {
		var zero Resp
		return zero, false
	}
	response, ok := cloned.(Resp)
	return response, ok
}

func (m *TypedCacheMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return typedCache(m, "text", ctx, request, next)
	}
}

// Streams deliberately bypass storage: a live channel is single-consumer and
// cannot be detached safely.
func (m *TypedCacheMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return next
}

func (m *TypedCacheMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return typedCache(m, "structured", ctx, request, next)
	}
}

func (m *TypedCacheMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return typedCache(m, "embeddings", ctx, request, next)
	}
}

func (m *TypedCacheMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return func(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
		return typedCache(m, "rerank", ctx, request, next)
	}
}

func (m *TypedCacheMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return typedCache(m, "audio", ctx, request, next)
	}
}

func (m *TypedCacheMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return typedCache(m, "image", ctx, request, next)
	}
}

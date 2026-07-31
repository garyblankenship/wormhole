package middleware

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TypedCacheMiddleware caches detached non-stream responses. Cache keys include
// both operation and provider identity, so shared caches cannot cross routes.
type TypedCacheMiddleware struct {
	config CacheConfig
}

// NewTypedCacheMiddleware creates typed response-caching middleware.
func NewTypedCacheMiddleware(config CacheConfig) *TypedCacheMiddleware {
	if config.KeyGenerator == nil {
		config.KeyGenerator = DefaultCacheKeyGenerator
	}
	return &TypedCacheMiddleware{config: config}
}

func typedCache[Req any, Resp any](m *TypedCacheMiddleware, operation string, ctx context.Context, request Req, next func(context.Context, Req) (Resp, error)) (Resp, error) {
	if m.config.Cache == nil || (m.config.CacheableFunc != nil && !m.config.CacheableFunc(request)) {
		return next(ctx, request)
	}
	key, err := m.config.KeyGenerator(request)
	if err != nil {
		return next(ctx, request)
	}
	key = fmt.Sprintf("%s:%s:%s", operation, providerIdentity(ctx), key)
	if cached, ok := m.config.Cache.Get(key); ok {
		cloned, cloneErr := cloneValue(cached)
		if cloneErr == nil {
			if response, ok := cloned.(Resp); ok {
				return response, nil
			}
		}
	}

	response, err := next(ctx, request)
	if err != nil {
		return response, err
	}
	stored, cloneErr := cloneValue(response)
	if cloneErr == nil {
		m.config.Cache.Set(key, stored, m.config.TTL)
	}
	return response, nil
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

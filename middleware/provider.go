package middleware

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// JSONCleaningMiddleware provides JSON response cleaning for providers
type JSONCleaningMiddleware struct {
	providers map[string]bool // Providers that need JSON cleaning
}

// NewJSONCleaningMiddleware creates middleware for cleaning JSON responses
func NewJSONCleaningMiddleware(providerNames ...string) *JSONCleaningMiddleware {
	providers := make(map[string]bool)
	for _, name := range providerNames {
		providers[strings.ToLower(name)] = true
	}
	return &JSONCleaningMiddleware{providers: providers}
}

func (m *JSONCleaningMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return next // Text responses don't need JSON cleaning
}

func (m *JSONCleaningMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return next // Stream responses don't need JSON cleaning
}

func (m *JSONCleaningMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		resp, err := next(ctx, request)
		if err != nil {
			return nil, err
		}

		// Apply JSON cleaning if this provider needs it
		if resp != nil && resp.Raw != "" {
			// Use standard JSON parsing for validation
			var data any
			if err := json.Unmarshal([]byte(resp.Raw), &data); err != nil {
				// If it fails, try to clean the JSON
				cleaned := cleanJSONResponse(resp.Raw)
				if err := json.Unmarshal([]byte(cleaned), &data); err == nil {
					resp.Raw = cleaned
				}
				// If cleaning doesn't help, leave original response
			}
		}

		return resp, nil
	}
}

func (m *JSONCleaningMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return next // Embeddings don't need JSON cleaning
}

func (m *JSONCleaningMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return next // Rerank doesn't need JSON cleaning
}

func (m *JSONCleaningMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return next // Audio responses don't need JSON cleaning
}

func (m *JSONCleaningMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return next // Image responses don't need JSON cleaning
}

// ProviderMetricsMiddleware provides metrics collection at the provider level
type ProviderMetricsMiddleware struct {
	providerName string
	metrics      *ProviderMetrics
}

// ProviderMetrics holds provider-specific metrics
type ProviderMetrics struct {
	TextRequests       int64
	StreamRequests     int64
	StructuredRequests int64
	EmbeddingsRequests int64
	AudioRequests      int64
	ImageRequests      int64
	TotalErrors        int64
	TotalLatencyMs     int64
	RerankRequests     int64
}

// NewProviderMetricsMiddleware creates middleware for provider metrics
func NewProviderMetricsMiddleware(providerName string) *ProviderMetricsMiddleware {
	return &ProviderMetricsMiddleware{
		providerName: providerName,
		metrics:      &ProviderMetrics{},
	}
}

// GetMetrics returns current provider metrics
func (m *ProviderMetricsMiddleware) GetMetrics() *ProviderMetrics {
	return m.metrics
}

func (m *ProviderMetricsMiddleware) recordRequest(counter *int64, elapsed time.Duration, err error) {
	atomic.AddInt64(counter, 1)
	atomic.AddInt64(&m.metrics.TotalLatencyMs, elapsed.Milliseconds())
	if err != nil {
		atomic.AddInt64(&m.metrics.TotalErrors, 1)
	}
}

func (m *ProviderMetricsMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.TextResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.TextRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		return withMeasuredRequest(ctx, request, next, func(_ <-chan types.TextChunk, err error, d time.Duration) {
			m.recordRequest(&m.metrics.StreamRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.StructuredResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.StructuredRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.EmbeddingsResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.EmbeddingsRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return func(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.RerankResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.RerankRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.AudioResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.AudioRequests, d, err)
		})
	}
}

func (m *ProviderMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.ImageResponse, err error, d time.Duration) {
			m.recordRequest(&m.metrics.ImageRequests, d, err)
		})
	}
}

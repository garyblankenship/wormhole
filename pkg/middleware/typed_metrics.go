package middleware

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TypedMetricsMiddleware implements the ProviderMiddleware interface with type-safe metrics collection
type TypedMetricsMiddleware struct {
	metrics *TypedMetrics
}

// TypedMetrics tracks provider metrics using atomic operations for better performance
type TypedMetrics struct {
	// Text generation metrics
	textRequests int64
	textErrors   int64
	textDuration int64 // nanoseconds

	// Streaming metrics
	streamRequests int64
	streamErrors   int64
	streamDuration int64 // nanoseconds

	// Structured output metrics
	structuredRequests int64
	structuredErrors   int64
	structuredDuration int64 // nanoseconds

	// Embeddings metrics
	embeddingsRequests int64
	embeddingsErrors   int64
	embeddingsDuration int64 // nanoseconds

	// Audio metrics
	audioRequests int64
	audioErrors   int64
	audioDuration int64 // nanoseconds

	// Image metrics
	imageRequests int64
	imageErrors   int64
	imageDuration int64 // nanoseconds
}

// NewTypedMetricsMiddleware creates a new type-safe metrics middleware
func NewTypedMetricsMiddleware(metrics *TypedMetrics) *TypedMetricsMiddleware {
	return &TypedMetricsMiddleware{
		metrics: metrics,
	}
}

// NewTypedMetrics creates a new typed metrics instance
func NewTypedMetrics() *TypedMetrics {
	return &TypedMetrics{}
}

// ApplyText wraps text generation calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.TextResponse, err error, duration time.Duration) {
			m.recordTextRequest(duration, err)
		})
	}
}

// ApplyStream wraps streaming calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		return withMeasuredRequest(ctx, request, next, func(_ <-chan types.TextChunk, err error, duration time.Duration) {
			m.recordStreamRequest(duration, err)
		})
	}
}

// ApplyStructured wraps structured output calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.StructuredResponse, err error, duration time.Duration) {
			m.recordStructuredRequest(duration, err)
		})
	}
}

// ApplyEmbeddings wraps embeddings calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.EmbeddingsResponse, err error, duration time.Duration) {
			m.recordEmbeddingsRequest(duration, err)
		})
	}
}

// ApplyAudio wraps audio calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.AudioResponse, err error, duration time.Duration) {
			m.recordAudioRequest(duration, err)
		})
	}
}

// ApplyImage wraps image generation calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.ImageResponse, err error, duration time.Duration) {
			m.recordImageRequest(duration, err)
		})
	}
}

// recordRequest is a shared helper that atomically increments counters for a single operation type.
func recordRequest(reqs, errs, dur *int64, duration time.Duration, err error) {
	atomic.AddInt64(reqs, 1)
	atomic.AddInt64(dur, int64(duration))
	if err != nil {
		atomic.AddInt64(errs, 1)
	}
}

// Metrics recording methods

func (m *TypedMetricsMiddleware) recordTextRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.textRequests, &m.metrics.textErrors, &m.metrics.textDuration, duration, err)
}

func (m *TypedMetricsMiddleware) recordStreamRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.streamRequests, &m.metrics.streamErrors, &m.metrics.streamDuration, duration, err)
}

func (m *TypedMetricsMiddleware) recordStructuredRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.structuredRequests, &m.metrics.structuredErrors, &m.metrics.structuredDuration, duration, err)
}

func (m *TypedMetricsMiddleware) recordEmbeddingsRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.embeddingsRequests, &m.metrics.embeddingsErrors, &m.metrics.embeddingsDuration, duration, err)
}

func (m *TypedMetricsMiddleware) recordAudioRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.audioRequests, &m.metrics.audioErrors, &m.metrics.audioDuration, duration, err)
}

func (m *TypedMetricsMiddleware) recordImageRequest(duration time.Duration, err error) {
	recordRequest(&m.metrics.imageRequests, &m.metrics.imageErrors, &m.metrics.imageDuration, duration, err)
}

// getStats is a shared helper that atomically reads counters and computes average duration.
func getStats(reqs, errs, dur *int64) (requests, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(reqs)
	errors = atomic.LoadInt64(errs)
	totalDuration := atomic.LoadInt64(dur)
	if requests > 0 {
		avgDuration = time.Duration(totalDuration / requests)
	}
	return
}

// TypedMetrics getter methods

// GetTextStats returns current text generation metrics
func (m *TypedMetrics) GetTextStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.textRequests, &m.textErrors, &m.textDuration)
}

// GetStreamStats returns current streaming metrics
func (m *TypedMetrics) GetStreamStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.streamRequests, &m.streamErrors, &m.streamDuration)
}

// GetStructuredStats returns current structured output metrics
func (m *TypedMetrics) GetStructuredStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.structuredRequests, &m.structuredErrors, &m.structuredDuration)
}

// GetEmbeddingsStats returns current embeddings metrics
func (m *TypedMetrics) GetEmbeddingsStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.embeddingsRequests, &m.embeddingsErrors, &m.embeddingsDuration)
}

// GetAudioStats returns current audio metrics
func (m *TypedMetrics) GetAudioStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.audioRequests, &m.audioErrors, &m.audioDuration)
}

// GetImageStats returns current image generation metrics
func (m *TypedMetrics) GetImageStats() (requests int64, errors int64, avgDuration time.Duration) {
	return getStats(&m.imageRequests, &m.imageErrors, &m.imageDuration)
}

// GetAllStats returns comprehensive metrics across all operation types
func (m *TypedMetrics) GetAllStats() map[string]interface{} {
	textReq, textErr, textAvg := m.GetTextStats()
	streamReq, streamErr, streamAvg := m.GetStreamStats()
	structuredReq, structuredErr, structuredAvg := m.GetStructuredStats()
	embeddingsReq, embeddingsErr, embeddingsAvg := m.GetEmbeddingsStats()
	audioReq, audioErr, audioAvg := m.GetAudioStats()
	imageReq, imageErr, imageAvg := m.GetImageStats()

	return map[string]interface{}{
		"text": map[string]interface{}{
			"requests":     textReq,
			"errors":       textErr,
			"avg_duration": textAvg,
		},
		"stream": map[string]interface{}{
			"requests":     streamReq,
			"errors":       streamErr,
			"avg_duration": streamAvg,
		},
		"structured": map[string]interface{}{
			"requests":     structuredReq,
			"errors":       structuredErr,
			"avg_duration": structuredAvg,
		},
		"embeddings": map[string]interface{}{
			"requests":     embeddingsReq,
			"errors":       embeddingsErr,
			"avg_duration": embeddingsAvg,
		},
		"audio": map[string]interface{}{
			"requests":     audioReq,
			"errors":       audioErr,
			"avg_duration": audioAvg,
		},
		"image": map[string]interface{}{
			"requests":     imageReq,
			"errors":       imageErr,
			"avg_duration": imageAvg,
		},
	}
}

// Reset clears all metrics counters
func (m *TypedMetrics) Reset() {
	fields := []*int64{
		&m.textRequests, &m.textErrors, &m.textDuration,
		&m.streamRequests, &m.streamErrors, &m.streamDuration,
		&m.structuredRequests, &m.structuredErrors, &m.structuredDuration,
		&m.embeddingsRequests, &m.embeddingsErrors, &m.embeddingsDuration,
		&m.audioRequests, &m.audioErrors, &m.audioDuration,
		&m.imageRequests, &m.imageErrors, &m.imageDuration,
	}
	for _, f := range fields {
		atomic.StoreInt64(f, 0)
	}
}

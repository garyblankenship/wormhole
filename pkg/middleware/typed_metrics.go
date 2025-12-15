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
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)
		m.recordTextRequest(duration, err)

		return resp, err
	}
}

// ApplyStream wraps streaming calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		start := time.Now()

		stream, err := next(ctx, request)

		duration := time.Since(start)
		m.recordStreamRequest(duration, err)

		return stream, err
	}
}

// ApplyStructured wraps structured output calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)
		m.recordStructuredRequest(duration, err)

		return resp, err
	}
}

// ApplyEmbeddings wraps embeddings calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)
		m.recordEmbeddingsRequest(duration, err)

		return resp, err
	}
}

// ApplyAudio wraps audio calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)
		m.recordAudioRequest(duration, err)

		return resp, err
	}
}

// ApplyImage wraps image generation calls with metrics collection
func (m *TypedMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)
		m.recordImageRequest(duration, err)

		return resp, err
	}
}

// Metrics recording methods

func (m *TypedMetricsMiddleware) recordTextRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.textRequests, 1)
	atomic.AddInt64(&m.metrics.textDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.textErrors, 1)
	}
}

func (m *TypedMetricsMiddleware) recordStreamRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.streamRequests, 1)
	atomic.AddInt64(&m.metrics.streamDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.streamErrors, 1)
	}
}

func (m *TypedMetricsMiddleware) recordStructuredRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.structuredRequests, 1)
	atomic.AddInt64(&m.metrics.structuredDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.structuredErrors, 1)
	}
}

func (m *TypedMetricsMiddleware) recordEmbeddingsRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.embeddingsRequests, 1)
	atomic.AddInt64(&m.metrics.embeddingsDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.embeddingsErrors, 1)
	}
}

func (m *TypedMetricsMiddleware) recordAudioRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.audioRequests, 1)
	atomic.AddInt64(&m.metrics.audioDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.audioErrors, 1)
	}
}

func (m *TypedMetricsMiddleware) recordImageRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.metrics.imageRequests, 1)
	atomic.AddInt64(&m.metrics.imageDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.metrics.imageErrors, 1)
	}
}

// TypedMetrics getter methods

// GetTextStats returns current text generation metrics
func (m *TypedMetrics) GetTextStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.textRequests)
	errors = atomic.LoadInt64(&m.textErrors)
	totalDurationNs := atomic.LoadInt64(&m.textDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// GetStreamStats returns current streaming metrics
func (m *TypedMetrics) GetStreamStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.streamRequests)
	errors = atomic.LoadInt64(&m.streamErrors)
	totalDurationNs := atomic.LoadInt64(&m.streamDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// GetStructuredStats returns current structured output metrics
func (m *TypedMetrics) GetStructuredStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.structuredRequests)
	errors = atomic.LoadInt64(&m.structuredErrors)
	totalDurationNs := atomic.LoadInt64(&m.structuredDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// GetEmbeddingsStats returns current embeddings metrics
func (m *TypedMetrics) GetEmbeddingsStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.embeddingsRequests)
	errors = atomic.LoadInt64(&m.embeddingsErrors)
	totalDurationNs := atomic.LoadInt64(&m.embeddingsDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// GetAudioStats returns current audio metrics
func (m *TypedMetrics) GetAudioStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.audioRequests)
	errors = atomic.LoadInt64(&m.audioErrors)
	totalDurationNs := atomic.LoadInt64(&m.audioDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// GetImageStats returns current image generation metrics
func (m *TypedMetrics) GetImageStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.imageRequests)
	errors = atomic.LoadInt64(&m.imageErrors)
	totalDurationNs := atomic.LoadInt64(&m.imageDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
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
	atomic.StoreInt64(&m.textRequests, 0)
	atomic.StoreInt64(&m.textErrors, 0)
	atomic.StoreInt64(&m.textDuration, 0)

	atomic.StoreInt64(&m.streamRequests, 0)
	atomic.StoreInt64(&m.streamErrors, 0)
	atomic.StoreInt64(&m.streamDuration, 0)

	atomic.StoreInt64(&m.structuredRequests, 0)
	atomic.StoreInt64(&m.structuredErrors, 0)
	atomic.StoreInt64(&m.structuredDuration, 0)

	atomic.StoreInt64(&m.embeddingsRequests, 0)
	atomic.StoreInt64(&m.embeddingsErrors, 0)
	atomic.StoreInt64(&m.embeddingsDuration, 0)

	atomic.StoreInt64(&m.audioRequests, 0)
	atomic.StoreInt64(&m.audioErrors, 0)
	atomic.StoreInt64(&m.audioDuration, 0)

	atomic.StoreInt64(&m.imageRequests, 0)
	atomic.StoreInt64(&m.imageErrors, 0)
	atomic.StoreInt64(&m.imageDuration, 0)
}

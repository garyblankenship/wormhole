package middleware

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TypedEnhancedMetricsMiddleware implements the ProviderMiddleware interface with enhanced metrics collection
type TypedEnhancedMetricsMiddleware struct {
	collector *EnhancedMetricsCollector
}

// NewTypedEnhancedMetricsMiddleware creates a new type-safe enhanced metrics middleware
func NewTypedEnhancedMetricsMiddleware(collector *EnhancedMetricsCollector) *TypedEnhancedMetricsMiddleware {
	return &TypedEnhancedMetricsMiddleware{
		collector: collector,
	}
}

// ApplyText wraps text generation calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(resp *types.TextResponse, err error, duration time.Duration) {
			outputTokens := 0
			if resp != nil {
				outputTokens = estimateTextTokens(resp.Text)
			}
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "text", request.Model),
				duration,
				err,
				0,
				estimateInputTokens(request.Messages),
				outputTokens,
			)
		})
	}
}

// ApplyStream wraps streaming calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		return withMeasuredRequest(ctx, request, next, func(_ <-chan types.TextChunk, err error, duration time.Duration) {
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "stream", request.Model),
				duration,
				err,
				0,
				estimateInputTokens(request.Messages),
				0,
			)
		})
	}
}

// ApplyStructured wraps structured output calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(resp *types.StructuredResponse, err error, duration time.Duration) {
			outputTokens := 0
			if resp != nil {
				outputTokens = estimateStructuredOutputTokens(resp.Content)
			}
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "structured", request.Model),
				duration,
				err,
				0,
				estimateInputTokens(request.Messages),
				outputTokens,
			)
		})
	}
}

// ApplyEmbeddings wraps embeddings calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		inputTokens := 0
		for _, text := range request.Input {
			inputTokens += estimateTextTokens(text)
		}

		return withMeasuredRequest(ctx, request, next, func(_ *types.EmbeddingsResponse, err error, duration time.Duration) {
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "embeddings", request.Model),
				duration,
				err,
				0,
				inputTokens,
				0,
			)
		})
	}
}

// ApplyAudio wraps audio calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		inputTokens := 0
		if request.Type == "tts" {
			if text, ok := request.Input.(string); ok {
				inputTokens = estimateTextTokens(text)
			}
		}

		return withMeasuredRequest(ctx, request, next, func(_ *types.AudioResponse, err error, duration time.Duration) {
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "audio", request.Model),
				duration,
				err,
				0,
				inputTokens,
				0,
			)
		})
	}
}

// ApplyImage wraps image generation calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withMeasuredRequest(ctx, request, next, func(_ *types.ImageResponse, err error, duration time.Duration) {
			m.collector.RecordRequest(
				requestLabelsFromContext(ctx, "image", request.Model),
				duration,
				err,
				0,
				estimateTextTokens(request.Prompt),
				0,
			)
		})
	}
}

// Helper functions for token estimation

// estimateInputTokens estimates tokens from messages
func estimateInputTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		content := msg.GetContent()
		if str, ok := content.(string); ok {
			total += estimateTextTokens(str)
		}
	}
	return total
}

// estimateStructuredOutputTokens estimates tokens from structured output
func estimateStructuredOutputTokens(content interface{}) int {
	// Simplified estimation - in production, you'd serialize and count
	return 100 // Rough estimate
}

// estimateTextTokens estimates tokens in text (rough approximation)
func estimateTextTokens(text string) int {
	// Rough approximation: 1 token ≈ 4 characters for English
	// This is a simplified estimation
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

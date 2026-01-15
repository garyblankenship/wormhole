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

// extractLabels extracts labels from context
func (m *TypedEnhancedMetricsMiddleware) extractLabels(ctx context.Context, method, model string) *RequestLabels {
	provider := "unknown"

	// Try to extract provider from context
	if ctx != nil {
		if p, ok := ctx.Value("wormhole_provider").(string); ok {
			provider = p
		} else if p, ok := ctx.Value("provider").(string); ok {
			provider = p
		}
	}

	return &RequestLabels{
		Provider: provider,
		Model:    model,
		Method:   method,
		ErrorType: "", // Will be detected by error detector
	}
}

// ApplyText wraps text generation calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "text", request.Model)

		// Estimate token counts if available
		inputTokens := estimateInputTokens(request.Messages)
		outputTokens := 0
		if resp != nil {
			// Get text from response
			outputTokens = estimateOutputTokens(resp.Text)
		}

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, outputTokens)

		return resp, err
	}
}

// ApplyStream wraps streaming calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		start := time.Now()

		stream, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "stream", request.Model)

		// Estimate input tokens
		inputTokens := estimateInputTokens(request.Messages)

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, 0)

		return stream, err
	}
}

// ApplyStructured wraps structured output calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "structured", request.Model)

		// Estimate token counts
		inputTokens := estimateInputTokens(request.Messages)
		outputTokens := 0
		if resp != nil {
			// Structured output size estimation
			outputTokens = estimateStructuredOutputTokens(resp.Content)
		}

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, outputTokens)

		return resp, err
	}
}

// ApplyEmbeddings wraps embeddings calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "embeddings", request.Model)

		// Estimate input tokens
		inputTokens := 0
		for _, text := range request.Input {
			inputTokens += estimateTextTokens(text)
		}

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, 0)

		return resp, err
	}
}

// ApplyAudio wraps audio calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "audio", request.Model)

		// Audio requests typically have input
		inputTokens := 0
		if request.Type == "tts" {
			if text, ok := request.Input.(string); ok {
				inputTokens = estimateTextTokens(text)
			}
		}

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, 0)

		return resp, err
	}
}

// ApplyImage wraps image generation calls with enhanced metrics collection
func (m *TypedEnhancedMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		start := time.Now()

		resp, err := next(ctx, request)

		duration := time.Since(start)

		// Extract labels from context and request
		labels := m.extractLabels(ctx, "image", request.Model)

		// Image requests may have prompt text
		inputTokens := estimateTextTokens(request.Prompt)

		m.collector.RecordRequest(labels, duration, err, 0, inputTokens, 0)

		return resp, err
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

// estimateOutputTokens estimates tokens from output text
func estimateOutputTokens(text string) int {
	return estimateTextTokens(text)
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
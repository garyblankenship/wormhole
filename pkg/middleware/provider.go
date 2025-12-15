package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Import handler and middleware types from types package to avoid duplication

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

func (m *ProviderMetricsMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		start := time.Now()
		m.metrics.TextRequests++

		resp, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return resp, err
	}
}

func (m *ProviderMetricsMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		start := time.Now()
		m.metrics.StreamRequests++

		stream, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return stream, err
	}
}

func (m *ProviderMetricsMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		start := time.Now()
		m.metrics.StructuredRequests++

		resp, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return resp, err
	}
}

func (m *ProviderMetricsMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		start := time.Now()
		m.metrics.EmbeddingsRequests++

		resp, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return resp, err
	}
}

func (m *ProviderMetricsMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		start := time.Now()
		m.metrics.AudioRequests++

		resp, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return resp, err
	}
}

func (m *ProviderMetricsMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		start := time.Now()
		m.metrics.ImageRequests++

		resp, err := next(ctx, request)

		m.metrics.TotalLatencyMs += int64(time.Since(start).Milliseconds())
		if err != nil {
			m.metrics.TotalErrors++
		}

		return resp, err
	}
}

// ProviderLoggingMiddleware provides logging at the provider level
type ProviderLoggingMiddleware struct {
	providerName string
	logger       types.Logger
}

// NewProviderLoggingMiddleware creates middleware for provider logging
func NewProviderLoggingMiddleware(providerName string, logger types.Logger) *ProviderLoggingMiddleware {
	return &ProviderLoggingMiddleware{
		providerName: providerName,
		logger:       logger,
	}
}

// withProviderLogging wraps a handler with provider-level logging using generics
func withProviderLogging[Req any, Resp any](
	ctx context.Context,
	logger types.Logger,
	providerName string,
	requestType string,
	requestInfo string,
	getSuccessInfo func(Resp) string,
	handler func(context.Context, Req) (Resp, error),
	request Req,
) (Resp, error) {
	logger.Info(fmt.Sprintf("[%s] %s request: %s", providerName, requestType, requestInfo))

	resp, err := handler(ctx, request)

	if err != nil {
		logger.Info(fmt.Sprintf("[%s] %s request failed: %v", providerName, requestType, err))
	} else {
		logger.Info(fmt.Sprintf("[%s] %s request succeeded: %s", providerName, requestType, getSuccessInfo(resp)))
	}

	return resp, err
}

func (m *ProviderLoggingMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Text",
			fmt.Sprintf("model=%s, messages=%d", request.Model, len(request.Messages)),
			func(resp *types.TextResponse) string {
				return fmt.Sprintf("%d tokens", resp.Usage.TotalTokens)
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		m.logger.Info(fmt.Sprintf("[%s] Stream request: model=%s, messages=%d",
			m.providerName, request.Model, len(request.Messages)))

		stream, err := next(ctx, request)

		if err != nil {
			m.logger.Info(fmt.Sprintf("[%s] Stream request failed: %v", m.providerName, err))
		} else {
			m.logger.Info(fmt.Sprintf("[%s] Stream request succeeded", m.providerName))
		}

		return stream, err
	}
}

func (m *ProviderLoggingMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Structured",
			fmt.Sprintf("model=%s, schema=%s", request.Model, request.SchemaName),
			func(resp *types.StructuredResponse) string {
				return fmt.Sprintf("%d chars", len(resp.Raw))
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Embeddings",
			fmt.Sprintf("model=%s, inputs=%d", request.Model, len(request.Input)),
			func(resp *types.EmbeddingsResponse) string {
				return fmt.Sprintf("%d embeddings", len(resp.Embeddings))
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Audio",
			fmt.Sprintf("model=%s", request.Model),
			func(_ *types.AudioResponse) string { return "completed" },
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Image",
			fmt.Sprintf("model=%s, prompt=%s", request.Model, request.Prompt),
			func(resp *types.ImageResponse) string {
				return fmt.Sprintf("%d images", len(resp.Images))
			},
			next, request,
		)
	}
}

// cleanJSONResponse provides basic JSON cleaning for provider responses
func cleanJSONResponse(jsonStr string) string {
	// This is a placeholder for more sophisticated JSON cleaning
	// In the future, this could implement provider-specific cleaning strategies

	// Remove common escape sequence issues
	cleaned := strings.ReplaceAll(jsonStr, `\\\\`, `\\`)
	cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)

	// Remove leading/trailing whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

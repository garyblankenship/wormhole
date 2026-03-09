package middleware

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TypedLoggingMiddleware implements the ProviderMiddleware interface with type-safe logging
type TypedLoggingMiddleware struct {
	config LoggingConfig
}

// NewTypedLoggingMiddleware creates a new type-safe logging middleware
func NewTypedLoggingMiddleware(config LoggingConfig) *TypedLoggingMiddleware {
	return &TypedLoggingMiddleware{
		config: config,
	}
}

// NewDebugTypedLoggingMiddleware creates a debug logging middleware with default settings
func NewDebugTypedLoggingMiddleware(logger types.Logger) *TypedLoggingMiddleware {
	return NewTypedLoggingMiddleware(newDebugLoggingConfig(logger))
}

// withLogging wraps a handler with logging using generics to reduce duplication
func withLogging[Req any, Resp any](
	ctx context.Context,
	config LoggingConfig,
	requestType string,
	request Req,
	logRequest func(Req),
	logResponse func(Resp, time.Duration),
	handler func(context.Context, Req) (Resp, error),
) (Resp, error) {
	start := time.Now()

	// Log request if enabled
	if config.LogRequests && logRequest != nil {
		logRequest(request)
	}

	// Execute request
	resp, err := handler(ctx, request)
	duration := time.Since(start)

	// Log timing if enabled
	if config.LogTiming {
		config.Logger.Debug("Request completed", "request_type", requestType, "duration", duration)
	}

	// Log response if enabled (need to check for nil with type assertion)
	if config.LogResponses && logResponse != nil {
		// Use reflection to check if resp is non-nil pointer
		if !isNilResponse(resp) {
			logResponse(resp, duration)
		}
	}

	// Log error if enabled and error occurred
	if config.LogErrors && err != nil {
		logError(config, err, duration)
	}

	return resp, err
}

// isNilResponse checks if a response value is nil (handles both pointer and non-pointer types)
func isNilResponse[T any](resp T) bool {
	// For pointer types, we can use interface comparison
	var zero T
	return any(resp) == any(zero)
}

// ApplyText wraps text generation calls with logging
func (m *TypedLoggingMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withLogging(ctx, m.config, "Text", request,
			func(req types.TextRequest) { logRequestDetails(m.config, req) },
			func(resp *types.TextResponse, d time.Duration) { logResponseDetails(m.config, resp, d) },
			next,
		)
	}
}

// ApplyStream wraps streaming calls with logging
func (m *TypedLoggingMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
		start := time.Now()

		if m.config.LogRequests {
			logRequestDetails(m.config, request)
			m.config.Logger.Debug("Initiating streaming request")
		}

		stream, err := next(ctx, request)

		if m.config.LogTiming {
			m.config.Logger.Debug("Stream initiated", "duration", time.Since(start))
		}

		if m.config.LogErrors && err != nil {
			logError(m.config, err, time.Since(start))
			return stream, err
		}

		if stream != nil {
			wrappedStream := make(chan types.StreamChunk, 1)
			go func() {
				defer close(wrappedStream)
				chunkCount := 0
				for chunk := range stream {
					chunkCount++
					if m.config.LogResponses && chunkCount == 1 {
						m.config.Logger.Debug("First stream chunk received")
					}
					wrappedStream <- chunk
				}
				if m.config.LogTiming {
					m.config.Logger.Debug("Stream completed", "chunks", chunkCount, "duration", time.Since(start))
				}
			}()
			return wrappedStream, nil
		}

		return stream, err
	}
}

// ApplyStructured wraps structured output calls with logging
func (m *TypedLoggingMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withLogging(ctx, m.config, "Structured", request,
			func(req types.StructuredRequest) { logRequestDetails(m.config, req) },
			func(resp *types.StructuredResponse, d time.Duration) { logResponseDetails(m.config, resp, d) },
			next,
		)
	}
}

// ApplyEmbeddings wraps embeddings calls with logging
func (m *TypedLoggingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withLogging(ctx, m.config, "Embeddings", request,
			func(req types.EmbeddingsRequest) { logRequestDetails(m.config, req) },
			func(resp *types.EmbeddingsResponse, d time.Duration) { logResponseDetails(m.config, resp, d) },
			next,
		)
	}
}

// ApplyAudio wraps audio calls with logging
func (m *TypedLoggingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withLogging(ctx, m.config, "Audio", request,
			func(req types.AudioRequest) { logRequestDetails(m.config, req) },
			func(resp *types.AudioResponse, d time.Duration) { logResponseDetails(m.config, resp, d) },
			next,
		)
	}
}

// ApplyImage wraps image generation calls with logging
func (m *TypedLoggingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withLogging(ctx, m.config, "Image", request,
			func(req types.ImageRequest) { logRequestDetails(m.config, req) },
			func(resp *types.ImageResponse, d time.Duration) { logResponseDetails(m.config, resp, d) },
			next,
		)
	}
}

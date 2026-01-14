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
	config := LoggingConfig{
		Logger:       logger,
		LogRequests:  true,
		LogResponses: true,
		LogTiming:    true,
		LogErrors:    true,
		RedactKeys:   []string{"api_key", "apikey", "token", "authorization"},
	}
	return NewTypedLoggingMiddleware(config)
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
			m.logTextRequest,
			func(resp *types.TextResponse, d time.Duration) { m.logTextResponse(*resp, d) },
			next,
		)
	}
}

// ApplyStream wraps streaming calls with logging
func (m *TypedLoggingMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logTextRequest(request)
			m.config.Logger.Debug("Initiating streaming request")
		}

		// Execute request
		stream, err := next(ctx, request)

		// Log timing for stream initiation
		if m.config.LogTiming {
			m.config.Logger.Debug("Stream initiated", "duration", time.Since(start))
		}

		// Log error if stream creation failed
		if m.config.LogErrors && err != nil {
			logError(m.config, err, time.Since(start))
			return stream, err
		}

		// If stream is successful, wrap it with logging
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
			m.logStructuredRequest,
			func(resp *types.StructuredResponse, d time.Duration) { m.logStructuredResponse(*resp, d) },
			next,
		)
	}
}

// ApplyEmbeddings wraps embeddings calls with logging
func (m *TypedLoggingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withLogging(ctx, m.config, "Embeddings", request,
			m.logEmbeddingsRequest,
			func(resp *types.EmbeddingsResponse, d time.Duration) { m.logEmbeddingsResponse(*resp, d) },
			next,
		)
	}
}

// ApplyAudio wraps audio calls with logging
func (m *TypedLoggingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withLogging(ctx, m.config, "Audio", request,
			m.logAudioRequest,
			func(resp *types.AudioResponse, d time.Duration) { m.logAudioResponse(*resp, d) },
			next,
		)
	}
}

// ApplyImage wraps image generation calls with logging
func (m *TypedLoggingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withLogging(ctx, m.config, "Image", request,
			m.logImageRequest,
			func(resp *types.ImageResponse, d time.Duration) { m.logImageResponse(*resp, d) },
			next,
		)
	}
}

// Typed logging methods for each request type

func (m *TypedLoggingMiddleware) logTextRequest(request types.TextRequest) {
	m.config.Logger.Debug("Text request", "model", request.Model)
	if len(request.Messages) > 0 {
		m.config.Logger.Debug("Messages", "count", len(request.Messages))
		for i, msg := range request.Messages {
			m.config.Logger.Debug("Message",
				"index", i,
				"role", msg.GetRole(),
				"content", truncateString(getMessageContent(msg), 100))
		}
	}
	if request.Temperature != nil {
		m.config.Logger.Debug("Temperature", "value", *request.Temperature)
	}
	if request.MaxTokens != nil {
		m.config.Logger.Debug("Max tokens", "value", *request.MaxTokens)
	}
	if len(request.Tools) > 0 {
		m.config.Logger.Debug("Tools available", "count", len(request.Tools))
	}
}

func (m *TypedLoggingMiddleware) logTextResponse(response types.TextResponse, duration time.Duration) {
	m.config.Logger.Debug("Text response received", "duration", duration, "model", response.Model)
	m.config.Logger.Debug("Text details", "length", len(response.Text), "finish_reason", response.FinishReason)

	if response.Usage != nil {
		m.config.Logger.Debug("Token usage",
			"prompt_tokens", response.Usage.PromptTokens,
			"completion_tokens", response.Usage.CompletionTokens,
			"total_tokens", response.Usage.TotalTokens)

		// Log cost if available
		if cost, err := types.EstimateModelCost(response.Model, response.Usage.PromptTokens, response.Usage.CompletionTokens); err == nil && cost > 0 {
			m.config.Logger.Debug("Estimated cost", "cost", cost)
		}
	}

	if len(response.ToolCalls) > 0 {
		m.config.Logger.Debug("Tool calls", "count", len(response.ToolCalls))
		for i, call := range response.ToolCalls {
			m.config.Logger.Debug("Tool call", "index", i, "name", call.Name)
		}
	}

	// Log preview of response text
	preview := truncateString(response.Text, 200)
	m.config.Logger.Debug("Preview", "text", preview)
}

func (m *TypedLoggingMiddleware) logStructuredRequest(request types.StructuredRequest) {
	m.config.Logger.Debug("Structured request", "model", request.Model)
	if request.Schema != nil {
		m.config.Logger.Debug("Schema provided for structured output")
	}
}

func (m *TypedLoggingMiddleware) logStructuredResponse(response types.StructuredResponse, duration time.Duration) {
	m.config.Logger.Debug("Structured response received", "duration", duration, "model", response.Model)
	m.config.Logger.Debug("Structured data received")

	if response.Usage != nil {
		m.config.Logger.Debug("Token usage",
			"prompt_tokens", response.Usage.PromptTokens,
			"completion_tokens", response.Usage.CompletionTokens,
			"total_tokens", response.Usage.TotalTokens)
	}
}

func (m *TypedLoggingMiddleware) logEmbeddingsRequest(request types.EmbeddingsRequest) {
	m.config.Logger.Debug("Embeddings request", "model", request.Model, "input_count", len(request.Input))
}

func (m *TypedLoggingMiddleware) logEmbeddingsResponse(response types.EmbeddingsResponse, duration time.Duration) {
	m.config.Logger.Debug("Embeddings response received", "duration", duration, "model", response.Model)
	m.config.Logger.Debug("Embeddings details", "count", len(response.Embeddings))
	if len(response.Embeddings) > 0 {
		m.config.Logger.Debug("Dimensions", "value", len(response.Embeddings[0].Embedding))
	}

	if response.Usage != nil {
		m.config.Logger.Debug("Token usage", "total_tokens", response.Usage.TotalTokens)
	}
}

func (m *TypedLoggingMiddleware) logAudioRequest(request types.AudioRequest) {
	m.config.Logger.Debug("Audio request", "model", request.Model)
	switch request.Type {
	case types.AudioRequestTypeSTT:
		m.config.Logger.Debug("Type", "value", "Speech to Text")
	case types.AudioRequestTypeTTS:
		m.config.Logger.Debug("Type", "value", "Text to Speech")
		if request.Voice != "" {
			m.config.Logger.Debug("Voice", "value", request.Voice)
		}
	}
}

func (m *TypedLoggingMiddleware) logAudioResponse(response types.AudioResponse, duration time.Duration) {
	m.config.Logger.Debug("Audio response received", "duration", duration, "model", response.Model)

	if response.Text != "" {
		m.config.Logger.Debug("Text", "value", truncateString(response.Text, 100))
	}
	if len(response.Audio) > 0 {
		m.config.Logger.Debug("Audio data", "bytes", len(response.Audio))
	}
	if !response.Created.IsZero() {
		m.config.Logger.Debug("Created", "value", response.Created)
	}
}

func (m *TypedLoggingMiddleware) logImageRequest(request types.ImageRequest) {
	m.config.Logger.Debug("Image request", "model", request.Model)
	m.config.Logger.Debug("Prompt", "value", truncateString(request.Prompt, 100))
	if request.Size != "" {
		m.config.Logger.Debug("Size", "value", request.Size)
	}
	if request.Quality != "" {
		m.config.Logger.Debug("Quality", "value", request.Quality)
	}
	if request.N > 0 {
		m.config.Logger.Debug("Count", "value", request.N)
	}
}

func (m *TypedLoggingMiddleware) logImageResponse(response types.ImageResponse, duration time.Duration) {
	m.config.Logger.Debug("Image response received", "duration", duration, "model", response.Model)
	m.config.Logger.Debug("Images generated", "count", len(response.Images))

	for i, img := range response.Images {
		if img.URL != "" {
			m.config.Logger.Debug("Image", "index", i, "type", "URL provided")
		}
		if len(img.B64JSON) > 0 {
			m.config.Logger.Debug("Image", "index", i, "type", "Base64 data", "chars", len(img.B64JSON))
		}
	}
}

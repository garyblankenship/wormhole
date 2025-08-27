package middleware

import (
	"context"
	"fmt"
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

// ApplyText wraps text generation calls with logging
func (m *TypedLoggingMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logTextRequest(request)
		}

		// Execute request
		resp, err := next(ctx, request)
		duration := time.Since(start)

		// Log timing if enabled
		if m.config.LogTiming {
			m.config.Logger.Debug(fmt.Sprintf("Text request completed in %v", duration))
		}

		// Log response if enabled
		if m.config.LogResponses && resp != nil {
			m.logTextResponse(*resp, duration)
		}

		// Log error if enabled and error occurred
		if m.config.LogErrors && err != nil {
			logError(m.config, err, duration)
		}

		return resp, err
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
			m.config.Logger.Debug(fmt.Sprintf("Stream initiated in %v", time.Since(start)))
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
					m.config.Logger.Debug(fmt.Sprintf("Stream completed with %d chunks in %v", chunkCount, time.Since(start)))
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
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logStructuredRequest(request)
		}

		// Execute request
		resp, err := next(ctx, request)
		duration := time.Since(start)

		// Log timing if enabled
		if m.config.LogTiming {
			m.config.Logger.Debug(fmt.Sprintf("Structured request completed in %v", duration))
		}

		// Log response if enabled
		if m.config.LogResponses && resp != nil {
			m.logStructuredResponse(*resp, duration)
		}

		// Log error if enabled and error occurred
		if m.config.LogErrors && err != nil {
			logError(m.config, err, duration)
		}

		return resp, err
	}
}

// ApplyEmbeddings wraps embeddings calls with logging
func (m *TypedLoggingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logEmbeddingsRequest(request)
		}

		// Execute request
		resp, err := next(ctx, request)
		duration := time.Since(start)

		// Log timing if enabled
		if m.config.LogTiming {
			m.config.Logger.Debug(fmt.Sprintf("Embeddings request completed in %v", duration))
		}

		// Log response if enabled
		if m.config.LogResponses && resp != nil {
			m.logEmbeddingsResponse(*resp, duration)
		}

		// Log error if enabled and error occurred
		if m.config.LogErrors && err != nil {
			logError(m.config, err, duration)
		}

		return resp, err
	}
}

// ApplyAudio wraps audio calls with logging
func (m *TypedLoggingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logAudioRequest(request)
		}

		// Execute request
		resp, err := next(ctx, request)
		duration := time.Since(start)

		// Log timing if enabled
		if m.config.LogTiming {
			m.config.Logger.Debug(fmt.Sprintf("Audio request completed in %v", duration))
		}

		// Log response if enabled
		if m.config.LogResponses && resp != nil {
			m.logAudioResponse(*resp, duration)
		}

		// Log error if enabled and error occurred
		if m.config.LogErrors && err != nil {
			logError(m.config, err, duration)
		}

		return resp, err
	}
}

// ApplyImage wraps image generation calls with logging
func (m *TypedLoggingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		start := time.Now()

		// Log request if enabled
		if m.config.LogRequests {
			m.logImageRequest(request)
		}

		// Execute request
		resp, err := next(ctx, request)
		duration := time.Since(start)

		// Log timing if enabled
		if m.config.LogTiming {
			m.config.Logger.Debug(fmt.Sprintf("Image request completed in %v", duration))
		}

		// Log response if enabled
		if m.config.LogResponses && resp != nil {
			m.logImageResponse(*resp, duration)
		}

		// Log error if enabled and error occurred
		if m.config.LogErrors && err != nil {
			logError(m.config, err, duration)
		}

		return resp, err
	}
}

// Typed logging methods for each request type

func (m *TypedLoggingMiddleware) logTextRequest(request types.TextRequest) {
	m.config.Logger.Debug(fmt.Sprintf("Text request to model %s:", request.Model))
	if len(request.Messages) > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Messages: %d", len(request.Messages)))
		for i, msg := range request.Messages {
			m.config.Logger.Debug(fmt.Sprintf("    [%d] %s: %s", i, msg.GetRole(), truncateString(getMessageContent(msg), 100)))
		}
	}
	if request.Temperature != nil {
		m.config.Logger.Debug(fmt.Sprintf("  Temperature: %.2f", *request.Temperature))
	}
	if request.MaxTokens != nil {
		m.config.Logger.Debug(fmt.Sprintf("  Max tokens: %d", *request.MaxTokens))
	}
	if len(request.Tools) > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Tools: %d available", len(request.Tools)))
	}
}

func (m *TypedLoggingMiddleware) logTextResponse(response types.TextResponse, duration time.Duration) {
	m.config.Logger.Debug(fmt.Sprintf("Text response received in %v:", duration))
	m.config.Logger.Debug(fmt.Sprintf("  Model: %s", response.Model))
	m.config.Logger.Debug(fmt.Sprintf("  Text length: %d chars", len(response.Text)))
	m.config.Logger.Debug(fmt.Sprintf("  Finish reason: %s", response.FinishReason))

	if response.Usage != nil {
		m.config.Logger.Debug(fmt.Sprintf("  Usage: %d input + %d output = %d total tokens",
			response.Usage.PromptTokens, response.Usage.CompletionTokens, response.Usage.TotalTokens))

		// Log cost if available
		if cost, err := types.EstimateModelCost(response.Model, response.Usage.PromptTokens, response.Usage.CompletionTokens); err == nil && cost > 0 {
			m.config.Logger.Debug(fmt.Sprintf("  Estimated cost: $%.4f", cost))
		}
	}

	if len(response.ToolCalls) > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Tool calls: %d", len(response.ToolCalls)))
		for i, call := range response.ToolCalls {
			m.config.Logger.Debug(fmt.Sprintf("    [%d] %s", i, call.Name))
		}
	}

	// Log preview of response text
	preview := truncateString(response.Text, 200)
	m.config.Logger.Debug(fmt.Sprintf("  Preview: %s", preview))
}

func (m *TypedLoggingMiddleware) logStructuredRequest(request types.StructuredRequest) {
	m.config.Logger.Debug(fmt.Sprintf("Structured request to model %s", request.Model))
	if request.Schema != nil {
		m.config.Logger.Debug("  Schema provided for structured output")
	}
}

func (m *TypedLoggingMiddleware) logStructuredResponse(response types.StructuredResponse, duration time.Duration) {
	m.config.Logger.Debug(fmt.Sprintf("Structured response received in %v:", duration))
	m.config.Logger.Debug(fmt.Sprintf("  Model: %s", response.Model))
	m.config.Logger.Debug("  Structured data received")

	if response.Usage != nil {
		m.config.Logger.Debug(fmt.Sprintf("  Usage: %d input + %d output = %d total tokens",
			response.Usage.PromptTokens, response.Usage.CompletionTokens, response.Usage.TotalTokens))
	}
}

func (m *TypedLoggingMiddleware) logEmbeddingsRequest(request types.EmbeddingsRequest) {
	m.config.Logger.Debug(fmt.Sprintf("Embeddings request to model %s", request.Model))
	m.config.Logger.Debug(fmt.Sprintf("  Input count: %d", len(request.Input)))
}

func (m *TypedLoggingMiddleware) logEmbeddingsResponse(response types.EmbeddingsResponse, duration time.Duration) {
	m.config.Logger.Debug(fmt.Sprintf("Embeddings response received in %v:", duration))
	m.config.Logger.Debug(fmt.Sprintf("  Model: %s", response.Model))
	m.config.Logger.Debug(fmt.Sprintf("  Embeddings count: %d", len(response.Embeddings)))
	if len(response.Embeddings) > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Dimensions: %d", len(response.Embeddings[0].Embedding)))
	}

	if response.Usage != nil {
		m.config.Logger.Debug(fmt.Sprintf("  Usage: %d tokens", response.Usage.TotalTokens))
	}
}

func (m *TypedLoggingMiddleware) logAudioRequest(request types.AudioRequest) {
	m.config.Logger.Debug(fmt.Sprintf("Audio request to model %s", request.Model))
	switch request.Type {
	case types.AudioRequestTypeSTT:
		m.config.Logger.Debug("  Type: Speech to Text")
	case types.AudioRequestTypeTTS:
		m.config.Logger.Debug("  Type: Text to Speech")
		if request.Voice != "" {
			m.config.Logger.Debug(fmt.Sprintf("  Voice: %s", request.Voice))
		}
	}
}

func (m *TypedLoggingMiddleware) logAudioResponse(response types.AudioResponse, duration time.Duration) {
	m.config.Logger.Debug(fmt.Sprintf("Audio response received in %v:", duration))
	m.config.Logger.Debug(fmt.Sprintf("  Model: %s", response.Model))
	
	if response.Text != "" {
		m.config.Logger.Debug(fmt.Sprintf("  Text: %s", truncateString(response.Text, 100)))
	}
	if len(response.Audio) > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Audio data: %d bytes", len(response.Audio)))
	}
	if !response.Created.IsZero() {
		m.config.Logger.Debug(fmt.Sprintf("  Created: %v", response.Created))
	}
}

func (m *TypedLoggingMiddleware) logImageRequest(request types.ImageRequest) {
	m.config.Logger.Debug(fmt.Sprintf("Image request to model %s", request.Model))
	m.config.Logger.Debug(fmt.Sprintf("  Prompt: %s", truncateString(request.Prompt, 100)))
	if request.Size != "" {
		m.config.Logger.Debug(fmt.Sprintf("  Size: %s", request.Size))
	}
	if request.Quality != "" {
		m.config.Logger.Debug(fmt.Sprintf("  Quality: %s", request.Quality))
	}
	if request.N > 0 {
		m.config.Logger.Debug(fmt.Sprintf("  Count: %d", request.N))
	}
}

func (m *TypedLoggingMiddleware) logImageResponse(response types.ImageResponse, duration time.Duration) {
	m.config.Logger.Debug(fmt.Sprintf("Image response received in %v:", duration))
	m.config.Logger.Debug(fmt.Sprintf("  Model: %s", response.Model))
	m.config.Logger.Debug(fmt.Sprintf("  Images generated: %d", len(response.Images)))
	
	for i, img := range response.Images {
		if img.URL != "" {
			m.config.Logger.Debug(fmt.Sprintf("    [%d] URL provided", i))
		}
		if len(img.B64JSON) > 0 {
			m.config.Logger.Debug(fmt.Sprintf("    [%d] Base64 data: %d chars", i, len(img.B64JSON)))
		}
	}
}
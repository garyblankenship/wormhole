package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// LoggingConfig configures the logging middleware
type LoggingConfig struct {
	Logger       types.Logger
	LogRequests  bool
	LogResponses bool
	LogTiming    bool
	LogErrors    bool
	RedactKeys   []string // Keys to redact from logs (like API keys)
}

// DefaultLoggingConfig returns sensible defaults
func DefaultLoggingConfig(logger types.Logger) LoggingConfig {
	return LoggingConfig{
		Logger:       logger,
		LogRequests:  true,
		LogResponses: true,
		LogTiming:    true,
		LogErrors:    true,
		RedactKeys:   []string{"api_key", "apikey", "token", "authorization"},
	}
}

// DetailedLoggingMiddleware creates request/response logging middleware with configuration
func DetailedLoggingMiddleware(config LoggingConfig) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			// Log request if enabled
			if config.LogRequests {
				logRequest(config, req)
			}

			// Execute request
			resp, err := next(ctx, req)
			duration := time.Since(start)

			// Log timing if enabled
			if config.LogTiming {
				config.Logger.Debug("Request completed", "duration", duration)
			}

			// Log response if enabled
			if config.LogResponses && resp != nil {
				logResponse(config, resp, duration)
			}

			// Log error if enabled and error occurred
			if config.LogErrors && err != nil {
				logError(config, err, duration)
			}

			return resp, err
		}
	}
}

// DebugLoggingMiddleware creates verbose debug logging
func DebugLoggingMiddleware(logger types.Logger) Middleware {
	config := LoggingConfig{
		Logger:       logger,
		LogRequests:  true,
		LogResponses: true,
		LogTiming:    true,
		LogErrors:    true,
		RedactKeys:   []string{"api_key", "apikey", "token", "authorization"},
	}

	return DetailedLoggingMiddleware(config)
}

// logRequest logs the outgoing request
func logRequest(config LoggingConfig, req any) {
	sanitized := redactSensitiveData(req, config.RedactKeys)

	switch r := req.(type) {
	case *types.TextRequest:
		config.Logger.Debug("Text request", "model", r.Model)
		if len(r.Messages) > 0 {
			config.Logger.Debug("Messages", "count", len(r.Messages))
			for i, msg := range r.Messages {
				config.Logger.Debug("Message",
					"index", i,
					"role", msg.GetRole(),
					"content", truncateString(getMessageContent(msg), 100))
			}
		}
		if r.Temperature != nil {
			config.Logger.Debug("Temperature", "value", *r.Temperature)
		}
		if r.MaxTokens != nil {
			config.Logger.Debug("Max tokens", "value", *r.MaxTokens)
		}
		if len(r.Tools) > 0 {
			config.Logger.Debug("Tools available", "count", len(r.Tools))
		}

	case *types.StructuredRequest:
		config.Logger.Debug("Structured request", "model", r.Model)
		if r.Schema != nil {
			config.Logger.Debug("Schema provided for structured output")
		}

	case *types.EmbeddingsRequest:
		config.Logger.Debug("Embeddings request", "model", r.Model, "input_count", len(r.Input))

	default:
		// Generic logging for unknown request types
		if jsonData, err := json.MarshalIndent(sanitized, "", "  "); err == nil {
			config.Logger.Debug("Request", "data", string(jsonData))
		}
	}
}

// logResponse logs the response details
func logResponse(config LoggingConfig, resp any, duration time.Duration) {
	switch r := resp.(type) {
	case *types.TextResponse:
		config.Logger.Debug("Text response received", "duration", duration, "model", r.Model)
		config.Logger.Debug("Text details", "length", len(r.Text), "finish_reason", r.FinishReason)

		if r.Usage != nil {
			config.Logger.Debug("Token usage",
				"prompt_tokens", r.Usage.PromptTokens,
				"completion_tokens", r.Usage.CompletionTokens,
				"total_tokens", r.Usage.TotalTokens)

			// Log cost if available
			if cost, err := types.EstimateModelCost(r.Model, r.Usage.PromptTokens, r.Usage.CompletionTokens); err == nil && cost > 0 {
				config.Logger.Debug("Estimated cost", "cost", cost)
			}
		}

		if len(r.ToolCalls) > 0 {
			config.Logger.Debug("Tool calls", "count", len(r.ToolCalls))
			for i, call := range r.ToolCalls {
				config.Logger.Debug("Tool call", "index", i, "name", call.Name)
			}
		}

		// Log preview of response text
		preview := truncateString(r.Text, 200)
		config.Logger.Debug("Preview", "text", preview)

	case *types.StructuredResponse:
		config.Logger.Debug("Structured response received", "duration", duration, "model", r.Model)
		config.Logger.Debug("Structured data received")

		if r.Usage != nil {
			config.Logger.Debug("Token usage",
				"prompt_tokens", r.Usage.PromptTokens,
				"completion_tokens", r.Usage.CompletionTokens,
				"total_tokens", r.Usage.TotalTokens)
		}

	case *types.EmbeddingsResponse:
		config.Logger.Debug("Embeddings response received", "duration", duration, "model", r.Model)
		config.Logger.Debug("Embeddings details", "count", len(r.Embeddings))
		if len(r.Embeddings) > 0 {
			config.Logger.Debug("Dimensions", "value", len(r.Embeddings[0].Embedding))
		}

		if r.Usage != nil {
			config.Logger.Debug("Token usage", "total_tokens", r.Usage.TotalTokens)
		}

	default:
		config.Logger.Debug("Response received", "duration", duration)
	}
}

// logError logs error details
func logError(config LoggingConfig, err error, duration time.Duration) {
	if wormholeErr, ok := types.AsWormholeError(err); ok {
		config.Logger.Error("Request failed",
			"duration", duration,
			"code", wormholeErr.Code,
			"message", wormholeErr.Message)

		if wormholeErr.Details != "" {
			config.Logger.Error("Error details", "details", wormholeErr.Details)
		}
		if wormholeErr.Provider != "" {
			config.Logger.Error("Provider", "provider", wormholeErr.Provider)
		}
		if wormholeErr.Model != "" {
			config.Logger.Error("Model", "model", wormholeErr.Model)
		}
		if wormholeErr.StatusCode > 0 {
			config.Logger.Error("HTTP Status", "status_code", wormholeErr.StatusCode)
		}
		config.Logger.Error("Retryable", "retryable", wormholeErr.Retryable)
	} else {
		config.Logger.Error("Request failed", "duration", duration, "error", err)
	}
}

// redactSensitiveData removes sensitive information from data before logging
func redactSensitiveData(data any, redactKeys []string) any {
	// Convert to map for processing
	jsonData, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var mapData map[string]any
	if err := json.Unmarshal(jsonData, &mapData); err != nil {
		return data
	}

	// Redact sensitive keys
	for _, key := range redactKeys {
		if _, exists := mapData[key]; exists {
			mapData[key] = "[REDACTED]"
		}
	}

	return mapData
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getMessageContent extracts content from different message types
func getMessageContent(msg types.Message) string {
	switch m := msg.(type) {
	case *types.UserMessage:
		return m.Content
	case *types.SystemMessage:
		return m.Content
	case *types.AssistantMessage:
		return m.Content
	default:
		return fmt.Sprintf("%T", msg)
	}
}

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
				config.Logger.Debug(fmt.Sprintf("Request completed in %v", duration))
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
		config.Logger.Debug(fmt.Sprintf("Text request to model %s:", r.Model))
		if len(r.Messages) > 0 {
			config.Logger.Debug(fmt.Sprintf("  Messages: %d", len(r.Messages)))
			for i, msg := range r.Messages {
				config.Logger.Debug(fmt.Sprintf("    [%d] %s: %s", i, msg.GetRole(), truncateString(getMessageContent(msg), 100)))
			}
		}
		if r.Temperature != nil {
			config.Logger.Debug(fmt.Sprintf("  Temperature: %.2f", *r.Temperature))
		}
		if r.MaxTokens != nil {
			config.Logger.Debug(fmt.Sprintf("  Max tokens: %d", *r.MaxTokens))
		}
		if len(r.Tools) > 0 {
			config.Logger.Debug(fmt.Sprintf("  Tools: %d available", len(r.Tools)))
		}

	case *types.StructuredRequest:
		config.Logger.Debug(fmt.Sprintf("Structured request to model %s", r.Model))
		if r.Schema != nil {
			config.Logger.Debug("  Schema provided for structured output")
		}

	case *types.EmbeddingsRequest:
		config.Logger.Debug(fmt.Sprintf("Embeddings request to model %s", r.Model))
		config.Logger.Debug(fmt.Sprintf("  Input count: %d", len(r.Input)))

	default:
		// Generic logging for unknown request types
		if jsonData, err := json.MarshalIndent(sanitized, "", "  "); err == nil {
			config.Logger.Debug(fmt.Sprintf("Request: %s", jsonData))
		}
	}
}

// logResponse logs the response details
func logResponse(config LoggingConfig, resp any, duration time.Duration) {
	switch r := resp.(type) {
	case *types.TextResponse:
		config.Logger.Debug(fmt.Sprintf("Text response received in %v:", duration))
		config.Logger.Debug(fmt.Sprintf("  Model: %s", r.Model))
		config.Logger.Debug(fmt.Sprintf("  Text length: %d chars", len(r.Text)))
		config.Logger.Debug(fmt.Sprintf("  Finish reason: %s", r.FinishReason))

		if r.Usage != nil {
			config.Logger.Debug(fmt.Sprintf("  Usage: %d input + %d output = %d total tokens",
				r.Usage.PromptTokens, r.Usage.CompletionTokens, r.Usage.TotalTokens))

			// Log cost if available
			if cost, err := types.EstimateModelCost(r.Model, r.Usage.PromptTokens, r.Usage.CompletionTokens); err == nil && cost > 0 {
				config.Logger.Debug(fmt.Sprintf("  Estimated cost: $%.4f", cost))
			}
		}

		if len(r.ToolCalls) > 0 {
			config.Logger.Debug(fmt.Sprintf("  Tool calls: %d", len(r.ToolCalls)))
			for i, call := range r.ToolCalls {
				config.Logger.Debug(fmt.Sprintf("    [%d] %s", i, call.Name))
			}
		}

		// Log preview of response text
		preview := truncateString(r.Text, 200)
		config.Logger.Debug(fmt.Sprintf("  Preview: %s", preview))

	case *types.StructuredResponse:
		config.Logger.Debug(fmt.Sprintf("Structured response received in %v:", duration))
		config.Logger.Debug(fmt.Sprintf("  Model: %s", r.Model))
		config.Logger.Debug("  Structured data received")

		if r.Usage != nil {
			config.Logger.Debug(fmt.Sprintf("  Usage: %d input + %d output = %d total tokens",
				r.Usage.PromptTokens, r.Usage.CompletionTokens, r.Usage.TotalTokens))
		}

	case *types.EmbeddingsResponse:
		config.Logger.Debug(fmt.Sprintf("Embeddings response received in %v:", duration))
		config.Logger.Debug(fmt.Sprintf("  Model: %s", r.Model))
		config.Logger.Debug(fmt.Sprintf("  Embeddings count: %d", len(r.Embeddings)))
		if len(r.Embeddings) > 0 {
			config.Logger.Debug(fmt.Sprintf("  Dimensions: %d", len(r.Embeddings[0].Embedding)))
		}

		if r.Usage != nil {
			config.Logger.Debug(fmt.Sprintf("  Usage: %d tokens", r.Usage.TotalTokens))
		}

	default:
		config.Logger.Debug(fmt.Sprintf("Response received in %v", duration))
	}
}

// logError logs error details
func logError(config LoggingConfig, err error, duration time.Duration) {
	if wormholeErr, ok := types.AsWormholeError(err); ok {
		config.Logger.Error(fmt.Sprintf("Request failed in %v - %s: %s",
			duration, wormholeErr.Code, wormholeErr.Message))

		if wormholeErr.Details != "" {
			config.Logger.Error(fmt.Sprintf("  Details: %s", wormholeErr.Details))
		}
		if wormholeErr.Provider != "" {
			config.Logger.Error(fmt.Sprintf("  Provider: %s", wormholeErr.Provider))
		}
		if wormholeErr.Model != "" {
			config.Logger.Error(fmt.Sprintf("  Model: %s", wormholeErr.Model))
		}
		if wormholeErr.StatusCode > 0 {
			config.Logger.Error(fmt.Sprintf("  HTTP Status: %d", wormholeErr.StatusCode))
		}
		config.Logger.Error(fmt.Sprintf("  Retryable: %t", wormholeErr.Retryable))
	} else {
		config.Logger.Error(fmt.Sprintf("Request failed in %v: %v", duration, err))
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

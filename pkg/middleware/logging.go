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
	return newDebugLoggingConfig(logger)
}

// DetailedLoggingMiddleware creates request/response logging middleware with configuration
func DetailedLoggingMiddleware(config LoggingConfig) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			if config.LogRequests {
				logRequestDetails(config, req)
			}

			resp, err := next(ctx, req)
			duration := time.Since(start)

			if config.LogTiming {
				config.Logger.Debug("Request completed", "duration", duration)
			}

			if config.LogResponses && resp != nil {
				logResponseDetails(config, resp, duration)
			}

			if config.LogErrors && err != nil {
				logError(config, err, duration)
			}

			return resp, err
		}
	}
}

// DebugLoggingMiddleware creates verbose debug logging
func DebugLoggingMiddleware(logger types.Logger) Middleware {
	return DetailedLoggingMiddleware(newDebugLoggingConfig(logger))
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

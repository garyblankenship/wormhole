package middleware

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
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
	config = normalizeLoggingConfig(config)
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
				logError(ctx, config, err, duration)
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
func logError(ctx context.Context, config LoggingConfig, err error, duration time.Duration) {
	args := make([]any, 0, 9)
	args = append(args, "duration", duration)
	for _, attr := range types.SafeErrorAttrs(err) {
		args = append(args, attr)
	}
	args = append(args, requestMetadataAttrs(ctx)...)
	config.Logger.Error("Request failed", args...)
}

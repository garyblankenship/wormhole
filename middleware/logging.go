package middleware

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
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

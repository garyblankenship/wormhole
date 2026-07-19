package types

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

// Error types for better debugging and error handling
// ErrorCode represents different types of errors
type ErrorCode string

const (
	ErrorCodeAuth       ErrorCode = "AUTH_ERROR"
	ErrorCodeModel      ErrorCode = "MODEL_ERROR"
	ErrorCodeRateLimit  ErrorCode = "RATE_LIMIT_ERROR"
	ErrorCodeRequest    ErrorCode = "REQUEST_ERROR"
	ErrorCodeTimeout    ErrorCode = "TIMEOUT_ERROR"
	ErrorCodeProvider   ErrorCode = "PROVIDER_ERROR"
	ErrorCodeNetwork    ErrorCode = "NETWORK_ERROR"
	ErrorCodeValidation ErrorCode = "VALIDATION_ERROR"
	ErrorCodeMiddleware ErrorCode = "MIDDLEWARE_ERROR"
	ErrorCodeUnknown    ErrorCode = "UNKNOWN_ERROR"
)

var (
	// Authentication errors
	ErrInvalidAPIKey = NewWormholeError(ErrorCodeAuth, "invalid API key", false)
	ErrMissingAPIKey = NewWormholeError(ErrorCodeAuth, "API key is required", false)

	// Model errors
	ErrModelNotFound     = NewWormholeError(ErrorCodeModel, "model not available", false)
	ErrModelNotSupported = NewWormholeError(ErrorCodeModel, "model not supported by provider", false)
	ErrInvalidModel      = NewWormholeError(ErrorCodeModel, "invalid model name", false)

	// Rate limiting errors
	ErrRateLimited   = NewWormholeError(ErrorCodeRateLimit, "rate limit exceeded", true)
	ErrQuotaExceeded = NewWormholeError(ErrorCodeRateLimit, "quota exceeded", false)

	// Request errors
	ErrInvalidRequest  = NewWormholeError(ErrorCodeRequest, "invalid request parameters", false)
	ErrRequestTooLarge = NewWormholeError(ErrorCodeRequest, "request payload too large", false)
	ErrTimeout         = NewWormholeError(ErrorCodeTimeout, "request timeout", true)

	// Provider errors
	ErrProviderNotFound        = NewWormholeError(ErrorCodeProvider, "provider not configured", false)
	ErrProviderUnavailable     = NewWormholeError(ErrorCodeProvider, "provider service unavailable", true)
	ErrProviderConstraintError = NewWormholeError(ErrorCodeProvider, "provider constraint violation", false)

	// Network errors
	ErrNetworkError       = NewWormholeError(ErrorCodeNetwork, "network connection failed", true)
	ErrServiceUnavailable = NewWormholeError(ErrorCodeNetwork, "service temporarily unavailable", true)

	// Validation errors
	ErrValidation = NewWormholeError(ErrorCodeValidation, "validation failed", false)

	// Middleware errors
	ErrCircuitOpen        = NewWormholeError(ErrorCodeMiddleware, "circuit breaker is open", true)
	ErrRateLimitExceeded  = NewWormholeError(ErrorCodeMiddleware, "rate limit exceeded", true)
	ErrNoHealthyProviders = NewWormholeError(ErrorCodeMiddleware, "no healthy providers available", true)
)

// WormholeError provides structured error information
type WormholeError struct {
	Code       ErrorCode     `json:"code"`
	Message    string        `json:"message"`
	Retryable  bool          `json:"retryable"`
	StatusCode int           `json:"status_code,omitempty"`
	Provider   string        `json:"provider,omitempty"`
	Model      string        `json:"model,omitempty"`
	Details    string        `json:"details,omitempty"`
	Cause      error         `json:"-"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

const maxSafeErrorFieldLength = 512

// Error implements the error interface
func (e *WormholeError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// LogValue provides a bounded, structured representation for logs. Details and
// Cause intentionally remain available to SDK callers but are never emitted.
func (e *WormholeError) LogValue() slog.Value {
	if e == nil {
		return slog.GroupValue(slog.String("error_type", "*types.WormholeError"))
	}
	return slog.GroupValue(SafeErrorAttrs(e)...)
}

// SafeErrorAttrs returns the non-sensitive fields that may be written to logs.
// Arbitrary errors are represented only by their concrete type because their
// messages can contain upstream response bodies, credentials, or request data.
func SafeErrorAttrs(err error) []slog.Attr {
	if wormholeErr, ok := AsWormholeError(err); ok {
		attrs := []slog.Attr{
			slog.String("code", string(wormholeErr.Code)),
			slog.String("message", safeErrorMessage(wormholeErr.Code)),
			slog.Bool("retryable", wormholeErr.Retryable),
		}
		if wormholeErr.Provider != "" {
			attrs = append(attrs, slog.String("provider", SafeLogString(wormholeErr.Provider)))
		}
		if wormholeErr.Model != "" {
			attrs = append(attrs, slog.String("model", SafeLogString(wormholeErr.Model)))
		}
		if wormholeErr.StatusCode > 0 {
			attrs = append(attrs, slog.Int("status_code", wormholeErr.StatusCode))
		}
		return attrs
	}

	errorType := "<nil>"
	if err != nil {
		errorType = fmt.Sprintf("%T", err)
	}
	return []slog.Attr{slog.String("error_type", boundedLogField(errorType))}
}

// safeErrorMessage deliberately derives log prose from the error class. The
// caller-visible Message may contain provider-controlled response text.
func safeErrorMessage(code ErrorCode) string {
	switch code {
	case ErrorCodeAuth:
		return "authentication failed"
	case ErrorCodeModel:
		return "model request failed"
	case ErrorCodeRateLimit:
		return "rate limit exceeded"
	case ErrorCodeRequest:
		return "invalid request"
	case ErrorCodeTimeout:
		return "request timeout"
	case ErrorCodeProvider:
		return "provider request failed"
	case ErrorCodeNetwork:
		return "network request failed"
	case ErrorCodeValidation:
		return "validation failed"
	case ErrorCodeMiddleware:
		return "middleware request failed"
	default:
		return "request failed"
	}
}

// SafeLogString bounds log metadata and strips credentials and query strings
// from URL-shaped values.
func SafeLogString(value string) string {
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.ForceQuery = false
			parsed.Fragment = ""
			value = parsed.String()
		}
	}
	return boundedLogField(value)
}

// SafeErrorValue returns a structured slog value that cannot format the raw
// error or its cause chain.
func SafeErrorValue(err error) slog.Value {
	return slog.GroupValue(SafeErrorAttrs(err)...)
}

func boundedLogField(value string) string {
	if len(value) <= maxSafeErrorFieldLength {
		return value
	}
	return value[:maxSafeErrorFieldLength]
}

// Unwrap returns the underlying error
func (e *WormholeError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether this error should be retried
func (e *WormholeError) IsRetryable() bool {
	return e.Retryable
}

// WithProvider adds provider context to the error
func (e *WormholeError) WithProvider(provider string) *WormholeError {
	newErr := *e
	newErr.Provider = provider
	return &newErr
}

// WithModel adds model context to the error
func (e *WormholeError) WithModel(model string) *WormholeError {
	newErr := *e
	newErr.Model = model
	return &newErr
}

// WithDetails adds additional details to the error
func (e *WormholeError) WithDetails(details string) *WormholeError {
	newErr := *e
	newErr.Details = details
	return &newErr
}

// WithStatusCode adds HTTP status code to the error
func (e *WormholeError) WithStatusCode(code int) *WormholeError {
	newErr := *e
	newErr.StatusCode = code
	return &newErr
}

// WithCause adds the underlying cause
func (e *WormholeError) WithCause(cause error) *WormholeError {
	newErr := *e
	newErr.Cause = cause
	return &newErr
}

// WithRetryAfter sets a normalized provider-supplied retry delay on the error.
func (e *WormholeError) WithRetryAfter(d time.Duration) *WormholeError {
	newErr := *e
	newErr.RetryAfter = d
	return &newErr
}

// WithOperation adds operation context to the error, prepending to Details.
// This helps identify WHERE the error occurred in the call chain.
//
// Example:
//
//	return types.ErrInvalidRequest.
//	    WithOperation("TextRequestBuilder.Generate").
//	    WithDetails("no messages provided")
//
// Results in: "REQUEST_ERROR: invalid request parameters (TextRequestBuilder.Generate: no messages provided)"
func (e *WormholeError) WithOperation(operation string) *WormholeError {
	newErr := *e
	if newErr.Details != "" {
		newErr.Details = operation + ": " + newErr.Details
	} else {
		newErr.Details = operation
	}
	return &newErr
}

// NewWormholeError creates a new WormholeError
func NewWormholeError(code ErrorCode, message string, retryable bool) *WormholeError {
	return &WormholeError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}

// WrapError wraps an existing error with Wormhole error context
func WrapError(code ErrorCode, message string, retryable bool, cause error) *WormholeError {
	return &WormholeError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Cause:     cause,
	}
}

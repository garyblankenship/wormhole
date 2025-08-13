package types

import (
	"fmt"
	"net/http"
)

// Error types for better debugging and error handling
var (
	// Authentication errors
	ErrInvalidAPIKey = NewWormholeError(ErrorCodeAuth, "invalid API key", true)
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
)

// ErrorCode represents different types of errors
type ErrorCode string

const (
	ErrorCodeAuth      ErrorCode = "AUTH_ERROR"
	ErrorCodeModel     ErrorCode = "MODEL_ERROR"
	ErrorCodeRateLimit ErrorCode = "RATE_LIMIT_ERROR"
	ErrorCodeRequest   ErrorCode = "REQUEST_ERROR"
	ErrorCodeTimeout   ErrorCode = "TIMEOUT_ERROR"
	ErrorCodeProvider  ErrorCode = "PROVIDER_ERROR"
	ErrorCodeNetwork   ErrorCode = "NETWORK_ERROR"
	ErrorCodeUnknown   ErrorCode = "UNKNOWN_ERROR"
)

// WormholeError provides structured error information
type WormholeError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Retryable  bool      `json:"retryable"`
	StatusCode int       `json:"status_code,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	Details    string    `json:"details,omitempty"`
	Cause      error     `json:"-"`
}

// Error implements the error interface
func (e *WormholeError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
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

// HTTPStatusToError converts HTTP status codes to appropriate WormholeErrors
func HTTPStatusToError(statusCode int, body string) *WormholeError {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrInvalidAPIKey.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusForbidden:
		return ErrQuotaExceeded.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusNotFound:
		return ErrModelNotFound.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusTooManyRequests:
		return ErrRateLimited.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusBadRequest:
		return ErrInvalidRequest.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusRequestEntityTooLarge:
		return ErrRequestTooLarge.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusRequestTimeout:
		return ErrTimeout.WithStatusCode(statusCode).WithDetails(body)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrServiceUnavailable.WithStatusCode(statusCode).WithDetails(body)
	default:
		return NewWormholeError(ErrorCodeUnknown, "unknown error", false).
			WithStatusCode(statusCode).
			WithDetails(body)
	}
}

// IsWormholeError checks if an error is a WormholeError
func IsWormholeError(err error) bool {
	_, ok := err.(*WormholeError)
	return ok
}

// AsWormholeError extracts a WormholeError from an error
func AsWormholeError(err error) (*WormholeError, bool) {
	wormholeErr, ok := err.(*WormholeError)
	return wormholeErr, ok
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.IsRetryable()
	}
	return false
}

// ModelConstraintError represents a model-specific constraint violation
type ModelConstraintError struct {
	*WormholeError
	Constraint string      `json:"constraint"`
	Expected   interface{} `json:"expected"`
	Actual     interface{} `json:"actual"`
}

// NewModelConstraintError creates a new model constraint error
func NewModelConstraintError(model, constraint string, expected, actual interface{}) *ModelConstraintError {
	baseErr := ErrProviderConstraintError.
		WithModel(model).
		WithDetails(fmt.Sprintf("constraint '%s' violated: expected %v, got %v", constraint, expected, actual))

	return &ModelConstraintError{
		WormholeError: baseErr,
		Constraint:    constraint,
		Expected:      expected,
		Actual:        actual,
	}
}

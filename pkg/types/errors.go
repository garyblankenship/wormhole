package types

import (
	"errors"
	"fmt"
	"net/http"
	"time"
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

	// Validation errors
	ErrValidation = NewWormholeError(ErrorCodeValidation, "validation failed", false)
)

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
	ErrorCodeUnknown    ErrorCode = "UNKNOWN_ERROR"
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

// ==================== Error Type Checking Helpers ====================

// IsAuthError checks if an error is authentication-related (invalid key, missing key).
// Use this to detect when credentials need to be checked or refreshed.
//
// Example:
//
//	if types.IsAuthError(err) {
//	    return fmt.Errorf("check your API key: %w", err)
//	}
func IsAuthError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeAuth
	}
	return false
}

// IsRateLimitError checks if an error is due to rate limiting.
// When true, you should back off and retry after a delay.
//
// Example:
//
//	if types.IsRateLimitError(err) {
//	    time.Sleep(types.GetRetryAfter(err))
//	    // retry request
//	}
func IsRateLimitError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeRateLimit
	}
	return false
}

// IsModelError checks if an error is model-related (not found, not supported).
// Use this to detect invalid model names or provider capability mismatches.
func IsModelError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeModel
	}
	return false
}

// IsProviderConfigError checks if an error is provider configuration-related
// (not configured, constraint violation). For general provider errors, use IsProviderError.
func IsProviderConfigError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeProvider
	}
	return false
}

// IsNetworkError checks if an error is network-related (connection failed, service unavailable).
// Network errors are typically retryable after a delay.
func IsNetworkError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeNetwork
	}
	return false
}

// IsTimeoutError checks if an error is a timeout.
func IsTimeoutError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeTimeout
	}
	return false
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeValidation
	}
	return false
}

// GetRetryAfter returns a suggested retry delay for retryable errors.
// Returns 0 if the error is not retryable or has no retry hint.
//
// The delay is based on the error type:
//   - Rate limit errors: 30 seconds (provider-specific may vary)
//   - Network errors: 5 seconds
//   - Timeout errors: 10 seconds
//   - Other retryable: 1 second
func GetRetryAfter(err error) time.Duration {
	wormholeErr, ok := AsWormholeError(err)
	if !ok || !wormholeErr.IsRetryable() {
		return 0
	}

	switch wormholeErr.Code {
	case ErrorCodeRateLimit:
		return 30 * time.Second
	case ErrorCodeNetwork:
		return 5 * time.Second
	case ErrorCodeTimeout:
		return 10 * time.Second
	default:
		return 1 * time.Second
	}
}

// Errorf creates a wrapped error with formatted message
// Usage: types.Errorf("marshal request body", err)
// Result: "failed to marshal request body: <original error>"
func Errorf(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", operation, err)
}

// Errorff creates a wrapped error with formatted operation string
// Usage: types.Errorff("read %s", err, filename)
// Result: "failed to read <filename>: <original error>"
func Errorff(format string, err error, args ...any) error {
	if err == nil {
		return nil
	}
	operation := fmt.Sprintf(format, args...)
	return fmt.Errorf("failed to %s: %w", operation, err)
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

// IsWormholeError checks if an error is a WormholeError or contains one
func IsWormholeError(err error) bool {
	// Direct WormholeError
	if _, ok := err.(*WormholeError); ok {
		return true
	}

	// ModelConstraintError embeds WormholeError
	if _, ok := err.(*ModelConstraintError); ok {
		return true
	}

	return false
}

// AsWormholeError extracts a WormholeError from an error
func AsWormholeError(err error) (*WormholeError, bool) {
	// Use errors.As to properly unwrap error chains
	var wormholeErr *WormholeError
	if errors.As(err, &wormholeErr) {
		return wormholeErr, true
	}

	// Check for ModelConstraintError which embeds WormholeError
	var constraintErr *ModelConstraintError
	if errors.As(err, &constraintErr) {
		return constraintErr.WormholeError, true
	}

	return nil, false
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
	Constraint string `json:"constraint"`
	Expected   any    `json:"expected"`
	Actual     any    `json:"actual"`
}

// NewModelConstraintError creates a new model constraint error
func NewModelConstraintError(model, constraint string, expected, actual any) *ModelConstraintError {
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

// ==================== Validation Error ====================

// ValidationError represents a field-level validation failure with details about
// which field failed and why. Use Validate() on builders to catch these errors
// before calling Generate().
//
// Example:
//
//	if err := builder.Validate(); err != nil {
//	    if vErr, ok := types.AsValidationError(err); ok {
//	        fmt.Printf("Field %s: %s\n", vErr.Field, vErr.Message)
//	    }
//	}
type ValidationError struct {
	*WormholeError
	Field      string `json:"field"`                // The field that failed validation
	Constraint string `json:"constraint,omitempty"` // The constraint that was violated (e.g., "required", "range")
	Value      any    `json:"value,omitempty"`      // The invalid value (if safe to include)
}

// NewValidationError creates a validation error for a specific field.
//
// Example:
//
//	NewValidationError("model", "required", nil, "model is required")
//	NewValidationError("temperature", "range", 3.0, "must be between 0.0 and 2.0")
func NewValidationError(field, constraint string, value any, message string) *ValidationError {
	return &ValidationError{
		WormholeError: ErrValidation.WithDetails(fmt.Sprintf("%s: %s", field, message)),
		Field:         field,
		Constraint:    constraint,
		Value:         value,
	}
}

// AsValidationError extracts a ValidationError from an error if present.
//
// Example:
//
//	if vErr, ok := types.AsValidationError(err); ok {
//	    log.Printf("Validation failed for field: %s", vErr.Field)
//	}
func AsValidationError(err error) (*ValidationError, bool) {
	if vErr, ok := err.(*ValidationError); ok {
		return vErr, true
	}
	return nil, false
}

// ==================== Multi-Field Validation ====================

// ValidationErrors collects multiple validation errors for batch reporting.
// Use this when validating multiple fields at once.
//
// Example:
//
//	var errs types.ValidationErrors
//	if model == "" {
//	    errs.Add("model", "required", nil, "model is required")
//	}
//	if temp < 0 || temp > 2 {
//	    errs.Add("temperature", "range", temp, "must be between 0.0 and 2.0")
//	}
//	if errs.HasErrors() {
//	    return errs.Error()
//	}
type ValidationErrors struct {
	Errors []*ValidationError `json:"errors"`
}

// Add appends a new validation error.
func (ve *ValidationErrors) Add(field, constraint string, value any, message string) {
	ve.Errors = append(ve.Errors, NewValidationError(field, constraint, value, message))
}

// HasErrors returns true if any validation errors were collected.
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// Error returns a combined error if there are validation errors, nil otherwise.
func (ve *ValidationErrors) Error() error {
	if !ve.HasErrors() {
		return nil
	}
	if len(ve.Errors) == 1 {
		return ve.Errors[0]
	}
	// Combine into summary
	details := fmt.Sprintf("%d validation errors: ", len(ve.Errors))
	for i, e := range ve.Errors {
		if i > 0 {
			details += "; "
		}
		details += e.Field + " - " + e.WormholeError.Details
	}
	return ErrValidation.WithDetails(details)
}

// Fields returns a list of fields that failed validation.
func (ve *ValidationErrors) Fields() []string {
	fields := make([]string, len(ve.Errors))
	for i, e := range ve.Errors {
		fields[i] = e.Field
	}
	return fields
}

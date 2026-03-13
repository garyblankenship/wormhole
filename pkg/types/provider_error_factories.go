package types

import "fmt"

// ProviderWrapperError represents provider capability errors
type ProviderWrapperError struct {
	Message      string
	ProviderName string
}

func (e *ProviderWrapperError) Error() string {
	return e.Message + " (provider: " + e.ProviderName + ")"
}

// NewProviderWrapperError creates a new provider wrapper error
func NewProviderWrapperError(message, providerName string) *ProviderWrapperError {
	return &ProviderWrapperError{
		Message:      message,
		ProviderName: providerName,
	}
}

// IsProviderWrapperError checks if an error is a provider wrapper error
func IsProviderWrapperError(err error) bool {
	_, ok := err.(*ProviderWrapperError)
	return ok
}

// WrapProviderError wraps an error with Wormhole error context
func WrapProviderError(providerName string, code ErrorCode, message string, cause error) error {
	err := NewWormholeError(code, message, isRetryableCode(code))
	err.Provider = providerName
	err.Cause = cause
	return err
}

func isRetryableCode(code ErrorCode) bool {
	switch code {
	case ErrorCodeAuth, ErrorCodeRateLimit, ErrorCodeTimeout,
		ErrorCodeProvider, ErrorCodeNetwork:
		return true
	default:
		return false
	}
}

// NotImplementedError returns a standard not implemented error
func NotImplementedError(providerName, method string) error {
	return ProviderErrorf(providerName, "%s provider does not support %s", providerName, method)
}


// NewProviderValidationError returns a WormholeError with ErrorCodeValidation
func NewProviderValidationError(providerName, message string, details ...string) error {
	err := NewWormholeError(ErrorCodeValidation, message, false)
	err.Provider = providerName
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ValidationErrorf formats a validation error
func ValidationErrorf(providerName, format string, args ...any) error {
	return NewProviderValidationError(providerName, fmt.Sprintf(format, args...))
}

// ProviderError returns a WormholeError with ErrorCodeProvider
func ProviderError(providerName, message string, details ...string) error {
	err := NewWormholeError(ErrorCodeProvider, message, true) // provider errors are retryable
	err.Provider = providerName
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ProviderErrorf formats a provider error
func ProviderErrorf(providerName, format string, args ...any) error {
	return ProviderError(providerName, fmt.Sprintf(format, args...))
}

// RequestError wraps a cause with ErrorCodeRequest
func RequestError(providerName, message string, cause error) error {
	err := NewWormholeError(ErrorCodeRequest, message, false)
	err.Provider = providerName
	err.Cause = cause
	return err
}

// ModelError returns a WormholeError with ErrorCodeModel
func ModelError(providerName, message string, details ...string) error {
	err := NewWormholeError(ErrorCodeModel, message, false)
	err.Provider = providerName
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ModelErrorf formats a model error
func ModelErrorf(providerName, format string, args ...any) error {
	return ModelError(providerName, fmt.Sprintf(format, args...))
}

// AuthError returns a WormholeError with ErrorCodeAuth
func AuthError(providerName, message string, details ...string) error {
	err := NewWormholeError(ErrorCodeAuth, message, true) // auth errors often retryable
	err.Provider = providerName
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// AuthErrorf formats an auth error
func AuthErrorf(providerName, format string, args ...any) error {
	return AuthError(providerName, fmt.Sprintf(format, args...))
}

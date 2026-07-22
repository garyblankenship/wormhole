package types

import (
	"errors"
	"fmt"
	"net/http"
)

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
	_, ok := AsWormholeError(err)
	return ok
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

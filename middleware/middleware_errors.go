package middleware

import (
	"errors"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// MiddlewareError provides structured error information for middleware failures
type MiddlewareError struct {
	Operation  string    // The operation being performed (e.g., "execute", "cache_get")
	Middleware string    // Name of the middleware (e.g., "cache", "circuit_breaker")
	Cause      error     // The underlying error
	Timestamp  time.Time // When the error occurred
}

// Error implements the error interface
func (e *MiddlewareError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s middleware failed in %s: %v", e.Middleware, e.Operation, e.Cause)
	}
	return fmt.Sprintf("%s middleware failed in %s", e.Middleware, e.Operation)
}

// Unwrap returns the underlying error
func (e *MiddlewareError) Unwrap() error {
	return e.Cause
}

// wrapMiddlewareError wraps an error with middleware context if it's not already a MiddlewareError
func wrapMiddlewareError(middlewareName, operation string, err error) error {
	if err == nil {
		return nil
	}
	// Check if already a MiddlewareError
	if _, ok := err.(*MiddlewareError); ok {
		return err
	}
	return &MiddlewareError{
		Operation:  operation,
		Middleware: middlewareName,
		Cause:      err,
		Timestamp:  time.Now(),
	}
}

// wrapIfNotWormholeError wraps an error with middleware context unless it's already a WormholeError
// This preserves the structured WormholeError while adding middleware context for other errors
func wrapIfNotWormholeError(middlewareName string, err error) error {
	if err == nil {
		return nil
	}
	// Check if already a WormholeError
	if _, ok := err.(*types.WormholeError); ok {
		return err
	}
	// Check if already a MiddlewareError
	if _, ok := err.(*MiddlewareError); ok {
		return err
	}
	return &MiddlewareError{
		Operation:  "execute",
		Middleware: middlewareName,
		Cause:      err,
		Timestamp:  time.Now(),
	}
}

// IsMiddlewareError checks if an error is a MiddlewareError or contains one
func IsMiddlewareError(err error) bool {
	var me *MiddlewareError
	return errors.As(err, &me)
}

// AsMiddlewareError extracts a MiddlewareError from an error
func AsMiddlewareError(err error) (*MiddlewareError, bool) {
	var middlewareErr *MiddlewareError
	if errors.As(err, &middlewareErr) {
		return middlewareErr, true
	}
	return nil, false
}

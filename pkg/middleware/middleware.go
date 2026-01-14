package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Middleware represents a function that wraps provider calls
// DEPRECATED: Use types.ProviderMiddleware for type-safe middleware instead
type Middleware func(next Handler) Handler

// Handler represents any provider method signature
type Handler func(ctx context.Context, req any) (any, error)

// Chain manages a chain of middleware
// DEPRECATED: Use types.ProviderMiddlewareChain for type-safe middleware instead
type Chain struct {
	middlewares []Middleware
}

// NewChain creates a new middleware chain
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Apply wraps a handler function with all middleware
func (c *Chain) Apply(handler Handler) Handler {
	// Apply middleware in reverse order so they execute in the order added
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}

// Add adds middleware to the chain
func (c *Chain) Add(middleware Middleware) {
	c.middlewares = append(c.middlewares, middleware)
}

// MetricsMiddleware tracks request metrics
func MetricsMiddleware(metrics *Metrics) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			resp, err := next(ctx, req)

			duration := time.Since(start)
			metrics.RecordRequest(duration, err)

			return resp, wrapIfNotWormholeError("metrics", "execute", err)
		}
	}
}

// LoggingMiddleware creates basic logging middleware
func LoggingMiddleware(logger types.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			logger.Debug("Wormhole request", "request", req)

			resp, err := next(ctx, req)

			if err != nil {
				logger.Error("Wormhole request failed", "error", err)
			} else {
				logger.Debug("Wormhole response", "response", resp)
			}

			return resp, wrapIfNotWormholeError("logging", "execute", err)
		}
	}
}

// TimeoutMiddleware enforces request timeouts
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			type result struct {
				resp any
				err  error
			}

			done := make(chan result, 1)

			go func() {
				resp, err := next(ctx, req)
				done <- result{resp, err}
			}()

			select {
			case <-ctx.Done():
				return nil, wrapMiddlewareError("timeout", "execute", ctx.Err())
			case res := <-done:
				return res.resp, wrapIfNotWormholeError("timeout", "execute", res.err)
			}
		}
	}
}

// Metrics tracks provider metrics using atomic operations for better performance
type Metrics struct {
	totalRequests int64 // atomic counter
	totalErrors   int64 // atomic counter
	totalDuration int64 // atomic counter (nanoseconds)
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordRequest records a request metric using atomic operations
func (m *Metrics) RecordRequest(duration time.Duration, err error) {
	atomic.AddInt64(&m.totalRequests, 1)
	atomic.AddInt64(&m.totalDuration, int64(duration))

	if err != nil {
		atomic.AddInt64(&m.totalErrors, 1)
	}
}

// GetStats returns current metrics using atomic loads
func (m *Metrics) GetStats() (requests int64, errors int64, avgDuration time.Duration) {
	requests = atomic.LoadInt64(&m.totalRequests)
	errors = atomic.LoadInt64(&m.totalErrors)
	totalDurationNs := atomic.LoadInt64(&m.totalDuration)

	if requests > 0 {
		avgDuration = time.Duration(totalDurationNs / requests)
	}

	return
}

// MiddlewareInfo describes available middleware
type MiddlewareInfo struct {
	Name       string
	Purpose    string
	Example    string
	ConfigType string
}

// AvailableMiddleware returns information about all available middleware
func AvailableMiddleware() []MiddlewareInfo {
	return []MiddlewareInfo{
		{
			Name:       "CacheMiddleware",
			Purpose:    "Response caching with TTL support",
			Example:    "middleware.CacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: 5*time.Minute})",
			ConfigType: "CacheConfig",
		},
		{
			Name:       "CircuitBreakerMiddleware",
			Purpose:    "Circuit breaking for failing providers",
			Example:    "middleware.CircuitBreakerMiddleware(5, 30*time.Second)",
			ConfigType: "threshold int, timeout time.Duration",
		},
		{
			Name:       "RateLimitMiddleware",
			Purpose:    "Request rate limiting",
			Example:    "middleware.RateLimitMiddleware(100)",
			ConfigType: "requestsPerSecond int",
		},
		{
			Name:       "LoadBalancerMiddleware",
			Purpose:    "Load balancing across multiple providers",
			Example:    "middleware.LoadBalancerMiddleware(providers, strategy)",
			ConfigType: "providers []string, strategy LoadBalanceStrategy",
		},
		{
			Name:       "HealthMiddleware",
			Purpose:    "Provider health checking",
			Example:    "middleware.HealthMiddleware(config)",
			ConfigType: "HealthConfig",
		},
		{
			Name:       "LoggingMiddleware",
			Purpose:    "Request/response logging",
			Example:    "middleware.LoggingMiddleware(logger)",
			ConfigType: "logger types.Logger",
		},
		{
			Name:       "MetricsMiddleware",
			Purpose:    "Request metrics collection",
			Example:    "middleware.MetricsMiddleware(metrics)",
			ConfigType: "metrics *Metrics",
		},
		{
			Name:       "TimeoutMiddleware",
			Purpose:    "Request timeout enforcement",
			Example:    "middleware.TimeoutMiddleware(30*time.Second)",
			ConfigType: "timeout time.Duration",
		},
	}
}

// ==================== Error Standardization ====================

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
func wrapIfNotWormholeError(middlewareName, operation string, err error) error {
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
		Operation:  operation,
		Middleware: middlewareName,
		Cause:      err,
		Timestamp:  time.Now(),
	}
}

// IsMiddlewareError checks if an error is a MiddlewareError or contains one
func IsMiddlewareError(err error) bool {
	if _, ok := err.(*MiddlewareError); ok {
		return true
	}
	return false
}

// AsMiddlewareError extracts a MiddlewareError from an error
func AsMiddlewareError(err error) (*MiddlewareError, bool) {
	var middlewareErr *MiddlewareError
	if errors.As(err, &middlewareErr) {
		return middlewareErr, true
	}
	return nil, false
}

package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// contextKey is an unexported type for context keys in the middleware package.
// Using a distinct type prevents collisions with keys defined in other packages.
type contextKey string

// Context keys used by middleware to extract request metadata.
const (
	// CtxKeyProvider identifies the LLM provider (e.g. "openai", "anthropic").
	CtxKeyProvider contextKey = "provider"

	// CtxKeyModel identifies the model being used.
	CtxKeyModel contextKey = "model"

	// CtxKeyMethod identifies the request method (e.g. "text", "stream").
	CtxKeyMethod contextKey = "method"

	// CtxKeyWormholeProvider is an alternative provider key used by the typed
	// enhanced metrics middleware.
	CtxKeyWormholeProvider contextKey = "wormhole_provider"
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
			resp, err := withMeasuredRequest(ctx, req, next, func(resp any, err error, duration time.Duration) {
				metrics.RecordRequest(duration, err)
			})
			return resp, wrapIfNotWormholeError("metrics", err)
		}
	}
}

// EnhancedMetricsMiddleware tracks request metrics with enhanced features
func EnhancedMetricsMiddleware(collector *EnhancedMetricsCollector) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			resp, err := withMeasuredRequest(ctx, req, next, func(resp any, err error, duration time.Duration) {
				collector.RecordRequest(requestLabelsFromContext(ctx, "", ""), duration, err, 0, 0, 0)
			})
			return resp, wrapIfNotWormholeError("metrics", err)
		}
	}
}

// LoggingMiddleware creates basic logging middleware
func LoggingMiddleware(logger types.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			logger.Debug("Wormhole request", "request_type", fmt.Sprintf("%T", req))

			resp, err := next(ctx, req)

			if err != nil {
				args := make([]any, 0, 5)
				args = append(args, "error", types.SafeErrorValue(err))
				args = append(args, requestMetadataAttrs(ctx)...)
				logger.Error("Wormhole request failed", args...)
			} else {
				logger.Debug("Wormhole response", "response_type", fmt.Sprintf("%T", resp))
			}

			return resp, wrapIfNotWormholeError("logging", err)
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
				return res.resp, wrapIfNotWormholeError("timeout", res.err)
			}
		}
	}
}

// Metrics tracks provider metrics using atomic operations for better performance
// DEPRECATED: Use EnhancedMetricsCollector for richer metrics with labels and histograms
type Metrics struct {
	totalRequests int64 // atomic counter
	totalErrors   int64 // atomic counter
	totalDuration int64 // atomic counter (nanoseconds)

	// Enhanced metrics collector (optional)
	enhanced *EnhancedMetricsCollector
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

	// Also record to enhanced metrics if available
	if m.enhanced != nil {
		// Record without labels for backward compatibility
		m.enhanced.RecordRequest(nil, duration, err, 0, 0, 0)
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

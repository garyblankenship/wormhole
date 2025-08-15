package middleware

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Middleware represents a function that wraps provider calls
type Middleware func(next Handler) Handler

// Handler represents any provider method signature
type Handler func(ctx context.Context, req interface{}) (interface{}, error)

// Chain manages a chain of middleware
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
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			start := time.Now()

			resp, err := next(ctx, req)

			duration := time.Since(start)
			metrics.RecordRequest(duration, err)

			return resp, err
		}
	}
}

// LoggingMiddleware creates basic logging middleware
func LoggingMiddleware(logger types.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			logger.Debug("Wormhole request", "request", req)

			resp, err := next(ctx, req)

			if err != nil {
				logger.Error("Wormhole request failed", "error", err)
			} else {
				logger.Debug("Wormhole response", "response", resp)
			}

			return resp, err
		}
	}
}

// TimeoutMiddleware enforces request timeouts
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			type result struct {
				resp interface{}
				err  error
			}

			done := make(chan result, 1)

			go func() {
				resp, err := next(ctx, req)
				done <- result{resp, err}
			}()

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case res := <-done:
				return res.resp, res.err
			}
		}
	}
}

// Metrics tracks provider metrics using atomic operations for better performance
type Metrics struct {
	totalRequests int64        // atomic counter
	totalErrors   int64        // atomic counter  
	totalDuration int64        // atomic counter (nanoseconds)
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
	Name        string
	Purpose     string
	Example     string
	ConfigType  string
}

// AvailableMiddleware returns information about all available middleware
func AvailableMiddleware() []MiddlewareInfo {
	return []MiddlewareInfo{
		{
			Name:       "RetryMiddleware",
			Purpose:    "Exponential backoff retry with jitter",
			Example:    "middleware.RetryMiddleware(middleware.DefaultRetryConfig())",
			ConfigType: "RetryConfig",
		},
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

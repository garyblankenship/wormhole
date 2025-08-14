package middleware

import (
	"context"
	"sync"
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

// Metrics tracks provider metrics
type Metrics struct {
	mu            sync.RWMutex
	totalRequests int64
	totalErrors   int64
	totalDuration time.Duration
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordRequest records a request metric
func (m *Metrics) RecordRequest(duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.totalDuration += duration

	if err != nil {
		m.totalErrors++
	}
}

// GetStats returns current metrics
func (m *Metrics) GetStats() (requests int64, errors int64, avgDuration time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	requests = m.totalRequests
	errors = m.totalErrors

	if requests > 0 {
		avgDuration = m.totalDuration / time.Duration(requests)
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

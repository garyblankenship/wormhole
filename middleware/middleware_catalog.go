package middleware

import (
	"context"
	"time"
)

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
		{
			Name:       "AdaptiveRateLimitMiddleware",
			Purpose:    "Adaptive rate limiting based on response latency",
			Example:    "middleware.AdaptiveRateLimitMiddleware(5, 2, 10, 50*time.Millisecond)",
			ConfigType: "initialRate, minRate, maxRate int, targetLatency time.Duration",
		},
		{
			Name:       "HealthAwareAdaptiveRateLimitMiddleware",
			Purpose:    "Health-aware adaptive rate limiting with circuit breaker integration",
			Example:    "middleware.HealthAwareAdaptiveRateLimitMiddleware(5, 2, 10, 50*time.Millisecond, \"openai\", checker, breaker)",
			ConfigType: "initialRate, minRate, maxRate int, targetLatency time.Duration, providerName string, checker *HealthChecker, breaker *CircuitBreaker",
		},
		{
			Name:       "ProviderAwareConcurrencyLimitMiddleware",
			Purpose:    "Provider-aware adaptive concurrency control with PID tuning",
			Example:    "middleware.ProviderAwareConcurrencyLimitMiddleware(limiter)",
			ConfigType: "limiter ProviderAwareLimiter",
		},
		{
			Name:       "ProviderAwareConcurrencyLimitMiddlewareWithConfig",
			Purpose:    "Provider-aware adaptive concurrency control with configurable provider-awareness",
			Example:    "middleware.ProviderAwareConcurrencyLimitMiddlewareWithConfig(middleware.ProviderAwareConcurrencyLimitConfig{Limiter: limiter, EnableProviderAware: true})",
			ConfigType: "ProviderAwareConcurrencyLimitConfig",
		},
	}
}

// ProviderAwareLimiter defines the interface for provider-aware adaptive limiters
// This interface allows middleware to work with any limiter implementation
// without creating import cycles
type ProviderAwareLimiter interface {
	// Acquire acquires a slot from the global limiter.
	//
	// Deprecated: Use AcquireToken instead to prevent race conditions.
	Acquire(ctx context.Context) bool

	// AcquireWithProvider acquires a slot with provider/model awareness.
	//
	// Deprecated: Use AcquireTokenWithProvider instead to prevent race conditions.
	AcquireWithProvider(ctx context.Context, provider, model string) bool

	// Release releases a slot to the global limiter.
	//
	// Deprecated: Use the release function returned by AcquireToken instead.
	Release()

	// ReleaseWithProvider releases a slot with provider/model awareness.
	//
	// Deprecated: Use the release function returned by AcquireTokenWithProvider instead.
	ReleaseWithProvider(provider, model string)

	// AcquireToken acquires a slot and returns a release function that captures
	// the specific limiter instance, preventing race conditions if capacity
	// adjustment swaps the limiter between acquire and release.
	AcquireToken(ctx context.Context) (release func(), ok bool)

	// AcquireTokenWithProvider acquires a slot with provider/model awareness
	// and returns a release function that captures the specific limiter instance.
	AcquireTokenWithProvider(ctx context.Context, provider, model string) (release func(), ok bool)

	// RecordLatency records latency for global limiter
	RecordLatency(latency time.Duration)

	// RecordLatencyWithProvider records latency with provider/model and error info
	RecordLatencyWithProvider(latency time.Duration, provider, model string, err error)
}

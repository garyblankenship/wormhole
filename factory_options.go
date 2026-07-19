package wormhole

import (
	"os"
	"time"

	"github.com/garyblankenship/wormhole/v2/middleware"
	"github.com/garyblankenship/wormhole/v2/types"
)

// WithRateLimit returns an option to add rate limiting middleware
func (f *SimpleFactory) WithRateLimit(requestsPerSecond int) Option {
	return WithMiddleware(middleware.RateLimitMiddleware(requestsPerSecond))
}

// WithCircuitBreaker returns an option to add circuit breaker middleware
func (f *SimpleFactory) WithCircuitBreaker(threshold int, timeout time.Duration) Option {
	return WithMiddleware(middleware.CircuitBreakerMiddleware(threshold, timeout))
}

// WithCache returns an option to add caching middleware
func (f *SimpleFactory) WithCache(ttl time.Duration) Option {
	cache := middleware.NewMemoryCache(1000)
	config := middleware.CacheConfig{
		Cache: cache,
		TTL:   ttl,
	}
	return func(c *Config) {
		c.Closers = append(c.Closers, cache)
		WithMiddleware(middleware.CacheMiddleware(config))(c)
	}
}

// WithTimeout returns an option to add timeout middleware
func (f *SimpleFactory) WithTimeout(timeout time.Duration) Option {
	return WithProviderMiddleware(middleware.NewTypedTimeoutMiddleware(timeout))
}

// WithMetrics returns an option to add metrics tracking middleware and the metrics instance
func (f *SimpleFactory) WithMetrics() (Option, *middleware.TypedMetrics) {
	metrics := middleware.NewTypedMetrics()
	return WithProviderMiddleware(middleware.NewTypedMetricsMiddleware(metrics)), metrics
}

// WithLogging returns an option to add basic logging middleware
func (f *SimpleFactory) WithLogging(logger types.Logger) Option {
	return WithMiddleware(middleware.LoggingMiddleware(logger))
}

// WithDetailedLogging returns an option to add detailed logging middleware with configuration
func (f *SimpleFactory) WithDetailedLogging(logger types.Logger) Option {
	config := middleware.DefaultLoggingConfig(logger)
	return WithMiddleware(middleware.DetailedLoggingMiddleware(config))
}

// WithDebugLogging returns an option to add debug logging middleware
func (f *SimpleFactory) WithDebugLogging(logger types.Logger) Option {
	return WithProviderMiddleware(middleware.NewDebugTypedLoggingMiddleware(logger))
}

// getAPIKey retrieves API key from provided value or environment variables
func (f *SimpleFactory) getAPIKey(provided []string, envVars ...string) string {
	// Check if API key was provided directly
	if len(provided) > 0 && provided[0] != "" {
		return provided[0]
	}

	// Check environment variables
	for _, env := range envVars {
		if key := os.Getenv(env); key != "" {
			return key
		}
	}

	return ""
}

func (f *SimpleFactory) getProfileAPIKey(provided []string, provider string) string {
	if len(provided) > 0 && provided[0] != "" {
		return provided[0]
	}
	profile, ok := providerProfile(provider)
	if !ok {
		return ""
	}
	return configuredAPIKey(profile)
}

func (f *SimpleFactory) getRequiredProfileBaseURL(provided []string, provider string) (string, bool) {
	if len(provided) > 0 && provided[0] != "" {
		return provided[0], true
	}
	profile, ok := providerProfile(provider)
	if !ok {
		return "", false
	}
	if profile.BaseURLEnv != "" {
		if value := os.Getenv(profile.BaseURLEnv); value != "" {
			return value, true
		}
	}
	return "", false
}

func primaryAPIKeyEnv(provider string) string {
	profile, ok := providerProfile(provider)
	if !ok || len(profile.APIKeyEnv) == 0 {
		return provider + "_API_KEY"
	}
	return profile.APIKeyEnv[0]
}

func primaryBaseURLEnv(provider string) string {
	profile, ok := providerProfile(provider)
	if !ok || profile.BaseURLEnv == "" {
		return provider + "_BASE_URL"
	}
	return profile.BaseURLEnv
}

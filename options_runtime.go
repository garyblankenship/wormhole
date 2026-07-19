package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/v2/middleware"
	"github.com/garyblankenship/wormhole/v2/types"
)

// WithCustomProvider registers a custom provider with its factory function.
func WithCustomProvider(name string, factory types.ProviderFactory) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}

		// Ensure a config placeholder exists
		if _, ok := c.Providers[name]; !ok {
			c.Providers[name] = types.ProviderConfig{}
		}

		c.CustomFactories[name] = factory
	}
}

// WithProviderConfig sets the configuration for a specific provider.
func WithProviderConfig(name string, config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		c.Providers[name] = config
	}
}

// WithMiddleware adds middleware to the client's execution chain.
// DEPRECATED: Use WithProviderMiddleware for type-safe middleware instead.
// This function automatically converts legacy middleware to type-safe middleware
// using adapter pattern for backward compatibility.
func WithMiddleware(mw ...middleware.Middleware) Option {
	return func(c *Config) {
		// Store legacy middleware for backward compatibility
		c.Middleware = append(c.Middleware, mw...)

		// Convert legacy middleware to type-safe middleware using adapter
		for _, legacyMw := range mw {
			adapter := middleware.NewLegacyAdapter(legacyMw)
			c.ProviderMiddlewares = append(c.ProviderMiddlewares, adapter)
		}
	}
}

// WithProviderMiddleware adds type-safe middleware to the client's execution chain.
// Use this for compile-time type checking instead of the deprecated WithMiddleware.
func WithProviderMiddleware(mw ...types.ProviderMiddleware) Option {
	return func(c *Config) {
		c.ProviderMiddlewares = append(c.ProviderMiddlewares, mw...)
	}
}

// WithTimeout sets the default timeout for requests.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.DefaultTimeout = timeout
		c.DefaultTimeoutSet = true
	}
}

// WithUnlimitedTimeout disables HTTP client timeouts for long-running AI processing.
// Use for heavy text processing that may take 3+ minutes.
func WithUnlimitedTimeout() Option {
	return func(c *Config) {
		c.DefaultTimeout = 0 // 0 = unlimited timeout
		c.DefaultTimeoutSet = true
	}
}

// WithRetries sets default HTTP retry behavior for providers that do not set
// ProviderConfig.MaxRetries or RetryDelay. maxRetries may be zero to disable
// retries by default.
func WithRetries(maxRetries int, delay time.Duration) Option {
	return func(c *Config) {
		c.DefaultRetries = maxRetries
		c.DefaultRetriesSet = true
		c.DefaultRetryDelay = delay
		c.DefaultRetryDelaySet = true
	}
}

// WithDebugLogging enables debug logging with an optional custom logger.
func WithDebugLogging(logger ...types.Logger) Option {
	return func(c *Config) {
		c.DebugLogging = true
		if len(logger) > 0 && logger[0] != nil {
			c.Logger = logger[0]
		}
	}
}

// WithLogger sets a custom logger for the client.
func WithLogger(logger types.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithAttemptTrace configures a callback for provider/model attempts.
func WithAttemptTrace(trace AttemptTraceFunc) Option {
	return func(c *Config) {
		c.AttemptTrace = trace
	}
}

// WithStreamIdleTimeout configures a per-chunk idle timeout for streaming responses.
// A stream that stops emitting chunks for longer than this duration fails with
// a typed timeout error. Zero or negative disables the watchdog (default).
func WithStreamIdleTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.StreamIdleTimeout = d
	}
}

// WithStreamTrace configures a callback for stream lifecycle events.
// Terminal events (StreamEnded, StreamError) are emitted exactly once per stream.
func WithStreamTrace(trace StreamTraceFunc) Option {
	return func(c *Config) {
		c.StreamTrace = trace
	}
}

// WithModelValidation enables or disables model validation against the registry.
func WithModelValidation(enabled bool) Option {
	return func(c *Config) {
		c.ModelValidation = enabled
	}
}

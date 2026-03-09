package middleware

import (
	"context"
	"time"
)

// ProviderAwareConcurrencyLimitConfig holds configuration for provider-aware concurrency limiting.
type ProviderAwareConcurrencyLimitConfig struct {
	Limiter ProviderAwareLimiter

	// EnableProviderAware controls whether provider-aware limiting is enabled.
	// When false, the middleware falls back to global limiting only.
	EnableProviderAware bool
}

// ProviderAwareConcurrencyLimitMiddleware creates a middleware with provider-aware adaptive concurrency control.
func ProviderAwareConcurrencyLimitMiddleware(limiter ProviderAwareLimiter) Middleware {
	return ProviderAwareConcurrencyLimitMiddlewareWithConfig(ProviderAwareConcurrencyLimitConfig{
		Limiter:             limiter,
		EnableProviderAware: true,
	})
}

// ProviderAwareConcurrencyLimitMiddlewareWithConfig creates a middleware with provider-aware adaptive concurrency control using config.
func ProviderAwareConcurrencyLimitMiddlewareWithConfig(config ProviderAwareConcurrencyLimitConfig) Middleware {
	limiter := config.Limiter
	enableProviderAware := config.EnableProviderAware

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			provider, model := providerModelFromContext(ctx, enableProviderAware)
			release, ok := acquireConcurrencyToken(ctx, limiter, provider, model, enableProviderAware)
			if !ok {
				return nil, wrapMiddlewareError("provider_aware_concurrency_limit", "acquire", ctx.Err())
			}

			resp, err := next(ctx, req)
			latency := time.Since(start)

			recordConcurrencyLatency(limiter, latency, provider, model, err, enableProviderAware)
			release()

			return resp, wrapIfNotWormholeError("provider_aware_concurrency_limit", err)
		}
	}
}

func providerModelFromContext(ctx context.Context, enabled bool) (string, string) {
	if !enabled {
		return "", ""
	}
	labels := requestLabelsFromContext(ctx, "", "")
	if labels == nil {
		return "", ""
	}
	return labels.Provider, labels.Model
}

func acquireConcurrencyToken(ctx context.Context, limiter ProviderAwareLimiter, provider, model string, enabled bool) (func(), bool) {
	if enabled && provider != "" {
		return limiter.AcquireTokenWithProvider(ctx, provider, model)
	}
	return limiter.AcquireToken(ctx)
}

func recordConcurrencyLatency(limiter ProviderAwareLimiter, latency time.Duration, provider, model string, err error, enabled bool) {
	if enabled && provider != "" {
		limiter.RecordLatencyWithProvider(latency, provider, model, err)
		return
	}
	limiter.RecordLatency(latency)
}

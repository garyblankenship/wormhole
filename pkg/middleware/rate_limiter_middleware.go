package middleware

import (
	"context"
	"time"
)

type waitingLimiter interface {
	Wait(context.Context) error
}

type healthMetricsSource struct {
	providerName string
	checker      *HealthChecker
	breaker      *CircuitBreaker
}

func RateLimitMiddleware(requestsPerSecond int) Middleware {
	return newRateLimitedMiddleware("rate_limiter", NewRateLimiter(requestsPerSecond), nil)
}

// AdaptiveRateLimitMiddleware creates a middleware with adaptive rate limiting.
func AdaptiveRateLimitMiddleware(initialRate, minRate, maxRate int, targetLatency time.Duration) Middleware {
	limiter := NewAdaptiveRateLimiter(initialRate, minRate, maxRate, targetLatency)
	return newRateLimitedMiddleware("adaptive_rate_limiter", limiter, func(latency time.Duration, err error) {
		limiter.RecordLatency(latency)
	})
}

// HealthAwareAdaptiveRateLimitMiddleware creates a middleware with health-aware adaptive rate limiting.
func HealthAwareAdaptiveRateLimitMiddleware(initialRate, minRate, maxRate int, targetLatency time.Duration, providerName string, checker *HealthChecker, breaker *CircuitBreaker) Middleware {
	limiter := NewHealthAwareAdaptiveRateLimiter(initialRate, minRate, maxRate, targetLatency)
	source := healthMetricsSource{
		providerName: providerName,
		checker:      checker,
		breaker:      breaker,
	}

	return newRateLimitedMiddleware("health_aware_adaptive_rate_limiter", limiter, func(latency time.Duration, err error) {
		limiter.RecordLatency(latency)
		if metrics := source.snapshot(err); metrics != nil {
			limiter.RecordHealthMetrics(metrics)
		}
	})
}

func newRateLimitedMiddleware(name string, limiter waitingLimiter, after func(time.Duration, error)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()

			if err := limiter.Wait(ctx); err != nil {
				return nil, wrapMiddlewareError(name, "wait", err)
			}

			resp, err := next(ctx, req)
			if after != nil {
				after(time.Since(start), err)
			}

			return resp, wrapIfNotWormholeError(name, err)
		}
	}
}

func (s healthMetricsSource) snapshot(err error) *HealthMetrics {
	if s.checker == nil || s.breaker == nil {
		return nil
	}

	healthStatus := s.checker.GetStatus(s.providerName)
	errorRate := 0.0
	if err != nil {
		errorRate = 1.0
	}

	return &HealthMetrics{
		CircuitState:     s.breaker.GetState(),
		Healthy:          healthStatus.Healthy,
		ErrorRate:        errorRate,
		ResponseTime:     healthStatus.ResponseTime,
		ConsecutiveFails: healthStatus.ConsecutiveFails,
		LastCheck:        healthStatus.LastCheck,
	}
}

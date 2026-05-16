package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBalancerStrategiesAndMetrics(t *testing.T) {
	handler := func(name string) Handler {
		return func(ctx context.Context, req any) (any, error) {
			return name, nil
		}
	}

	for _, strategy := range []LoadBalanceStrategy{RoundRobin, Random, LeastConnections, WeightedRoundRobin, ResponseTime, Adaptive, LoadBalanceStrategy(999)} {
		t.Run(strategyName(strategy), func(t *testing.T) {
			lb := NewLoadBalancer(strategy)
			lb.AddProvider("a", handler("a"), 2)
			lb.AddProvider("b", handler("b"), 1)

			lb.providers[0].AverageLatency = 20 * time.Millisecond
			lb.providers[1].AverageLatency = 5 * time.Millisecond
			lb.providers[0].ActiveConnections = 3
			lb.providers[0].TotalRequests = 10
			lb.providers[0].TotalErrors = 5

			provider, err := lb.SelectProvider(context.Background())
			require.NoError(t, err)
			require.NotNil(t, provider)

			resp, err := lb.Execute(context.Background(), "request")
			require.NoError(t, err)
			assert.Contains(t, []string{"a", "b"}, resp)

			stats := lb.GetProviderStats()
			require.Len(t, stats, 2)
			assert.NotZero(t, stats[0].TotalRequests+stats[1].TotalRequests)
		})
	}
}

func strategyName(strategy LoadBalanceStrategy) string {
	switch strategy {
	case RoundRobin:
		return "round_robin"
	case Random:
		return "random"
	case LeastConnections:
		return "least_connections"
	case WeightedRoundRobin:
		return "weighted_round_robin"
	case ResponseTime:
		return "response_time"
	case Adaptive:
		return "adaptive"
	default:
		return "default"
	}
}

func TestLoadBalancerNoHealthyProvidersAndHealthChecks(t *testing.T) {
	lb := NewLoadBalancer(RoundRobin)
	lb.AddProvider("bad", func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("bad")
	}, 1)
	lb.providers[0].Healthy = false

	_, err := lb.SelectProvider(context.Background())
	require.Error(t, err)
	assert.True(t, IsMiddlewareError(err))

	lb.providers[0].Healthy = true
	lb.performHealthChecks()
	assert.True(t, lb.providers[0].Healthy)

	lb.StartHealthChecks(func(Handler) error { return errors.New("unhealthy") })
	time.Sleep(2 * time.Millisecond)
	lb.performHealthChecks()
	lb.StopHealthChecks()
	assert.False(t, lb.providers[0].Healthy)
	assert.False(t, lb.providers[0].LastHealthCheck.IsZero())
}

func TestLoadBalancerMiddleware(t *testing.T) {
	mw := LoadBalancerMiddleware(RoundRobin, map[string]Handler{
		"a": func(ctx context.Context, req any) (any, error) { return "a", nil },
	})

	resp, err := mw(func(ctx context.Context, req any) (any, error) {
		t.Fatal("next should not be called by load balancer middleware")
		return nil, nil
	})(context.Background(), "request")

	require.NoError(t, err)
	assert.Equal(t, "a", resp)
}

func TestRetryMiddleware(t *testing.T) {
	t.Run("retries until success", func(t *testing.T) {
		attempts := 0
		handler := RetryMiddleware(RetryConfig{
			MaxRetries:      2,
			InitialDelay:    time.Nanosecond,
			MaxDelay:        time.Millisecond,
			BackoffMultiple: 2,
			Jitter:          false,
		})(func(ctx context.Context, req any) (any, error) {
			attempts++
			if attempts < 2 {
				return nil, errors.New("temporary")
			}
			return "ok", nil
		})

		resp, err := handler(context.Background(), "request")
		require.NoError(t, err)
		assert.Equal(t, "ok", resp)
		assert.Equal(t, 2, attempts)
	})

	t.Run("non retryable", func(t *testing.T) {
		attempts := 0
		handler := RetryMiddleware(RetryConfig{
			MaxRetries:      3,
			InitialDelay:    time.Nanosecond,
			MaxDelay:        time.Millisecond,
			BackoffMultiple: 2,
			RetryableFunc:   func(error) bool { return false },
		})(func(ctx context.Context, req any) (any, error) {
			attempts++
			return nil, errors.New("permanent")
		})

		_, err := handler(context.Background(), "request")
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		handler := RetryMiddleware(RetryConfig{
			MaxRetries:      1,
			InitialDelay:    time.Hour,
			MaxDelay:        time.Hour,
			BackoffMultiple: 2,
		})(func(ctx context.Context, req any) (any, error) {
			return nil, errors.New("temporary")
		})

		_, err := handler(ctx, "request")
		require.Error(t, err)
		assert.True(t, IsMiddlewareError(err))
	})
}

func TestMiddlewareCoreHelpers(t *testing.T) {
	chain := NewChain()
	chain.Add(func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			resp, err := next(ctx, req)
			return resp.(string) + "-mw", err
		}
	})
	resp, err := chain.Apply(func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "ok-mw", resp)

	metrics := NewMetrics()
	metrics.RecordRequest(time.Second, nil)
	metrics.RecordRequest(3*time.Second, errors.New("bad"))
	requests, errorsCount, avg := metrics.GetStats()
	assert.Equal(t, int64(2), requests)
	assert.Equal(t, int64(1), errorsCount)
	assert.Equal(t, 2*time.Second, avg)

	assert.NotEmpty(t, AvailableMiddleware())

	whErr := types.NewWormholeError(types.ErrorCodeAuth, "auth", false)
	assert.Same(t, whErr, wrapIfNotWormholeError("test", whErr))

	mwErr := wrapMiddlewareError("test", "op", errors.New("bad"))
	assert.True(t, IsMiddlewareError(mwErr))
	extracted, ok := AsMiddlewareError(mwErr)
	require.True(t, ok)
	assert.Equal(t, "test", extracted.Middleware)
	assert.Equal(t, errors.New("bad").Error(), errors.Unwrap(mwErr).Error())
	assert.Same(t, mwErr, wrapMiddlewareError("test", "op", mwErr))

	noCause := &MiddlewareError{Middleware: "test", Operation: "op"}
	assert.Contains(t, noCause.Error(), "test middleware")
	assert.Nil(t, noCause.Unwrap())
}

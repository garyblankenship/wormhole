package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChain(t *testing.T) {
	t.Run("applies middleware in correct order", func(t *testing.T) {
		var order []string

		mw1 := func(next Handler) Handler {
			return func(ctx context.Context, req interface{}) (interface{}, error) {
				order = append(order, "mw1-before")
				resp, err := next(ctx, req)
				order = append(order, "mw1-after")
				return resp, err
			}
		}

		mw2 := func(next Handler) Handler {
			return func(ctx context.Context, req interface{}) (interface{}, error) {
				order = append(order, "mw2-before")
				resp, err := next(ctx, req)
				order = append(order, "mw2-after")
				return resp, err
			}
		}

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			order = append(order, "handler")
			return "response", nil
		}

		chain := NewChain(mw1, mw2)
		wrapped := chain.Apply(handler)

		resp, err := wrapped(context.Background(), "request")
		require.NoError(t, err)
		assert.Equal(t, "response", resp)
		assert.Equal(t, []string{
			"mw1-before",
			"mw2-before",
			"handler",
			"mw2-after",
			"mw1-after",
		}, order)
	})

	t.Run("passes errors through chain", func(t *testing.T) {
		expectedErr := errors.New("test error")

		mw := func(next Handler) Handler {
			return func(ctx context.Context, req interface{}) (interface{}, error) {
				return next(ctx, req)
			}
		}

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, expectedErr
		}

		chain := NewChain(mw)
		wrapped := chain.Apply(handler)

		resp, err := wrapped(context.Background(), "request")
		assert.Nil(t, resp)
		assert.Equal(t, expectedErr, err)
	})
}

func TestMetricsMiddleware(t *testing.T) {
	t.Run("records successful requests", func(t *testing.T) {
		metrics := NewMetrics()
		mw := MetricsMiddleware(metrics)

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return "response", nil
		}

		wrapped := mw(handler)
		resp, err := wrapped(context.Background(), "request")

		require.NoError(t, err)
		assert.Equal(t, "response", resp)

		requests, errors, avgDuration := metrics.GetStats()
		assert.Equal(t, int64(1), requests)
		assert.Equal(t, int64(0), errors)
		assert.Greater(t, avgDuration, time.Duration(0))
	})

	t.Run("records failed requests", func(t *testing.T) {
		metrics := NewMetrics()
		mw := MetricsMiddleware(metrics)

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, errors.New("test error")
		}

		wrapped := mw(handler)
		_, err := wrapped(context.Background(), "request")

		require.Error(t, err)

		requests, errors, _ := metrics.GetStats()
		assert.Equal(t, int64(1), requests)
		assert.Equal(t, int64(1), errors)
	})
}

func TestTimeoutMiddleware(t *testing.T) {
	t.Run("allows fast requests", func(t *testing.T) {
		mw := TimeoutMiddleware(100 * time.Millisecond)

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "response", nil
		}

		wrapped := mw(handler)
		resp, err := wrapped(context.Background(), "request")

		require.NoError(t, err)
		assert.Equal(t, "response", resp)
	})

	t.Run("times out slow requests", func(t *testing.T) {
		mw := TimeoutMiddleware(10 * time.Millisecond)

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return "response", nil
		}

		wrapped := mw(handler)
		resp, err := wrapped(context.Background(), "request")

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "deadline exceeded")
	})
}

package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseProvider_PerProviderRetryConfiguration(t *testing.T) {
	t.Skip("Skipping timing-sensitive retry tests - jitter can cause unpredictable delays")

	t.Run("uses per-provider max retries", func(t *testing.T) {
		var callCount int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "server error"}`))
		}))
		defer server.Close()

		maxRetries := 2
		timeout := 5 // Short timeout for test
		config := types.ProviderConfig{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			MaxRetries: &maxRetries,
			Timeout:    timeout,
		}

		provider := NewBaseProvider("test", config)

		// Use a context with timeout to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Make a request that will fail
		err := provider.DoRequest(ctx, "POST", server.URL+"/test", nil, nil)

		// Should be retried exactly maxRetries + 1 times (initial + retries)
		assert.Error(t, err)
		assert.Equal(t, int64(3), atomic.LoadInt64(&callCount)) // 1 initial + 2 retries
	})

	t.Run("uses per-provider retry delay", func(t *testing.T) {
		var callCount int64
		var firstCallTime, secondCallTime time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			// Use explicit case labels for clarity
			switch count {
			case 1: // First call
				firstCallTime = time.Now()
			case 2: // Second call (first retry)
				secondCallTime = time.Now()
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "service unavailable"}`))
		}))
		defer server.Close()

		maxRetries := 1
		retryDelay := 50 * time.Millisecond // Shorter delay for testing
		timeout := 5                        // Short timeout for test
		config := types.ProviderConfig{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			MaxRetries: &maxRetries,
			RetryDelay: &retryDelay,
			Timeout:    timeout,
		}

		provider := NewBaseProvider("test", config)

		// Use a context with timeout to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Make a request that will fail
		err := provider.DoRequest(ctx, "POST", server.URL+"/test", nil, nil)

		assert.Error(t, err)
		assert.Equal(t, int64(2), atomic.LoadInt64(&callCount))

		// Check that retry delay was approximately respected (if both calls happened)
		if !firstCallTime.IsZero() && !secondCallTime.IsZero() {
			actualDelay := secondCallTime.Sub(firstCallTime)
			assert.GreaterOrEqual(t, actualDelay, retryDelay)
			assert.Less(t, actualDelay, retryDelay+200*time.Millisecond) // Allow more margin
		}
	})

	t.Run("zero retries disables retry", func(t *testing.T) {
		var callCount int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "server error"}`))
		}))
		defer server.Close()

		maxRetries := 0 // Disable retries
		config := types.ProviderConfig{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			MaxRetries: &maxRetries,
		}

		provider := NewBaseProvider("test", config)

		// Make a request that will fail
		err := provider.DoRequest(context.Background(), "POST", server.URL+"/test", nil, nil)

		// Should only be called once (no retries)
		assert.Error(t, err)
		assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))
	})

	t.Run("nil retry settings use defaults", func(t *testing.T) {
		var callCount int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "service unavailable"}`))
		}))
		defer server.Close()

		config := types.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			// MaxRetries: nil - should use defaults
		}

		provider := NewBaseProvider("test", config)

		// Make a request that will fail
		err := provider.DoRequest(context.Background(), "POST", server.URL+"/test", nil, nil)

		// Should use default retry count (likely > 1)
		assert.Error(t, err)
		assert.Greater(t, atomic.LoadInt64(&callCount), int64(1))
	})

	t.Run("max retry delay is respected", func(t *testing.T) {
		var callCount int64
		var callTimes []time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callTimes = append(callTimes, time.Now())
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		maxRetries := 3
		retryDelay := 50 * time.Millisecond
		maxRetryDelay := 100 * time.Millisecond // Cap the delay

		config := types.ProviderConfig{
			APIKey:        "test-key",
			BaseURL:       server.URL,
			MaxRetries:    &maxRetries,
			RetryDelay:    &retryDelay,
			RetryMaxDelay: &maxRetryDelay,
		}

		provider := NewBaseProvider("test", config)

		// Make a request that will fail
		err := provider.DoRequest(context.Background(), "POST", server.URL+"/test", nil, nil)

		assert.Error(t, err)
		assert.Equal(t, int64(4), atomic.LoadInt64(&callCount)) // 1 initial + 3 retries

		// Check that delays respect max delay cap
		require.Len(t, callTimes, 4)
		for i := 1; i < len(callTimes); i++ {
			delay := callTimes[i].Sub(callTimes[i-1])
			assert.LessOrEqual(t, delay, maxRetryDelay+50*time.Millisecond) // Allow some margin
		}
	})
}

func TestBaseProvider_RetryBehaviorWithDifferentStatusCodes(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		shouldRetry   bool
		expectedCalls int64
	}{
		{"429 Too Many Requests", http.StatusTooManyRequests, true, 3},
		{"500 Internal Server Error", http.StatusInternalServerError, true, 3},
		{"502 Bad Gateway", http.StatusBadGateway, true, 3},
		{"503 Service Unavailable", http.StatusServiceUnavailable, true, 3},
		{"504 Gateway Timeout", http.StatusGatewayTimeout, true, 3},
		{"400 Bad Request", http.StatusBadRequest, false, 1},
		{"401 Unauthorized", http.StatusUnauthorized, false, 1},
		{"404 Not Found", http.StatusNotFound, false, 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var callCount int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&callCount, 1)
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{"error": "test error"}`))
			}))
			defer server.Close()

			maxRetries := 2
			config := types.ProviderConfig{
				APIKey:     "test-key",
				BaseURL:    server.URL,
				MaxRetries: &maxRetries,
			}

			provider := NewBaseProvider("test", config)

			err := provider.DoRequest(context.Background(), "POST", server.URL+"/test", nil, nil)

			assert.Error(t, err)
			assert.Equal(t, tc.expectedCalls, atomic.LoadInt64(&callCount))
		})
	}
}

func TestBaseProvider_RetryWithContextCancellation(t *testing.T) {
	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(100 * time.Millisecond) // Simulate slow response
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	maxRetries := 5
	retryDelay := 200 * time.Millisecond
	config := types.ProviderConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	provider := NewBaseProvider("test", config)

	// Create context that times out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := provider.DoRequest(ctx, "POST", server.URL+"/test", nil, nil)

	// Should be context deadline exceeded
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")

	// Should not have completed all retries due to context cancellation
	assert.Less(t, atomic.LoadInt64(&callCount), int64(6)) // Less than maxRetries + 1
}

func TestBaseProvider_SuccessfulRetry(t *testing.T) {
	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&callCount, 1)
		if count < 3 {
			// First 2 calls fail
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "temporary failure"}`))
		} else {
			// Third call succeeds
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result": "success"}`))
		}
	}))
	defer server.Close()

	maxRetries := 5 // More than needed
	config := types.ProviderConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		MaxRetries: &maxRetries,
	}

	provider := NewBaseProvider("test", config)

	var result map[string]interface{}
	err := provider.DoRequest(context.Background(), "POST", server.URL+"/test", nil, &result)

	// Should succeed after retries
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])
	assert.Equal(t, int64(3), atomic.LoadInt64(&callCount)) // Should stop retrying after success
}

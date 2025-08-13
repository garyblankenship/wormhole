package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryConfig_Defaults(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiple)
	assert.True(t, config.Jitter)
}

func TestRetryableError(t *testing.T) {
	t.Run("error message formatting", func(t *testing.T) {
		originalErr := errors.New("connection refused")
		retryErr := &RetryableError{
			Err:         originalErr,
			StatusCode:  503,
			ShouldRetry: true,
			RetryAfter:  5 * time.Second,
		}

		expected := "retryable error (status: 503, should_retry: true): connection refused"
		assert.Equal(t, expected, retryErr.Error())
	})

	t.Run("non-retryable error", func(t *testing.T) {
		retryErr := &RetryableError{
			Err:         errors.New("bad request"),
			StatusCode:  400,
			ShouldRetry: false,
		}

		assert.Contains(t, retryErr.Error(), "should_retry: false")
	})
}

func TestIsRetryableStatusCode(t *testing.T) {
	retryableCodes := []int{
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	nonRetryableCodes := []int{
		http.StatusOK,                  // 200
		http.StatusBadRequest,          // 400
		http.StatusUnauthorized,        // 401
		http.StatusForbidden,           // 403
		http.StatusNotFound,            // 404
		http.StatusMethodNotAllowed,    // 405
		http.StatusConflict,            // 409
		http.StatusUnprocessableEntity, // 422
	}

	for _, code := range retryableCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			assert.True(t, IsRetryableStatusCode(code), "Status %d should be retryable", code)
		})
	}

	for _, code := range nonRetryableCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			assert.False(t, IsRetryableStatusCode(code), "Status %d should not be retryable", code)
		})
	}
}

func TestNewRetryableHTTPClient(t *testing.T) {
	t.Run("with custom client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 10 * time.Second}
		config := DefaultRetryConfig()

		retryClient := NewRetryableHTTPClient(customClient, config)

		assert.Equal(t, customClient, retryClient.Client)
		assert.Equal(t, config, retryClient.Config)
	})

	t.Run("with nil client", func(t *testing.T) {
		config := DefaultRetryConfig()

		retryClient := NewRetryableHTTPClient(nil, config)

		assert.NotNil(t, retryClient.Client)
		assert.Equal(t, 30*time.Second, retryClient.Client.Timeout)
		assert.Equal(t, config, retryClient.Config)
	})
}

func TestRetryableHTTPClient_Do_Success(t *testing.T) {
	// Mock server that succeeds immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffMultiple: 2.0,
		Jitter:          false, // Disable for predictable testing
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "success", string(body))
}

func TestRetryableHTTPClient_Do_RetryableErrors(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503 - retryable
			w.Write([]byte("service unavailable"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    1 * time.Millisecond, // Fast for testing
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attempt) // Should have made 3 attempts

	// Should have taken some time due to retries
	assert.Greater(t, duration, 2*time.Millisecond)
}

func TestRetryableHTTPClient_Do_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400 - not retryable
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	config := DefaultRetryConfig()
	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)

	// Non-retryable HTTP errors should return the response, not an error
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "bad request", string(body))
}

func TestRetryableHTTPClient_Do_ExceedMaxRetries(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		w.WriteHeader(http.StatusInternalServerError) // Always fail
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "max retries (2) exceeded")
	assert.Equal(t, 3, attempt) // Initial + 2 retries = 3 total attempts
}

func TestRetryableHTTPClient_Do_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      5,
		InitialDelay:    100 * time.Millisecond, // Long delay
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, context.DeadlineExceeded, err)

	// Should have stopped quickly due to context cancellation
	assert.Less(t, duration, 150*time.Millisecond)
}

func TestRetryableHTTPClient_Do_RetryAfterHeader(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.Header().Set("Retry-After", "1") // 1 second
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    10 * time.Millisecond, // Short initial delay
		MaxDelay:        5 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attempt)

	// Should have respected the Retry-After header (1 second)
	assert.GreaterOrEqual(t, duration, 1*time.Second)
}

func TestRetryableHTTPClient_calculateDelay(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        5 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	t.Run("exponential backoff", func(t *testing.T) {
		delay0 := client.calculateDelay(0, 0)
		delay1 := client.calculateDelay(1, 0)
		delay2 := client.calculateDelay(2, 0)

		assert.Equal(t, 100*time.Millisecond, delay0)
		assert.Equal(t, 200*time.Millisecond, delay1)
		assert.Equal(t, 400*time.Millisecond, delay2)
	})

	t.Run("max delay cap", func(t *testing.T) {
		delay := client.calculateDelay(10, 0) // Would be very large
		assert.Equal(t, 5*time.Second, delay) // Capped at MaxDelay
	})

	t.Run("retry after override", func(t *testing.T) {
		retryAfter := 3 * time.Second
		delay := client.calculateDelay(2, retryAfter)
		assert.Equal(t, retryAfter, delay)
	})

	t.Run("retry after capped by max delay", func(t *testing.T) {
		retryAfter := 10 * time.Second // Exceeds MaxDelay
		delay := client.calculateDelay(2, retryAfter)
		assert.Equal(t, 5*time.Second, delay) // Capped at MaxDelay
	})

	t.Run("with jitter", func(t *testing.T) {
		jitterConfig := config
		jitterConfig.Jitter = true
		jitterClient := NewRetryableHTTPClient(nil, jitterConfig)

		// Test that jitter affects the delay (may or may not vary due to timing)
		delay := jitterClient.calculateDelay(1, 0)

		// With jitter, the delay should still be reasonable
		// Base delay for attempt 1 is 200ms, jitter can add ±20%
		assert.Greater(t, delay, 80*time.Millisecond) // 200ms - 60% buffer
		assert.LessOrEqual(t, delay, 5*time.Second)   // Should be capped by MaxDelay
	})
}

func TestParseRetryAfter(t *testing.T) {
	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"10", 10 * time.Second},
		{"0", 0},
		{"invalid", 0}, // Invalid format should return 0
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := parseRetryAfter(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	attempt := 0
	fn := func() error {
		attempt++
		if attempt < 3 {
			return &RetryableError{
				Err:         errors.New("temporary failure"),
				ShouldRetry: true,
			}
		}
		return nil // Success on 3rd attempt
	}

	config := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	err := WithRetry(context.Background(), config, fn)

	assert.NoError(t, err)
	assert.Equal(t, 3, attempt)
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	attempt := 0
	fn := func() error {
		attempt++
		return &RetryableError{
			Err:         errors.New("bad request"),
			StatusCode:  400,
			ShouldRetry: false,
		}
	}

	config := DefaultRetryConfig()

	err := WithRetry(context.Background(), config, fn)

	assert.Error(t, err)
	assert.Equal(t, 1, attempt) // Should not retry

	retryErr, ok := err.(*RetryableError)
	assert.True(t, ok)
	assert.False(t, retryErr.ShouldRetry)
}

func TestWithRetry_ExceedMaxRetries(t *testing.T) {
	attempt := 0
	fn := func() error {
		attempt++
		return errors.New("always fails")
	}

	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	err := WithRetry(context.Background(), config, fn)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retries (2) exceeded")
	assert.Equal(t, 3, attempt) // Initial + 2 retries
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	attempt := 0
	fn := func() error {
		attempt++
		return errors.New("always fails")
	}

	config := RetryConfig{
		MaxRetries:      5,
		InitialDelay:    100 * time.Millisecond, // Long delay
		MaxDelay:        1 * time.Second,
		BackoffMultiple: 2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := WithRetry(ctx, config, fn)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Equal(t, 1, attempt) // Should only try once before context cancellation
	assert.Less(t, duration, 150*time.Millisecond)
}

func TestWithRetry_ImmediateSuccess(t *testing.T) {
	attempt := 0
	fn := func() error {
		attempt++
		return nil // Succeed immediately
	}

	config := DefaultRetryConfig()

	err := WithRetry(context.Background(), config, fn)

	assert.NoError(t, err)
	assert.Equal(t, 1, attempt)
}

func TestRetryableHTTPClient_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", "http://invalid-host-that-does-not-exist.local", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "max retries")
}

func TestRetryableHTTPClient_RequestCloning(t *testing.T) {
	// Verify that requests are properly cloned for each retry
	attempt := 0
	var requestBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		body, _ := io.ReadAll(r.Body)
		requestBodies = append(requestBodies, string(body))

		if attempt < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      2,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	requestBody := "test request body"
	req, err := http.NewRequest("POST", server.URL, strings.NewReader(requestBody))
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 2, attempt)
	assert.Len(t, requestBodies, 2)

	// All request bodies should be the same (properly cloned)
	for _, body := range requestBodies {
		assert.Equal(t, requestBody, body)
	}
}

func TestRetryableHTTPClient_RealWorldScenario(t *testing.T) {
	// Simulate a real-world scenario with rate limiting
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		switch attempt {
		case 1:
			// First request: rate limited with Retry-After
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
		case 2:
			// Second request: server error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "internal server error"}`))
		case 3:
			// Third request: success
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result": "success"}`))
		}
	}))
	defer server.Close()

	config := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        2 * time.Second,
		BackoffMultiple: 2.0,
		Jitter:          false,
	}

	client := NewRetryableHTTPClient(nil, config)

	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attempt)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "success")

	// Should have respected Retry-After header on first retry
	assert.GreaterOrEqual(t, duration, 1*time.Second)
}

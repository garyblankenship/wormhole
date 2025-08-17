package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", config.MaxRetries)
	}
	if config.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay=1s, got %v", config.InitialDelay)
	}
	if config.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay=30s, got %v", config.MaxDelay)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier=2.0, got %f", config.Multiplier)
	}
	if !config.Jitter {
		t.Error("Expected Jitter=true")
	}
	if config.RetryableFunc == nil {
		t.Error("Expected RetryableFunc to be set")
	}

	// Test default RetryableFunc
	if !config.RetryableFunc(errors.New("test error")) {
		t.Error("Expected default RetryableFunc to return true for any error")
	}
	if config.RetryableFunc(nil) {
		t.Error("Expected default RetryableFunc to return false for nil error")
	}
}

func TestRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("success_on_first_try", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:   3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}

		callCount := 0
		fn := func() error {
			callCount++
			return nil // Success on first call
		}

		err := Retry(ctx, config, fn)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("Expected 1 call, got %d", callCount)
		}
	})

	t.Run("success_after_retries", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:   3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}

		callCount := 0
		fn := func() error {
			callCount++
			if callCount < 3 {
				return errors.New("temporary failure")
			}
			return nil // Success on third call
		}

		err := Retry(ctx, config, fn)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if callCount != 3 {
			t.Errorf("Expected 3 calls, got %d", callCount)
		}
	})

	t.Run("exhausts_all_retries", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:   2,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		}

		callCount := 0
		expectedErr := errors.New("persistent failure")
		fn := func() error {
			callCount++
			return expectedErr
		}

		err := Retry(ctx, config, fn)
		if err != expectedErr {
			t.Errorf("Expected persistent failure error, got %v", err)
		}
		if callCount != 3 { // Initial attempt + 2 retries
			t.Errorf("Expected 3 calls (1 initial + 2 retries), got %d", callCount)
		}
	})

	t.Run("non_retryable_error", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:   3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			RetryableFunc: func(err error) bool {
				return err.Error() != "non-retryable"
			},
		}

		callCount := 0
		nonRetryableErr := errors.New("non-retryable")
		fn := func() error {
			callCount++
			return nonRetryableErr
		}

		err := Retry(ctx, config, fn)
		if err != nonRetryableErr {
			t.Errorf("Expected non-retryable error, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("Expected 1 call (no retries for non-retryable), got %d", callCount)
		}
	})

	t.Run("context_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		config := RetryConfig{
			MaxRetries:   10,
			InitialDelay: 10 * time.Millisecond, // Longer than context timeout
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}

		callCount := 0
		fn := func() error {
			callCount++
			return errors.New("failure")
		}

		err := Retry(ctx, config, fn)
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
		// Should have at least one call, possibly two
		if callCount == 0 {
			t.Error("Expected at least one call before context cancellation")
		}
	})

	t.Run("jitter_applied", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:   2,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       true,
		}

		callCount := 0
		startTime := time.Now()
		fn := func() error {
			callCount++
			return errors.New("failure")
		}

		Retry(ctx, config, fn)
		duration := time.Since(startTime)

		// With jitter, total time should be at least base delay time but vary
		expectedMinTime := 10 * time.Millisecond // First retry delay
		if duration < expectedMinTime {
			t.Errorf("Expected at least %v delay, got %v", expectedMinTime, duration)
		}
	})
}

func TestRetryMiddleware(t *testing.T) {
	config := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	t.Run("successful_request", func(t *testing.T) {
		callCount := 0
		mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			callCount++
			return "success", nil
		}

		middleware := RetryMiddleware(config)
		wrappedHandler := middleware(mockHandler)

		ctx := context.Background()
		resp, err := wrappedHandler(ctx, "test request")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if resp != "success" {
			t.Errorf("Expected 'success', got %v", resp)
		}
		if callCount != 1 {
			t.Errorf("Expected 1 call for successful request, got %d", callCount)
		}
	})

	t.Run("retry_until_success", func(t *testing.T) {
		callCount := 0
		mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			callCount++
			if callCount < 2 {
				return nil, errors.New("temporary failure")
			}
			return "eventual success", nil
		}

		middleware := RetryMiddleware(config)
		wrappedHandler := middleware(mockHandler)

		ctx := context.Background()
		resp, err := wrappedHandler(ctx, "test request")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if resp != "eventual success" {
			t.Errorf("Expected 'eventual success', got %v", resp)
		}
		if callCount != 2 {
			t.Errorf("Expected 2 calls, got %d", callCount)
		}
	})

	t.Run("exhausts_retries", func(t *testing.T) {
		callCount := 0
		expectedErr := errors.New("persistent failure")
		mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			callCount++
			return nil, expectedErr
		}

		middleware := RetryMiddleware(config)
		wrappedHandler := middleware(mockHandler)

		ctx := context.Background()
		resp, err := wrappedHandler(ctx, "test request")

		if err != expectedErr {
			t.Errorf("Expected persistent failure, got %v", err)
		}
		if resp != nil {
			t.Errorf("Expected nil response, got %v", resp)
		}
		if callCount != 3 { // 1 initial + 2 retries
			t.Errorf("Expected 3 calls, got %d", callCount)
		}
	})
}

func TestExponentialBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Second

	// Test exponential growth
	delay0 := ExponentialBackoff(0, base, max)
	delay1 := ExponentialBackoff(1, base, max)
	delay2 := ExponentialBackoff(2, base, max)

	expected0 := base     // 100ms * 2^0 = 100ms
	expected1 := base * 2 // 100ms * 2^1 = 200ms
	expected2 := base * 4 // 100ms * 2^2 = 400ms

	if delay0 != expected0 {
		t.Errorf("Expected delay0=%v, got %v", expected0, delay0)
	}
	if delay1 != expected1 {
		t.Errorf("Expected delay1=%v, got %v", expected1, delay1)
	}
	if delay2 != expected2 {
		t.Errorf("Expected delay2=%v, got %v", expected2, delay2)
	}

	// Test max cap
	delayLarge := ExponentialBackoff(20, base, max)
	if delayLarge != max {
		t.Errorf("Expected delay to be capped at %v, got %v", max, delayLarge)
	}
}

func TestLinearBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Second

	delay0 := LinearBackoff(0, base, max)
	delay1 := LinearBackoff(1, base, max)
	delay2 := LinearBackoff(2, base, max)

	expected0 := base     // 100ms * (0+1) = 100ms
	expected1 := base * 2 // 100ms * (1+1) = 200ms
	expected2 := base * 3 // 100ms * (2+1) = 300ms

	if delay0 != expected0 {
		t.Errorf("Expected delay0=%v, got %v", expected0, delay0)
	}
	if delay1 != expected1 {
		t.Errorf("Expected delay1=%v, got %v", expected1, delay1)
	}
	if delay2 != expected2 {
		t.Errorf("Expected delay2=%v, got %v", expected2, delay2)
	}

	// Test max cap
	delayLarge := LinearBackoff(100, base, max)
	if delayLarge != max {
		t.Errorf("Expected delay to be capped at %v, got %v", max, delayLarge)
	}
}

func TestFibonacciBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Second

	delay0 := FibonacciBackoff(0, base, max)
	delay1 := FibonacciBackoff(1, base, max)
	delay2 := FibonacciBackoff(2, base, max)
	delay3 := FibonacciBackoff(3, base, max)

	// Fibonacci: 1, 1, 2, 3, 5, 8, 13, ...
	expected0 := base     // 100ms * 1 = 100ms
	expected1 := base     // 100ms * 1 = 100ms
	expected2 := base * 2 // 100ms * 2 = 200ms
	expected3 := base * 3 // 100ms * 3 = 300ms

	if delay0 != expected0 {
		t.Errorf("Expected delay0=%v, got %v", expected0, delay0)
	}
	if delay1 != expected1 {
		t.Errorf("Expected delay1=%v, got %v", expected1, delay1)
	}
	if delay2 != expected2 {
		t.Errorf("Expected delay2=%v, got %v", expected2, delay2)
	}
	if delay3 != expected3 {
		t.Errorf("Expected delay3=%v, got %v", expected3, delay3)
	}
}

func TestRetryableError(t *testing.T) {
	originalErr := errors.New("network timeout")
	retryableErr := RetryableError{Err: originalErr}

	// Test Error method
	if retryableErr.Error() != originalErr.Error() {
		t.Errorf("Expected '%s', got '%s'", originalErr.Error(), retryableErr.Error())
	}

	// Test IsRetryable
	if !IsRetryable(retryableErr) {
		t.Error("Expected RetryableError to be retryable")
	}

	// Test with regular error
	regularErr := errors.New("regular error")
	if IsRetryable(regularErr) {
		t.Error("Expected regular error to not be retryable")
	}

	// Test with wrapped RetryableError
	wrappedErr := errors.New("wrapped: " + retryableErr.Error())
	if IsRetryable(wrappedErr) {
		t.Error("Expected wrapped error to not be automatically retryable")
	}
}

func TestAdaptiveRetry(t *testing.T) {
	config := AdaptiveRetryConfig{
		RetryConfig: RetryConfig{
			MaxRetries:   5,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		},
		SuccessThreshold: 3,
		FailureThreshold: 2,
	}

	retry := NewAdaptiveRetry(config)
	ctx := context.Background()

	t.Run("adapts_to_success", func(t *testing.T) {
		// First increase the delay by having failures
		failCount := 0
		fn1 := func() error {
			failCount++
			if failCount <= config.FailureThreshold {
				return errors.New("initial failure")
			}
			return nil
		}

		// This should increase the delay
		retry.Execute(ctx, fn1)
		initialIncreasedDelay := retry.currentDelay

		// Now record several successes to reduce delay
		for i := 0; i < config.SuccessThreshold; i++ {
			fn := func() error { return nil }
			err := retry.Execute(ctx, fn)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		}

		// Current delay should be reduced from the increased value
		if retry.currentDelay >= initialIncreasedDelay {
			t.Errorf("Expected delay to be reduced from %v, got %v", initialIncreasedDelay, retry.currentDelay)
		}
	})

	t.Run("adapts_to_failure", func(t *testing.T) {
		retry := NewAdaptiveRetry(config) // Fresh instance

		// Force failures to increase delay
		callCount := 0
		fn := func() error {
			callCount++
			if callCount <= config.FailureThreshold {
				return errors.New("failure")
			}
			return nil // Success after threshold failures
		}

		err := retry.Execute(ctx, fn)
		if err != nil {
			t.Fatalf("Expected eventual success, got %v", err)
		}

		// Delay should be increased after consecutive failures
		if retry.currentDelay <= config.InitialDelay {
			t.Errorf("Expected delay to be increased above %v, got %v", config.InitialDelay, retry.currentDelay)
		}
	})
}

func TestAdaptiveRetryMiddleware(t *testing.T) {
	config := AdaptiveRetryConfig{
		RetryConfig: RetryConfig{
			MaxRetries:   3,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
		},
		SuccessThreshold: 2,
		FailureThreshold: 2,
	}

	callCount := 0
	mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		if callCount < 2 {
			return nil, errors.New("failure")
		}
		return "success", nil
	}

	middleware := AdaptiveRetryMiddleware(config)
	wrappedHandler := middleware(mockHandler)

	ctx := context.Background()
	resp, err := wrappedHandler(ctx, "test")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}
}

package config

import (
	"os"
	"testing"
	"time"
)

func TestGetDefaultHTTPTimeout(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackHTTPTimeout,
		},
		{
			name:     "valid environment variable - seconds",
			envValue: "45s",
			expected: 45 * time.Second,
		},
		{
			name:     "valid environment variable - minutes",
			envValue: "10m",
			expected: 10 * time.Minute,
		},
		{
			name:     "valid environment variable - zero",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "invalid environment variable",
			envValue: "invalid",
			expected: FallbackHTTPTimeout,
		},
		{
			name:     "valid environment variable - milliseconds",
			envValue: "500ms",
			expected: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_DEFAULT_TIMEOUT")
			defer os.Setenv("WORMHOLE_DEFAULT_TIMEOUT", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_DEFAULT_TIMEOUT")
			} else {
				os.Setenv("WORMHOLE_DEFAULT_TIMEOUT", tt.envValue)
			}

			result := GetDefaultHTTPTimeout()
			if result != tt.expected {
				t.Errorf("GetDefaultHTTPTimeout() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetDefaultMaxRetries(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackMaxRetries,
		},
		{
			name:     "valid environment variable - positive",
			envValue: "5",
			expected: 5,
		},
		{
			name:     "valid environment variable - zero",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "invalid environment variable - non-numeric",
			envValue: "invalid",
			expected: FallbackMaxRetries,
		},
		{
			name:     "invalid environment variable - negative",
			envValue: "-1",
			expected: FallbackMaxRetries,
		},
		{
			name:     "valid environment variable - large number",
			envValue: "100",
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_MAX_RETRIES")
			defer os.Setenv("WORMHOLE_MAX_RETRIES", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_MAX_RETRIES")
			} else {
				os.Setenv("WORMHOLE_MAX_RETRIES", tt.envValue)
			}

			result := GetDefaultMaxRetries()
			if result != tt.expected {
				t.Errorf("GetDefaultMaxRetries() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetDefaultMaxDelay(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackMaxDelay,
		},
		{
			name:     "valid environment variable - seconds",
			envValue: "60s",
			expected: 60 * time.Second,
		},
		{
			name:     "valid environment variable - minutes",
			envValue: "2m",
			expected: 2 * time.Minute,
		},
		{
			name:     "valid environment variable - zero",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "invalid environment variable",
			envValue: "invalid",
			expected: FallbackMaxDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_MAX_RETRY_DELAY")
			defer os.Setenv("WORMHOLE_MAX_RETRY_DELAY", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_MAX_RETRY_DELAY")
			} else {
				os.Setenv("WORMHOLE_MAX_RETRY_DELAY", tt.envValue)
			}

			result := GetDefaultMaxDelay()
			if result != tt.expected {
				t.Errorf("GetDefaultMaxDelay() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetDefaultInitialDelay(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackInitialDelay,
		},
		{
			name:     "valid environment variable - seconds",
			envValue: "2s",
			expected: 2 * time.Second,
		},
		{
			name:     "valid environment variable - milliseconds",
			envValue: "500ms",
			expected: 500 * time.Millisecond,
		},
		{
			name:     "valid environment variable - zero",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "invalid environment variable",
			envValue: "invalid",
			expected: FallbackInitialDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_INITIAL_RETRY_DELAY")
			defer os.Setenv("WORMHOLE_INITIAL_RETRY_DELAY", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_INITIAL_RETRY_DELAY")
			} else {
				os.Setenv("WORMHOLE_INITIAL_RETRY_DELAY", tt.envValue)
			}

			result := GetDefaultInitialDelay()
			if result != tt.expected {
				t.Errorf("GetDefaultInitialDelay() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetDefaultCircuitBreakerTimeout(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackCircuitBreakerTimeout,
		},
		{
			name:     "valid environment variable - seconds",
			envValue: "45s",
			expected: 45 * time.Second,
		},
		{
			name:     "valid environment variable - minutes",
			envValue: "1m",
			expected: 1 * time.Minute,
		},
		{
			name:     "invalid environment variable",
			envValue: "invalid",
			expected: FallbackCircuitBreakerTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_CIRCUIT_BREAKER_TIMEOUT")
			defer os.Setenv("WORMHOLE_CIRCUIT_BREAKER_TIMEOUT", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_CIRCUIT_BREAKER_TIMEOUT")
			} else {
				os.Setenv("WORMHOLE_CIRCUIT_BREAKER_TIMEOUT", tt.envValue)
			}

			result := GetDefaultCircuitBreakerTimeout()
			if result != tt.expected {
				t.Errorf("GetDefaultCircuitBreakerTimeout() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetDefaultHealthCheckInterval(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: FallbackHealthCheckInterval,
		},
		{
			name:     "valid environment variable - seconds",
			envValue: "60s",
			expected: 60 * time.Second,
		},
		{
			name:     "valid environment variable - minutes",
			envValue: "5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "invalid environment variable",
			envValue: "invalid",
			expected: FallbackHealthCheckInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := os.Getenv("WORMHOLE_HEALTH_CHECK_INTERVAL")
			defer os.Setenv("WORMHOLE_HEALTH_CHECK_INTERVAL", original)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("WORMHOLE_HEALTH_CHECK_INTERVAL")
			} else {
				os.Setenv("WORMHOLE_HEALTH_CHECK_INTERVAL", tt.envValue)
			}

			result := GetDefaultHealthCheckInterval()
			if result != tt.expected {
				t.Errorf("GetDefaultHealthCheckInterval() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify that critical constants are properly defined
	if FallbackHTTPTimeout != 300*time.Second {
		t.Errorf("FallbackHTTPTimeout = %v, expected 300s", FallbackHTTPTimeout)
	}

	if FallbackMaxRetries != 3 {
		t.Errorf("FallbackMaxRetries = %v, expected 3", FallbackMaxRetries)
	}

	if FallbackInitialDelay != 1*time.Second {
		t.Errorf("FallbackInitialDelay = %v, expected 1s", FallbackInitialDelay)
	}

	if FallbackMaxDelay != 30*time.Second {
		t.Errorf("FallbackMaxDelay = %v, expected 30s", FallbackMaxDelay)
	}

	if DefaultBackoffMultiple != 2.0 {
		t.Errorf("DefaultBackoffMultiple = %v, expected 2.0", DefaultBackoffMultiple)
	}

	if !DefaultJitterEnabled {
		t.Error("DefaultJitterEnabled should be true")
	}

	if UnlimitedTimeout != 0 {
		t.Errorf("UnlimitedTimeout = %v, expected 0", UnlimitedTimeout)
	}
}

func TestBackwardsCompatibilityAliases(t *testing.T) {
	// Verify backwards compatibility constants
	if DefaultMaxRetries != FallbackMaxRetries {
		t.Errorf("DefaultMaxRetries = %v, should equal FallbackMaxRetries = %v",
			DefaultMaxRetries, FallbackMaxRetries)
	}

	if DefaultInitialDelay != FallbackInitialDelay {
		t.Errorf("DefaultInitialDelay = %v, should equal FallbackInitialDelay = %v",
			DefaultInitialDelay, FallbackInitialDelay)
	}

	if DefaultMaxDelay != FallbackMaxDelay {
		t.Errorf("DefaultMaxDelay = %v, should equal FallbackMaxDelay = %v",
			DefaultMaxDelay, FallbackMaxDelay)
	}
}

// Test concurrent access to environment variable functions
func TestConcurrentAccess(t *testing.T) {
	// Set a consistent environment variable
	os.Setenv("WORMHOLE_DEFAULT_TIMEOUT", "60s")
	defer os.Unsetenv("WORMHOLE_DEFAULT_TIMEOUT")

	// Test concurrent access doesn't cause race conditions
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			result := GetDefaultHTTPTimeout()
			if result != 60*time.Second {
				t.Errorf("Expected 60s, got %v", result)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

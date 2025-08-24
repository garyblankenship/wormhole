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
			expected: FALLBACK_DefaultHTTPTimeout,
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
			expected: FALLBACK_DefaultHTTPTimeout,
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
			expected: FALLBACK_DefaultMaxRetries,
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
			expected: FALLBACK_DefaultMaxRetries,
		},
		{
			name:     "invalid environment variable - negative",
			envValue: "-1",
			expected: FALLBACK_DefaultMaxRetries,
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
			expected: FALLBACK_DefaultMaxDelay,
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
			expected: FALLBACK_DefaultMaxDelay,
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
			expected: FALLBACK_DefaultInitialDelay,
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
			expected: FALLBACK_DefaultInitialDelay,
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
			expected: FALLBACK_DefaultCircuitBreakerTimeout,
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
			expected: FALLBACK_DefaultCircuitBreakerTimeout,
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
			expected: FALLBACK_DefaultHealthCheckInterval,
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
			expected: FALLBACK_DefaultHealthCheckInterval,
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
	if FALLBACK_DefaultHTTPTimeout != 300*time.Second {
		t.Errorf("FALLBACK_DefaultHTTPTimeout = %v, expected 300s", FALLBACK_DefaultHTTPTimeout)
	}

	if FALLBACK_DefaultMaxRetries != 3 {
		t.Errorf("FALLBACK_DefaultMaxRetries = %v, expected 3", FALLBACK_DefaultMaxRetries)
	}

	if FALLBACK_DefaultInitialDelay != 1*time.Second {
		t.Errorf("FALLBACK_DefaultInitialDelay = %v, expected 1s", FALLBACK_DefaultInitialDelay)
	}

	if FALLBACK_DefaultMaxDelay != 30*time.Second {
		t.Errorf("FALLBACK_DefaultMaxDelay = %v, expected 30s", FALLBACK_DefaultMaxDelay)
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
	if DefaultMaxRetries != FALLBACK_DefaultMaxRetries {
		t.Errorf("DefaultMaxRetries = %v, should equal FALLBACK_DefaultMaxRetries = %v", 
			DefaultMaxRetries, FALLBACK_DefaultMaxRetries)
	}

	if DefaultInitialDelay != FALLBACK_DefaultInitialDelay {
		t.Errorf("DefaultInitialDelay = %v, should equal FALLBACK_DefaultInitialDelay = %v",
			DefaultInitialDelay, FALLBACK_DefaultInitialDelay)
	}

	if DefaultMaxDelay != FALLBACK_DefaultMaxDelay {
		t.Errorf("DefaultMaxDelay = %v, should equal FALLBACK_DefaultMaxDelay = %v",
			DefaultMaxDelay, FALLBACK_DefaultMaxDelay)
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
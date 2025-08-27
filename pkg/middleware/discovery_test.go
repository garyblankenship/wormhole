package middleware

import (
	"testing"
)

func TestAvailableMiddleware(t *testing.T) {
	middlewares := AvailableMiddleware()

	// Should have at least the core middleware
	expectedMiddleware := []string{
		"CacheMiddleware",
		"CircuitBreakerMiddleware",
		"RateLimitMiddleware",
		"LoggingMiddleware",
		"MetricsMiddleware",
		"TimeoutMiddleware",
	}

	// Create a map for easier lookup
	foundMiddleware := make(map[string]bool)
	for _, mw := range middlewares {
		foundMiddleware[mw.Name] = true

		// Verify each middleware has required fields
		if mw.Name == "" {
			t.Errorf("Middleware missing Name field")
		}
		if mw.Purpose == "" {
			t.Errorf("Middleware %s missing Purpose field", mw.Name)
		}
		if mw.Example == "" {
			t.Errorf("Middleware %s missing Example field", mw.Name)
		}
		if mw.ConfigType == "" {
			t.Errorf("Middleware %s missing ConfigType field", mw.Name)
		}
	}

	// Verify expected middleware are present
	for _, expected := range expectedMiddleware {
		if !foundMiddleware[expected] {
			t.Errorf("Expected middleware %s not found in AvailableMiddleware()", expected)
		}
	}

	// Should have reasonable number of middleware
	if len(middlewares) < 6 {
		t.Errorf("Expected at least 6 middleware, got %d", len(middlewares))
	}
}

func TestMiddlewareInfoStructure(t *testing.T) {
	middlewares := AvailableMiddleware()

	// Test specific examples for correctness
	for _, mw := range middlewares {
		switch mw.Name {
		case "CacheMiddleware":
			if mw.ConfigType != "CacheConfig" {
				t.Errorf("CacheMiddleware should have ConfigType 'CacheConfig', got '%s'", mw.ConfigType)
			}

		case "TimeoutMiddleware":
			expectedConfig := "timeout time.Duration"
			if mw.ConfigType != expectedConfig {
				t.Errorf("TimeoutMiddleware should have ConfigType '%s', got '%s'", expectedConfig, mw.ConfigType)
			}
		}
	}
}

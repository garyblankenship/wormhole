package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
)

func TestMiddlewareDiscovery(t *testing.T) {
	// Test that middleware discovery returns expected information
	middlewares := middleware.AvailableMiddleware()

	// Should have the key middleware from DX improvements
	expectedMiddleware := map[string]bool{
		"RetryMiddleware":          true,
		"CacheMiddleware":          true,
		"CircuitBreakerMiddleware": true,
		"RateLimitMiddleware":      true,
		"LoggingMiddleware":        true,
		"MetricsMiddleware":        true,
		"TimeoutMiddleware":        true,
	}

	found := make(map[string]bool)
	for _, mw := range middlewares {
		found[mw.Name] = true

		// Verify examples contain expected patterns
		if mw.Name == "RetryMiddleware" {
			if !strings.Contains(mw.Example, "DefaultRetryConfig()") {
				t.Errorf("RetryMiddleware example should mention DefaultRetryConfig(), got: %s", mw.Example)
			}
		}

		if mw.Name == "CacheMiddleware" {
			if !strings.Contains(mw.Example, "CacheConfig") {
				t.Errorf("CacheMiddleware example should mention CacheConfig, got: %s", mw.Example)
			}
		}
	}

	// Verify all expected middleware are present
	for name := range expectedMiddleware {
		if !found[name] {
			t.Errorf("Expected middleware %s not found", name)
		}
	}
}

func TestCacheConfigurationPattern(t *testing.T) {
	// Test the "correct way" pattern from DX improvements
	cache := middleware.NewMemoryCache(100)
	config := middleware.CacheConfig{
		Cache: cache,
		TTL:   5 * time.Minute,
	}

	if config.Cache == nil {
		t.Error("Expected cache to be set")
	}
	if config.TTL != 5*time.Minute {
		t.Errorf("Expected TTL to be 5 minutes, got %v", config.TTL)
	}

	// Test that the middleware can be created with this config
	cacheMW := middleware.CacheMiddleware(config)
	if cacheMW == nil {
		t.Error("Expected middleware to be created successfully")
	}
}

func TestRetryConfigurationPattern(t *testing.T) {
	// Test DefaultRetryConfig usage (recommended pattern)
	defaultConfig := middleware.DefaultRetryConfig()
	if defaultConfig.MaxRetries == 0 {
		t.Error("Expected default config to have non-zero MaxRetries")
	}
	if defaultConfig.InitialDelay == 0 {
		t.Error("Expected default config to have non-zero InitialDelay")
	}

	// Test custom configuration pattern
	customConfig := middleware.RetryConfig{
		MaxRetries:   5,
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}

	if customConfig.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", customConfig.MaxRetries)
	}
	if customConfig.InitialDelay != 2*time.Second {
		t.Errorf("Expected InitialDelay=2s, got %v", customConfig.InitialDelay)
	}

	// Test that middleware can be created with both patterns
	defaultMW := middleware.RetryMiddleware(defaultConfig)
	customMW := middleware.RetryMiddleware(customConfig)

	if defaultMW == nil || customMW == nil {
		t.Error("Expected both middleware to be created successfully")
	}
}

func TestProductionMiddlewareStack(t *testing.T) {
	// Test the production stack pattern from DX improvements
	cache := middleware.NewMemoryCache(100)
	cacheConfig := middleware.CacheConfig{
		Cache: cache,
		TTL:   5 * time.Minute,
	}

	// Verify all middleware can be created (the stack from the example)
	middlewares := []middleware.Middleware{
		middleware.RetryMiddleware(middleware.DefaultRetryConfig()),
		middleware.CircuitBreakerMiddleware(5, 30*time.Second),
		middleware.RateLimitMiddleware(100),
		middleware.CacheMiddleware(cacheConfig),
		middleware.TimeoutMiddleware(60 * time.Second),
	}

	if len(middlewares) != 5 {
		t.Errorf("Expected 5 middleware in production stack, got %d", len(middlewares))
	}

	// Test that they can be chained
	chain := middleware.NewChain(middlewares...)
	if chain == nil {
		t.Error("Expected middleware chain to be created successfully")
	}
}

func TestDXImprovementPatterns(t *testing.T) {
	// Test patterns mentioned in the DX improvements

	// 1. Test middleware discovery (no more source diving)
	middlewares := middleware.AvailableMiddleware()
	if len(middlewares) == 0 {
		t.Error("Expected middleware discovery to return available middleware")
	}

	// 2. Test clear configuration patterns
	// Cache with TTL
	ttlCache := middleware.NewTTLCache(100, 5*time.Minute)
	if ttlCache == nil {
		t.Error("Expected TTL cache to be created")
	}

	// Memory cache
	memCache := middleware.NewMemoryCache(100)
	if memCache == nil {
		t.Error("Expected memory cache to be created")
	}

	// LRU cache
	lruCache := middleware.NewLRUCache(100)
	if lruCache == nil {
		t.Error("Expected LRU cache to be created")
	}

	// 3. Test backoff algorithms (documented options)
	exponential := middleware.ExponentialBackoff(2, 100*time.Millisecond, 5*time.Second)
	linear := middleware.LinearBackoff(2, 100*time.Millisecond, 5*time.Second)
	fibonacci := middleware.FibonacciBackoff(2, 100*time.Millisecond, 5*time.Second)

	// All should return reasonable values
	if exponential == 0 || linear == 0 || fibonacci == 0 {
		t.Errorf("Expected non-zero backoff values, got exp=%v, lin=%v, fib=%v",
			exponential, linear, fibonacci)
	}
}

// Integration test that verifies the example patterns actually work
func TestDXImprovementsIntegration(t *testing.T) {
	// This test verifies that the patterns shown in the DX improvements
	// actually work end-to-end

	// Create cache following the documented pattern
	cache := middleware.NewMemoryCache(10)
	cacheConfig := middleware.CacheConfig{
		Cache: cache,
		TTL:   1 * time.Minute,
	}
	cacheMW := middleware.CacheMiddleware(cacheConfig)

	// Create retry following the documented pattern
	retryConfig := middleware.DefaultRetryConfig()
	retryMW := middleware.RetryMiddleware(retryConfig)

	// Create a complete middleware stack
	chain := middleware.NewChain(
		retryMW,
		middleware.CircuitBreakerMiddleware(3, 10*time.Second),
		middleware.RateLimitMiddleware(50),
		cacheMW,
		middleware.TimeoutMiddleware(30*time.Second),
	)

	// Verify the chain works
	callCount := 0
	mockHandler := func(ctx context.Context, req any) (any, error) {
		callCount++
		return "success", nil
	}

	wrappedHandler := chain.Apply(mockHandler)

	// Test the wrapped handler
	ctx := context.Background()
	resp, err := wrappedHandler(ctx, "request")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

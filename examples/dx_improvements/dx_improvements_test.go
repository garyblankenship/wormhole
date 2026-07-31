package main

import (
	"context"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestMiddlewareDiscovery(t *testing.T) {
	t.Parallel()
	found := make(map[string]middleware.MiddlewareInfo)
	for _, item := range middleware.AvailableMiddleware() {
		found[item.Name] = item
	}
	for _, name := range []string{
		"NewTypedCacheMiddleware",
		"NewTypedCircuitBreakerMiddleware",
		"NewTypedRateLimitMiddleware",
		"NewTypedLoggingMiddleware",
		"NewTypedMetricsMiddleware",
		"NewTypedTimeoutMiddleware",
	} {
		if _, ok := found[name]; !ok {
			t.Errorf("AvailableMiddleware missing %s", name)
		}
	}
	if item := found["NewTypedCacheMiddleware"]; item.ConfigType != "CacheConfig" {
		t.Errorf("typed cache config type = %q, want CacheConfig", item.ConfigType)
	}
}

func TestCacheConfigurationPattern(t *testing.T) {
	t.Parallel()
	cache := middleware.NewMemoryCache(100)
	t.Cleanup(func() { _ = cache.Close() })
	config := middleware.CacheConfig{Cache: cache, TTL: 5 * time.Minute}
	if config.Cache == nil || config.TTL != 5*time.Minute {
		t.Fatalf("CacheConfig = %#v", config)
	}
	if middleware.NewTypedCacheMiddleware(config) == nil {
		t.Fatal("NewTypedCacheMiddleware returned nil")
	}
}

func TestProviderConfigurationReplacements(t *testing.T) {
	t.Parallel()
	maxRetries := 5
	retryDelay := 2 * time.Second
	retryMaxDelay := 30 * time.Second
	config := types.ProviderConfig{MaxRetries: &maxRetries, RetryDelay: &retryDelay, RetryMaxDelay: &retryMaxDelay}
	if config.MaxRetries == nil || *config.MaxRetries != 5 || config.RetryDelay == nil || *config.RetryDelay != 2*time.Second || config.RetryMaxDelay == nil || *config.RetryMaxDelay != 30*time.Second {
		t.Fatalf("ProviderConfig retry replacement = %#v", config)
	}
	// Fallback belongs on the typed text builder (WithFallback/WithProviderFallback),
	// and adaptive concurrency remains a Wormhole client capability, not middleware.
	if middleware.NewTypedRateLimitMiddleware(1) == nil {
		t.Fatal("typed rate limit middleware was not constructed")
	}
}

func TestProductionMiddlewareStack(t *testing.T) {
	t.Parallel()
	cache := middleware.NewMemoryCache(100)
	t.Cleanup(func() { _ = cache.Close() })
	stack := []types.ProviderMiddleware{
		middleware.NewTypedCircuitBreakerMiddleware(5, 30*time.Second),
		middleware.NewTypedRateLimitMiddleware(100),
		middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: 5 * time.Minute}),
		middleware.NewTypedTimeoutMiddleware(60 * time.Second),
	}
	if len(stack) != 4 {
		t.Fatalf("production stack length = %d, want 4", len(stack))
	}
	if types.NewProviderChain(stack...) == nil {
		t.Fatal("NewProviderChain returned nil")
	}
}

func TestDXImprovementPatterns(t *testing.T) {
	t.Parallel()
	if len(middleware.AvailableMiddleware()) == 0 {
		t.Fatal("middleware discovery returned no entries")
	}
	for _, cache := range []middleware.Cache{middleware.NewTTLCache(100, 5*time.Minute), middleware.NewMemoryCache(100), middleware.NewLRUCache(100)} {
		if cache == nil {
			t.Fatal("cache constructor returned nil")
		}
		if err := cache.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDXImprovementsIntegration(t *testing.T) {
	t.Parallel()
	cache := middleware.NewMemoryCache(10)
	t.Cleanup(func() { _ = cache.Close() })
	chain := types.NewProviderChain(
		middleware.NewTypedCircuitBreakerMiddleware(3, 10*time.Second),
		middleware.NewTypedRateLimitMiddleware(50),
		middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: time.Minute}),
		middleware.NewTypedTimeoutMiddleware(30*time.Second),
	)
	var calls int
	handler := chain.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls++
		return &types.TextResponse{Text: "success"}, nil
	})
	request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage("request")}}
	response, err := handler(context.Background(), request)
	if err != nil || response.Text != "success" || calls != 1 {
		t.Fatalf("typed DX stack = (%#v, %v), calls = %d", response, err, calls)
	}
}

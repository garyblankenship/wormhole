package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Test constants
const (
	testValue1   = "value1"
	testResponse = "response"
)

func TestMemoryCache(t *testing.T) {
	cache := NewMemoryCache(3)

	// Test Set and Get
	cache.Set("key1", testValue1, 1*time.Hour)
	if value, found := cache.Get("key1"); !found || value != testValue1 {
		t.Errorf("Expected to find 'value1', got %v, found: %t", value, found)
	}

	// Test non-existent key
	if value, found := cache.Get("nonexistent"); found {
		t.Errorf("Expected not to find key, but got %v", value)
	}

	// Test TTL expiration
	cache.Set("expired", "value", 1*time.Millisecond)
	time.Sleep(2 * time.Millisecond)
	if _, found := cache.Get("expired"); found {
		t.Error("Expected expired key to not be found")
	}

	// Test capacity eviction
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)
	cache.Set("key4", "value4", 1*time.Hour) // This should evict oldest

	// At least one of the first 3 should be evicted
	found1 := false
	found2 := false
	found3 := false
	if _, found := cache.Get("key1"); found {
		found1 = true
	}
	if _, found := cache.Get("key2"); found {
		found2 = true
	}
	if _, found := cache.Get("key3"); found {
		found3 = true
	}

	foundCount := 0
	if found1 {
		foundCount++
	}
	if found2 {
		foundCount++
	}
	if found3 {
		foundCount++
	}

	if foundCount > 2 {
		t.Errorf("Expected at most 2 of first 3 keys to remain, but found %d", foundCount)
	}

	// key4 should always be present (newest)
	if _, found := cache.Get("key4"); !found {
		t.Error("Expected key4 to be found (newest)")
	}

	// Test Delete
	cache.Delete("key4")
	if _, found := cache.Get("key4"); found {
		t.Error("Expected key4 to be deleted")
	}

	// Test Clear
	cache.Clear()
	if _, found := cache.Get("key1"); found {
		t.Error("Expected cache to be empty after Clear")
	}
}

func TestTTLCache(t *testing.T) {
	cache := NewTTLCache(10, 50*time.Millisecond)

	// Test SetDefault uses default TTL
	cache.SetDefault("key1", "value1")
	if _, found := cache.Get("key1"); !found {
		t.Error("Expected to find key1 immediately after SetDefault")
	}

	// Test that default TTL expires
	time.Sleep(60 * time.Millisecond)
	if _, found := cache.Get("key1"); found {
		t.Error("Expected key1 to be expired after default TTL")
	}
}

func TestLRUCache(t *testing.T) {
	cache := NewLRUCache(2)

	// Test basic set and get
	cache.Set("key1", "value1", 0) // TTL not used in LRU
	cache.Set("key2", "value2", 0)

	if value, found := cache.Get("key1"); !found || value != "value1" {
		t.Errorf("Expected 'value1', got %v, found: %t", value, found)
	}

	// key1 is now most recent, add key3 to evict key2
	cache.Set("key3", "value3", 0)

	// key2 should be evicted (was least recently used)
	if _, found := cache.Get("key2"); found {
		t.Error("Expected key2 to be evicted")
	}

	// key1 and key3 should remain
	if _, found := cache.Get("key1"); !found {
		t.Error("Expected key1 to remain")
	}
	if _, found := cache.Get("key3"); !found {
		t.Error("Expected key3 to remain")
	}

	// Test update existing key
	cache.Set("key1", "updated_value1", 0)
	if value, found := cache.Get("key1"); !found || value != "updated_value1" {
		t.Errorf("Expected 'updated_value1', got %v", value)
	}

	// Test delete
	cache.Delete("key1")
	if _, found := cache.Get("key1"); found {
		t.Error("Expected key1 to be deleted")
	}

	// Test clear
	cache.Clear()
	if _, found := cache.Get("key3"); found {
		t.Error("Expected cache to be empty after Clear")
	}
}

func TestDefaultCacheKeyGenerator(t *testing.T) {
	// Test with simple struct
	type TestRequest struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}

	req1 := TestRequest{Model: "gpt-4", Prompt: "Hello"}
	req2 := TestRequest{Model: "gpt-4", Prompt: "Hello"}
	req3 := TestRequest{Model: "gpt-4", Prompt: "World"}

	key1, err1 := DefaultCacheKeyGenerator(req1)
	key2, err2 := DefaultCacheKeyGenerator(req2)
	key3, err3 := DefaultCacheKeyGenerator(req3)

	if err1 != nil || err2 != nil || err3 != nil {
		t.Errorf("Expected no errors, got %v, %v, %v", err1, err2, err3)
	}

	// Same requests should have same keys
	if key1 != key2 {
		t.Errorf("Expected same keys for identical requests, got %s != %s", key1, key2)
	}

	// Different requests should have different keys
	if key1 == key3 {
		t.Errorf("Expected different keys for different requests, but both were %s", key1)
	}

	// Test with unmarshalable type
	ch := make(chan int)
	_, err := DefaultCacheKeyGenerator(ch)
	if err == nil {
		t.Error("Expected error for unmarshalable type")
	}
}

func TestCacheMiddleware(t *testing.T) {
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   1 * time.Hour,
	}

	// Mock handler that increments a counter
	callCount := 0
	mockHandler := func(ctx context.Context, req any) (any, error) {
		callCount++
		return map[string]any{"count": callCount, "req": req}, nil
	}

	middleware := CacheMiddleware(config)
	wrappedHandler := middleware(mockHandler)

	ctx := context.Background()
	req := map[string]string{"test": "request"}

	// First call should execute handler
	resp1, err1 := wrappedHandler(ctx, req)
	if err1 != nil {
		t.Fatalf("Expected no error, got %v", err1)
	}
	if callCount != 1 {
		t.Errorf("Expected handler to be called once, got %d", callCount)
	}

	// Second call with same request should use cache
	resp2, err2 := wrappedHandler(ctx, req)
	if err2 != nil {
		t.Fatalf("Expected no error, got %v", err2)
	}
	if callCount != 1 {
		t.Errorf("Expected handler to still be called once (cached), got %d", callCount)
	}

	// Responses should be identical
	resp1Map := resp1.(map[string]any)
	resp2Map := resp2.(map[string]any)
	if resp1Map["count"] != resp2Map["count"] {
		t.Errorf("Expected cached response, got different counts: %v vs %v", resp1Map["count"], resp2Map["count"])
	}

	// Different request should call handler again
	req2 := map[string]string{"test": "different"}
	_, err3 := wrappedHandler(ctx, req2)
	if err3 != nil {
		t.Fatalf("Expected no error, got %v", err3)
	}
	if callCount != 2 {
		t.Errorf("Expected handler to be called twice for different request, got %d", callCount)
	}
}

func TestCacheMiddlewareWithCacheableFunc(t *testing.T) {
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   1 * time.Hour,
		CacheableFunc: func(req any) bool {
			// Only cache requests with "cacheable": true
			if reqMap, ok := req.(map[string]any); ok {
				if cacheable, exists := reqMap["cacheable"].(bool); exists {
					return cacheable
				}
			}
			return false
		},
	}

	callCount := 0
	mockHandler := func(ctx context.Context, req any) (any, error) {
		callCount++
		return map[string]any{"count": callCount}, nil
	}

	middleware := CacheMiddleware(config)
	wrappedHandler := middleware(mockHandler)

	ctx := context.Background()

	// Non-cacheable request should not be cached
	req1 := map[string]any{"cacheable": false, "data": "test1"}
	_, _ = wrappedHandler(ctx, req1)
	_, _ = wrappedHandler(ctx, req1) // Second call
	if callCount != 2 {
		t.Errorf("Expected non-cacheable request to call handler twice, got %d", callCount)
	}

	// Cacheable request should be cached
	req2 := map[string]any{"cacheable": true, "data": "test2"}
	_, _ = wrappedHandler(ctx, req2)
	_, _ = wrappedHandler(ctx, req2) // Second call should use cache
	if callCount != 3 {
		t.Errorf("Expected cacheable request to call handler once more (total 3), got %d", callCount)
	}
}

func TestCacheMiddlewareErrorHandling(t *testing.T) {
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   1 * time.Hour,
	}

	// Handler that returns error
	mockHandler := func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("handler error")
	}

	middleware := CacheMiddleware(config)
	wrappedHandler := middleware(mockHandler)

	ctx := context.Background()
	req := map[string]string{"test": "error"}

	// Error should not be cached
	_, err1 := wrappedHandler(ctx, req)
	if err1 == nil {
		t.Error("Expected error from handler")
	}

	_, err2 := wrappedHandler(ctx, req)
	if err2 == nil {
		t.Error("Expected error from handler on second call (should not be cached)")
	}
}

func TestCacheMiddlewareKeyGeneratorError(t *testing.T) {
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   1 * time.Hour,
		KeyGenerator: func(req any) (string, error) {
			return "", errors.New("key generation failed")
		},
	}

	callCount := 0
	mockHandler := func(ctx context.Context, req any) (any, error) {
		callCount++
		return testResponse, nil
	}

	middleware := CacheMiddleware(config)
	wrappedHandler := middleware(mockHandler)

	ctx := context.Background()
	req := map[string]string{"test": "request"}

	// Should proceed without caching when key generation fails
	resp1, err1 := wrappedHandler(ctx, req)
	if err1 != nil {
		t.Fatalf("Expected no error, got %v", err1)
	}
	if resp1 != testResponse {
		t.Errorf("Expected 'response', got %v", resp1)
	}
	if callCount != 1 {
		t.Errorf("Expected handler to be called once, got %d", callCount)
	}

	// Second call should also proceed (no caching)
	_, err2 := wrappedHandler(ctx, req)
	if err2 != nil {
		t.Fatalf("Expected no error, got %v", err2)
	}
	if callCount != 2 {
		t.Errorf("Expected handler to be called twice (no caching), got %d", callCount)
	}
}

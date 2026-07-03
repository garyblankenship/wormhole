package middleware

import (
	"context"
	"encoding/json"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestCacheClose(t *testing.T) {
	t.Parallel()
	t.Run("memory cache", func(t *testing.T) {
		t.Parallel()
		cache := NewMemoryCache(1)
		if err := cache.Close(); err != nil {
			t.Fatalf("MemoryCache.Close() error = %v", err)
		}
	})

	t.Run("ttl cache", func(t *testing.T) {
		t.Parallel()
		cache := NewTTLCache(1, time.Minute)
		if err := cache.Close(); err != nil {
			t.Fatalf("TTLCache.Close() error = %v", err)
		}
	})

	t.Run("lru cache", func(t *testing.T) {
		t.Parallel()
		cache := NewLRUCache(1)
		if err := cache.Close(); err != nil {
			t.Fatalf("LRUCache.Close() error = %v", err)
		}
	})
}

func TestDefaultCacheKeyGenerator(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

	// Responses should have identical content but be independent copies.
	resp1Map := resp1.(map[string]any)
	resp2Map := resp2.(map[string]any)
	if resp1Map["count"] != resp2Map["count"] {
		t.Errorf("Expected cached response, got different counts: %v vs %v", resp1Map["count"], resp2Map["count"])
	}

	// Mutating the cached response must not affect the original (clone isolation).
	resp2Map["count"] = 999
	if resp1Map["count"].(int) != 1 {
		t.Errorf("Expected original response unmodified after mutating clone, got %v", resp1Map["count"])
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

func TestCacheMiddlewareMissStoresIndependentClone(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   time.Hour,
	}

	type response struct {
		Count  int               `json:"count"`
		Labels []string          `json:"labels"`
		Meta   map[string]string `json:"meta"`
	}

	callCount := 0
	wrappedHandler := CacheMiddleware(config)(func(ctx context.Context, req any) (any, error) {
		callCount++
		return &response{
			Count:  1,
			Labels: []string{"alpha", "beta"},
			Meta:   map[string]string{"status": "fresh"},
		}, nil
	})

	ctx := context.Background()
	req := map[string]string{"test": "request"}

	resp1, err := wrappedHandler(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	first := resp1.(*response)
	first.Count = 99
	first.Labels[0] = "mutated"
	first.Meta["status"] = "stale"

	resp2, err := wrappedHandler(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	second := resp2.(*response)

	if callCount != 1 {
		t.Fatalf("Expected handler to be called once, got %d", callCount)
	}
	if first == second {
		t.Fatal("Expected cached response to be a distinct pointer")
	}
	if second.Count != 1 {
		t.Fatalf("Expected cached count to remain 1, got %d", second.Count)
	}
	if second.Labels[0] != "alpha" {
		t.Fatalf("Expected cached labels to remain unchanged, got %v", second.Labels)
	}
	if second.Meta["status"] != "fresh" {
		t.Fatalf("Expected cached metadata to remain unchanged, got %v", second.Meta)
	}
}

func TestCacheMiddlewareHitClonesNestedMutableMapValues(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(10)
	config := CacheConfig{
		Cache: cache,
		TTL:   time.Hour,
	}

	callCount := 0
	wrappedHandler := CacheMiddleware(config)(func(ctx context.Context, req any) (any, error) {
		callCount++
		return map[string]any{
			"strings": []string{"alpha", "beta"},
			"bytes":   []byte("blob"),
			"raw":     json.RawMessage(`{"ok":true}`),
			"meta":    map[string]string{"status": "fresh"},
		}, nil
	})

	ctx := context.Background()
	req := map[string]string{"test": "request"}

	resp1, err := wrappedHandler(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	first := resp1.(map[string]any)
	first["strings"].([]string)[0] = "mutated-on-miss"
	first["bytes"].([]byte)[0] = 'X'
	first["raw"].(json.RawMessage)[0] = '['
	first["meta"].(map[string]string)["status"] = "stale-on-miss"

	resp2, err := wrappedHandler(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	second := resp2.(map[string]any)
	if callCount != 1 {
		t.Fatalf("Expected handler to be called once, got %d", callCount)
	}
	if got := second["strings"].([]string); got[0] != "alpha" {
		t.Fatalf("Expected cached []string to remain unchanged after miss mutation, got %v", got)
	}
	if got := string(second["bytes"].([]byte)); got != "blob" {
		t.Fatalf("Expected cached []byte to remain unchanged after miss mutation, got %q", got)
	}
	if got := string(second["raw"].(json.RawMessage)); got != `{"ok":true}` {
		t.Fatalf("Expected cached RawMessage to remain unchanged after miss mutation, got %s", got)
	}
	if got := second["meta"].(map[string]string)["status"]; got != "fresh" {
		t.Fatalf("Expected cached map[string]string to remain unchanged after miss mutation, got %q", got)
	}

	second["strings"].([]string)[1] = "mutated-on-hit"
	second["bytes"].([]byte)[1] = 'Y'
	second["raw"].(json.RawMessage)[1] = 'X'
	second["meta"].(map[string]string)["status"] = "stale-on-hit"

	resp3, err := wrappedHandler(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	third := resp3.(map[string]any)
	if got := third["strings"].([]string); got[1] != "beta" {
		t.Fatalf("Expected cache hit clone for []string, got %v", got)
	}
	if got := string(third["bytes"].([]byte)); got != "blob" {
		t.Fatalf("Expected cache hit clone for []byte, got %q", got)
	}
	if got := string(third["raw"].(json.RawMessage)); got != `{"ok":true}` {
		t.Fatalf("Expected cache hit clone for RawMessage, got %s", got)
	}
	if got := third["meta"].(map[string]string)["status"]; got != "fresh" {
		t.Fatalf("Expected cache hit clone for map[string]string, got %q", got)
	}
}

func TestCacheMiddlewareWithCacheableFunc(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

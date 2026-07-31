package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

const testValue1 = "value1"

func TestMemoryCache(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(3)
	t.Cleanup(func() { _ = cache.Close() })

	cache.Set("key1", testValue1, time.Hour)
	if value, found := cache.Get("key1"); !found || value != testValue1 {
		t.Errorf("Get(key1) = (%v, %t), want (%q, true)", value, found, testValue1)
	}
	if value, found := cache.Get("nonexistent"); found {
		t.Errorf("Get(nonexistent) = (%v, true), want miss", value)
	}

	cache.Set("expired", "value", time.Millisecond)
	time.Sleep(2 * time.Millisecond)
	if _, found := cache.Get("expired"); found {
		t.Error("expired key remained in cache")
	}

	cache.Set("key1", "value1", time.Hour)
	cache.Set("key2", "value2", time.Hour)
	cache.Set("key3", "value3", time.Hour)
	cache.Set("key4", "value4", time.Hour)
	remaining := 0
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, found := cache.Get(key); found {
			remaining++
		}
	}
	if remaining > 2 {
		t.Errorf("remaining pre-eviction entries = %d, want <= 2", remaining)
	}
	if _, found := cache.Get("key4"); !found {
		t.Error("newest key was evicted")
	}

	cache.Delete("key4")
	if _, found := cache.Get("key4"); found {
		t.Error("deleted key remained in cache")
	}
	cache.Clear()
	if _, found := cache.Get("key1"); found {
		t.Error("cache was not empty after Clear")
	}
}

func TestTTLCache(t *testing.T) {
	t.Parallel()
	cache := NewTTLCache(10, 50*time.Millisecond)
	t.Cleanup(func() { _ = cache.Close() })
	cache.SetDefault("key1", "value1")
	if _, found := cache.Get("key1"); !found {
		t.Fatal("SetDefault value was not immediately available")
	}
	time.Sleep(60 * time.Millisecond)
	if _, found := cache.Get("key1"); found {
		t.Error("default TTL value did not expire")
	}
}

func TestLRUCache(t *testing.T) {
	t.Parallel()
	cache := NewLRUCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)
	if value, found := cache.Get("key1"); !found || value != "value1" {
		t.Fatalf("Get(key1) = (%v, %t)", value, found)
	}
	cache.Set("key3", "value3", 0)
	if _, found := cache.Get("key2"); found {
		t.Error("least-recently-used key was not evicted")
	}
	cache.Set("key1", "updated_value1", 0)
	if value, found := cache.Get("key1"); !found || value != "updated_value1" {
		t.Fatalf("updated key = (%v, %t)", value, found)
	}
	cache.Delete("key1")
	cache.Clear()
	if _, found := cache.Get("key3"); found {
		t.Error("cache was not empty after Clear")
	}
}

func TestCacheClose(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		cache Cache
	}{
		{name: "memory", cache: NewMemoryCache(1)},
		{name: "ttl", cache: NewTTLCache(1, time.Minute)},
		{name: "lru", cache: NewLRUCache(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.cache.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestDefaultCacheKeyGenerator(t *testing.T) {
	t.Parallel()
	type request struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	key1, err1 := DefaultCacheKeyGenerator(request{Model: "gpt", Prompt: "Hello"})
	key2, err2 := DefaultCacheKeyGenerator(request{Model: "gpt", Prompt: "Hello"})
	key3, err3 := DefaultCacheKeyGenerator(request{Model: "gpt", Prompt: "World"})
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("key errors = %v, %v, %v", err1, err2, err3)
	}
	if key1 != key2 || key1 == key3 {
		t.Fatalf("keys must be equal for identical and differ for changed requests: %q %q %q", key1, key2, key3)
	}
	if _, err := DefaultCacheKeyGenerator(make(chan int)); err == nil {
		t.Error("unmarshalable request did not return an error")
	}
}

func TestDefaultCacheKeyGeneratorIncludesProviderOptionsForEveryTypedRequest(t *testing.T) {
	t.Parallel()
	requests := []struct {
		name  string
		left  any
		right any
	}{
		{"text", types.TextRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "left"}}}, types.TextRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "right"}}}},
		{"structured", types.StructuredRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "left"}}}, types.StructuredRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "right"}}}},
		{"embeddings", types.EmbeddingsRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.EmbeddingsRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"rerank", types.RerankRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.RerankRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"image", types.ImageRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.ImageRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"audio", types.AudioRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.AudioRequest{ProviderOptions: map[string]any{"mode": "right"}}},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			left, err := DefaultCacheKeyGenerator(request.left)
			if err != nil {
				t.Fatal(err)
			}
			right, err := DefaultCacheKeyGenerator(request.right)
			if err != nil {
				t.Fatal(err)
			}
			if left == right {
				t.Fatal("provider options did not affect the cache key")
			}
		})
	}
}

func TestDefaultCacheKeyGeneratorRejectsUnmarshalableProviderOptions(t *testing.T) {
	t.Parallel()
	request := types.TextRequest{BaseRequest: types.BaseRequest{
		ProviderOptions: map[string]any{"callback": func() {}},
	}}
	if _, err := DefaultCacheKeyGenerator(request); err == nil {
		t.Fatal("unmarshalable provider options did not return an error")
	}
}

func testTextRequest(prompt string) types.TextRequest {
	return types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage(prompt)}}
}

func TestTypedCacheMiddleware(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(10)
	t.Cleanup(func() { _ = cache.Close() })
	var calls int
	handler := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Hour}).ApplyText(func(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
		calls++
		return &types.TextResponse{Text: request.Messages[0].GetContent().(string), Metadata: map[string]any{"count": calls, "meta": map[string]any{"status": "fresh"}}}, nil
	})

	first, err := handler(context.Background(), testTextRequest("request"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := handler(context.Background(), testTextRequest("request"))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || first == second {
		t.Fatalf("calls = %d; responses must be cloned", calls)
	}
	second.Metadata["count"] = 999
	second.Metadata["meta"].(map[string]any)["status"] = "mutated"
	if first.Metadata["count"] != 1 || first.Metadata["meta"].(map[string]any)["status"] != "fresh" {
		t.Fatalf("cached response mutation leaked into first response: %#v", first.Metadata)
	}
	third, err := handler(context.Background(), testTextRequest("request"))
	if err != nil {
		t.Fatal(err)
	}
	if third.Metadata["meta"].(map[string]any)["status"] != "fresh" {
		t.Fatalf("cache hit was not independently cloned: %#v", third.Metadata)
	}
	if _, err := handler(context.Background(), testTextRequest("different")); err != nil || calls != 2 {
		t.Fatalf("different request = calls %d, err %v", calls, err)
	}
}

func TestTypedCacheMiddlewarePolicies(t *testing.T) {
	t.Parallel()
	t.Run("cacheable function", func(t *testing.T) {
		t.Parallel()
		cache := NewMemoryCache(10)
		t.Cleanup(func() { _ = cache.Close() })
		var calls int
		mw := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Hour, CacheableFunc: func(req any) bool {
			request := req.(types.TextRequest)
			return request.Messages[0].GetContent() == "cacheable"
		}})
		handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
			calls++
			return &types.TextResponse{Text: "response"}, nil
		})
		for range 2 {
			_, _ = handler(context.Background(), testTextRequest("not-cacheable"))
		}
		for range 2 {
			_, _ = handler(context.Background(), testTextRequest("cacheable"))
		}
		if calls != 3 {
			t.Fatalf("handler calls = %d, want 3", calls)
		}
	})

	t.Run("handler and key errors bypass storage", func(t *testing.T) {
		t.Parallel()
		cache := NewMemoryCache(10)
		t.Cleanup(func() { _ = cache.Close() })
		var calls int
		failing := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Hour}).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
			calls++
			return nil, errors.New("handler error")
		})
		for range 2 {
			if _, err := failing(context.Background(), testTextRequest("error")); err == nil {
				t.Fatal("handler error was lost")
			}
		}
		if calls != 2 {
			t.Fatalf("error response was cached: %d calls", calls)
		}
		calls = 0
		keyError := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Hour, KeyGenerator: func(any) (string, error) { return "", errors.New("key") }}).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
			calls++
			return &types.TextResponse{Text: "response"}, nil
		})
		for range 2 {
			if _, err := keyError(context.Background(), testTextRequest("key")); err != nil {
				t.Fatal(err)
			}
		}
		if calls != 2 {
			t.Fatalf("key-generation error did not bypass cache: %d calls", calls)
		}
	})
}

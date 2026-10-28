package middleware

import (
	"testing"
	"time"
)

func TestMemoryCacheReplacementPreservesOtherKeys(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("other", "retained", time.Hour)
	cache.Set("updated", "before", 2*time.Hour)
	cache.Set("updated", "after", 2*time.Hour)
	if got, ok := cache.Get("other"); !ok || got != "retained" {
		t.Fatalf("replacement evicted unrelated entry: %v, %t", got, ok)
	}
	if got, ok := cache.Get("updated"); !ok || got != "after" {
		t.Fatalf("replacement value = %v, %t", got, ok)
	}
}

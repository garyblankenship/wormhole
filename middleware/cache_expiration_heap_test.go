package middleware

import (
	"testing"
	"time"
)

func assertMemoryCacheIndex(t *testing.T, cache *MemoryCache) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.entries) != len(cache.expiry) {
		t.Fatalf("entries = %d, indexed expirations = %d", len(cache.entries), len(cache.expiry))
	}
	for i, entry := range cache.expiry {
		if entry.heapIndex != i || cache.entries[entry.key] != entry {
			t.Fatalf("heap index %d has stale entry %q (stored index %d)", i, entry.key, entry.heapIndex)
		}
		if i > 0 && entry.expiration.Before(cache.expiry[(i-1)/2].expiration) {
			t.Fatalf("expiration heap out of order at index %d", i)
		}
	}
}

func TestMemoryCacheEvictsEarliestExpirationNotLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("early", 1, time.Hour)
	cache.Set("late", 2, 3*time.Hour)
	cache.Get("early")
	cache.Set("new", 3, 2*time.Hour)
	if _, ok := cache.Get("early"); ok {
		t.Fatal("earliest expiration retained at capacity")
	}
	for _, key := range []string{"late", "new"} {
		if _, ok := cache.Get(key); !ok {
			t.Fatalf("key %q was evicted", key)
		}
	}
	assertMemoryCacheIndex(t, cache)
}

func TestMemoryCacheReplacementReordersExpirationWithoutEviction(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("a", "before", time.Hour)
	cache.Set("b", "retained", 2*time.Hour)
	cache.Set("a", "after", 3*time.Hour)
	assertMemoryCacheIndex(t, cache)
	if got, ok := cache.Get("b"); !ok || got != "retained" {
		t.Fatalf("replacement evicted b: %v, %t", got, ok)
	}
	cache.Set("c", "new", 4*time.Hour)
	if _, ok := cache.Get("b"); ok {
		t.Fatal("replacement did not reorder earliest expiration")
	}
	if got, ok := cache.Get("a"); !ok || got != "after" {
		t.Fatalf("replacement value = %v, %t", got, ok)
	}
	assertMemoryCacheIndex(t, cache)
}

func TestMemoryCacheMutationKeepsExpirationIndexBounded(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(3)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("a", 1, time.Hour)
	cache.Set("b", 2, 2*time.Hour)
	cache.Set("c", 3, 3*time.Hour)
	for range 20 {
		cache.Set("b", 4, 4*time.Hour)
		assertMemoryCacheIndex(t, cache)
	}
	cache.Delete("a")
	cache.Delete("missing")
	assertMemoryCacheIndex(t, cache)
	cache.Set("d", 5, time.Hour)
	cache.Set("e", 6, 5*time.Hour)
	if _, ok := cache.Get("d"); ok {
		t.Fatal("deleted entry left a stale heap node affecting eviction")
	}
	assertMemoryCacheIndex(t, cache)
	cache.Clear()
	assertMemoryCacheIndex(t, cache)
	cache.Set("fresh", 7, time.Hour)
	assertMemoryCacheIndex(t, cache)
}

func TestMemoryCacheCleanupExpirationIndex(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(2)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("expired", 1, -time.Hour)
	cache.Set("live", 2, time.Hour)
	cache.mu.Lock()
	cache.cleanupExpiredLocked(time.Now())
	cache.mu.Unlock()
	assertMemoryCacheIndex(t, cache)
	if _, ok := cache.Get("expired"); ok {
		t.Fatal("expired entry survived cleanup")
	}
	cache.Set("next", 3, 2*time.Hour)
	if _, ok := cache.Get("live"); !ok {
		t.Fatal("cleanup left stale capacity accounting")
	}
	assertMemoryCacheIndex(t, cache)
}

func TestMemoryCacheEmptyKeyRetainsSentinelEvictionBehavior(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(1)
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("", "empty", time.Hour)
	cache.Set("next", "next", 2*time.Hour)
	if got, ok := cache.Get(""); !ok || got != "empty" {
		t.Fatalf("empty key evicted: %v, %t", got, ok)
	}
	if _, ok := cache.Get("next"); !ok {
		t.Fatal("new key absent after empty-key sentinel scan")
	}
	assertMemoryCacheIndex(t, cache)
}

package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Cache interface for middleware
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
	Close() error
}

// MemoryCache implements an in-memory cache
type MemoryCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	maxSize   int
	stopCh    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

type cacheEntry struct {
	value      any
	expiration time.Time
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int) *MemoryCache {
	cache := &MemoryCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	cache.wg.Add(1)
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (mc *MemoryCache) Get(key string) (any, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.entries[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.expiration) {
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in the cache
func (mc *MemoryCache) Set(key string, value any, ttl time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Evict oldest entries if at capacity
	if len(mc.entries) >= mc.maxSize {
		mc.evictOldest()
	}

	mc.entries[key] = &cacheEntry{
		value:      value,
		expiration: time.Now().Add(ttl),
	}
}

// Delete removes a value from the cache
func (mc *MemoryCache) Delete(key string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	delete(mc.entries, key)
}

// Clear removes all entries from the cache
func (mc *MemoryCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.entries = make(map[string]*cacheEntry)
}

func (mc *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range mc.entries {
		if oldestKey == "" || entry.expiration.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiration
		}
	}

	if oldestKey != "" {
		delete(mc.entries, oldestKey)
	}
}

func (mc *MemoryCache) cleanup() {
	defer mc.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.mu.Lock()
			now := time.Now()
			for key, entry := range mc.entries {
				if now.After(entry.expiration) {
					delete(mc.entries, key)
				}
			}
			mc.mu.Unlock()
		case <-mc.stopCh:
			return
		}
	}
}

// CacheKeyGenerator generates cache keys from requests
type CacheKeyGenerator func(req any) (string, error)

// DefaultCacheKeyGenerator creates a cache key by hashing the JSON representation
// plus ProviderOptions, which carries json:"-" (so it is invisible to Marshal)
// yet changes the upstream call — requests differing only in ProviderOptions must
// not collide.
func DefaultCacheKeyGenerator(req any) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	h.Write(data)
	if po, ok := req.(interface{ GetProviderOptions() map[string]any }); ok {
		if opts := po.GetProviderOptions(); len(opts) > 0 {
			ob, err := json.Marshal(opts)
			if err != nil {
				return "", err
			}
			h.Write(ob)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CacheConfig holds cache middleware configuration
type CacheConfig struct {
	Cache         Cache
	TTL           time.Duration
	KeyGenerator  CacheKeyGenerator
	CacheableFunc func(req any) bool
}

// Close stops the cleanup goroutine and waits for it to finish
func (mc *MemoryCache) Close() error {
	mc.closeOnce.Do(func() {
		close(mc.stopCh)
		mc.wg.Wait()
	})
	return nil
}

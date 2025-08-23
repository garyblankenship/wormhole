package middleware

import (
	"context"
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
}

// MemoryCache implements an in-memory cache
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
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
	}

	// Start cleanup goroutine
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
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mc.mu.Lock()
		now := time.Now()
		for key, entry := range mc.entries {
			if now.After(entry.expiration) {
				delete(mc.entries, key)
			}
		}
		mc.mu.Unlock()
	}
}

// CacheKeyGenerator generates cache keys from requests
type CacheKeyGenerator func(req any) (string, error)

// DefaultCacheKeyGenerator creates a cache key by hashing the JSON representation
func DefaultCacheKeyGenerator(req any) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// CacheConfig holds cache middleware configuration
type CacheConfig struct {
	Cache         Cache
	TTL           time.Duration
	KeyGenerator  CacheKeyGenerator
	CacheableFunc func(req any) bool
}

// CacheMiddleware implements response caching.
//
// Example usage:
//
//	cache := middleware.NewMemoryCache(100)
//	config := middleware.CacheConfig{
//	    Cache: cache,
//	    TTL: 5 * time.Minute,
//	}
//	middleware.CacheMiddleware(config)
//
// For simple TTL caching:
//
//	cache := middleware.NewTTLCache(100, 5 * time.Minute)
//	config := middleware.CacheConfig{Cache: cache, TTL: config.DefaultTTL}
func CacheMiddleware(config CacheConfig) Middleware {
	if config.KeyGenerator == nil {
		config.KeyGenerator = DefaultCacheKeyGenerator
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			// Check if request is cacheable
			if config.CacheableFunc != nil && !config.CacheableFunc(req) {
				return next(ctx, req)
			}

			// Generate cache key
			key, err := config.KeyGenerator(req)
			if err != nil {
				// If we can't generate a key, just proceed without caching
				return next(ctx, req)
			}

			// Check cache
			if cached, found := config.Cache.Get(key); found {
				return cached, nil
			}

			// Execute request
			resp, err := next(ctx, req)
			if err != nil {
				return nil, err
			}

			// Cache successful response
			config.Cache.Set(key, resp, config.TTL)

			return resp, err
		}
	}
}

// TTLCache extends MemoryCache with automatic TTL management
type TTLCache struct {
	*MemoryCache
	defaultTTL time.Duration
}

// NewTTLCache creates a cache with a default TTL
func NewTTLCache(maxSize int, defaultTTL time.Duration) *TTLCache {
	return &TTLCache{
		MemoryCache: NewMemoryCache(maxSize),
		defaultTTL:  defaultTTL,
	}
}

// SetDefault stores a value with the default TTL
func (tc *TTLCache) SetDefault(key string, value any) {
	tc.Set(key, value, tc.defaultTTL)
}

// LRUCache implements a Least Recently Used cache
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	cache    map[string]*lruNode
	head     *lruNode
	tail     *lruNode
}

type lruNode struct {
	key   string
	value any
	prev  *lruNode
	next  *lruNode
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int) *LRUCache {
	lru := &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*lruNode),
	}

	// Create sentinel nodes
	lru.head = &lruNode{}
	lru.tail = &lruNode{}
	lru.head.next = lru.tail
	lru.tail.prev = lru.head

	return lru
}

// Get retrieves a value from the LRU cache
func (lru *LRUCache) Get(key string) (any, bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	node, exists := lru.cache[key]
	if !exists {
		return nil, false
	}

	// Move to front
	lru.moveToFront(node)

	return node.value, true
}

// Set stores a value in the LRU cache
func (lru *LRUCache) Set(key string, value any, ttl time.Duration) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if node, exists := lru.cache[key]; exists {
		// Update existing node
		node.value = value
		lru.moveToFront(node)
		return
	}

	// Add new node
	node := &lruNode{
		key:   key,
		value: value,
	}

	lru.cache[key] = node
	lru.addToFront(node)

	// Evict if over capacity
	if len(lru.cache) > lru.capacity {
		lru.evictLRU()
	}
}

// Delete removes a value from the LRU cache
func (lru *LRUCache) Delete(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if node, exists := lru.cache[key]; exists {
		lru.removeNode(node)
		delete(lru.cache, key)
	}
}

// Clear removes all entries from the LRU cache
func (lru *LRUCache) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.cache = make(map[string]*lruNode)
	lru.head.next = lru.tail
	lru.tail.prev = lru.head
}

func (lru *LRUCache) moveToFront(node *lruNode) {
	lru.removeNode(node)
	lru.addToFront(node)
}

func (lru *LRUCache) addToFront(node *lruNode) {
	node.prev = lru.head
	node.next = lru.head.next
	lru.head.next.prev = node
	lru.head.next = node
}

func (lru *LRUCache) removeNode(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (lru *LRUCache) evictLRU() {
	node := lru.tail.prev
	lru.removeNode(node)
	delete(lru.cache, node.key)
}

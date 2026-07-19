package middleware

import (
	"time"
)

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

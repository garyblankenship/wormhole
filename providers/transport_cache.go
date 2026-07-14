package providers

import (
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const maxCachedTransports = 64

type cachedTransport struct {
	transport    *http.Transport
	lastUsedNano atomic.Int64
}

// TransportCache is an instance-scoped, bounded LRU cache of *http.Transport
// keyed by transport-config fingerprint. Each HTTPClientWrapper owns one, so
// transports are NOT shared across wrapper instances or differing TLS configs.
type TransportCache struct {
	mu         sync.RWMutex
	transports map[string]*cachedTransport
	hits       atomic.Int64
	misses     atomic.Int64
}

// NewTransportCache returns an empty, ready-to-use TransportCache.
func NewTransportCache() *TransportCache {
	return &TransportCache{
		transports: make(map[string]*cachedTransport),
	}
}

func (tc *TransportCache) get(key string) (*http.Transport, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	entry, ok := tc.transports[key]
	if ok {
		entry.lastUsedNano.Store(time.Now().UnixNano())
		tc.hits.Add(1)
		return entry.transport, true
	}
	return nil, false
}

func (tc *TransportCache) set(key string, transport *http.Transport) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if existing, ok := tc.transports[key]; ok && existing.transport != nil {
		existing.transport.CloseIdleConnections()
	}
	entry := &cachedTransport{transport: transport}
	entry.lastUsedNano.Store(time.Now().UnixNano())
	tc.transports[key] = entry
	tc.evictOldestLocked()
}

// recordMiss increments the cache-miss counter. Call when get() returns false.
func (tc *TransportCache) recordMiss() {
	tc.misses.Add(1)
}

// evictOldestLocked must be called with tc.mu held.
func (tc *TransportCache) evictOldestLocked() {
	if len(tc.transports) <= maxCachedTransports {
		return
	}

	type transportInfo struct {
		key          string
		lastUsedNano int64
	}

	entries := make([]transportInfo, 0, len(tc.transports))
	for key, entry := range tc.transports {
		entries = append(entries, transportInfo{key: key, lastUsedNano: entry.lastUsedNano.Load()})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastUsedNano < entries[j].lastUsedNano
	})

	for len(tc.transports) > maxCachedTransports && len(entries) > 0 {
		victim := entries[0]
		entries = entries[1:]
		if entry, ok := tc.transports[victim.key]; ok {
			if entry.transport != nil {
				entry.transport.CloseIdleConnections()
			}
			delete(tc.transports, victim.key)
		}
	}
}

// TransportCacheMetrics holds transport cache performance statistics
type TransportCacheMetrics struct {
	Hits   int64
	Misses int64
	Size   int
}

// Metrics returns current cache performance statistics for this instance.
func (tc *TransportCache) Metrics() TransportCacheMetrics {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	return TransportCacheMetrics{
		Hits:   tc.hits.Load(),
		Misses: tc.misses.Load(),
		Size:   len(tc.transports),
	}
}

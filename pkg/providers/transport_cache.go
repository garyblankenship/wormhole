package providers

import (
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const maxCachedTransports = 64

var (
	transportCache sync.RWMutex
	transports     = make(map[string]*cachedTransport)

	// Transport cache metrics
	transportCacheHits   atomic.Int64
	transportCacheMisses atomic.Int64
)

type cachedTransport struct {
	transport    *http.Transport
	lastUsedNano atomic.Int64
}

func getCachedTransport(key string) (*http.Transport, bool) {
	transportCache.RLock()
	defer transportCache.RUnlock()
	entry, ok := transports[key]
	if ok {
		entry.lastUsedNano.Store(time.Now().UnixNano())
		transportCacheHits.Add(1)
		return entry.transport, true
	}
	return nil, false
}

func setCachedTransport(key string, transport *http.Transport) {
	transportCache.Lock()
	defer transportCache.Unlock()
	if existing, ok := transports[key]; ok && existing.transport != nil {
		existing.transport.CloseIdleConnections()
	}
	entry := &cachedTransport{transport: transport}
	entry.lastUsedNano.Store(time.Now().UnixNano())
	transports[key] = entry
	evictOldestTransportsLocked()
}

func evictOldestTransportsLocked() {
	if len(transports) <= maxCachedTransports {
		return
	}

	type transportInfo struct {
		key          string
		lastUsedNano int64
	}

	entries := make([]transportInfo, 0, len(transports))
	for key, entry := range transports {
		entries = append(entries, transportInfo{key: key, lastUsedNano: entry.lastUsedNano.Load()})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastUsedNano < entries[j].lastUsedNano
	})

	for len(transports) > maxCachedTransports && len(entries) > 0 {
		victim := entries[0]
		entries = entries[1:]
		if entry, ok := transports[victim.key]; ok {
			if entry.transport != nil {
				entry.transport.CloseIdleConnections()
			}
			delete(transports, victim.key)
		}
	}
}

// TransportCacheMetrics holds transport cache performance statistics
type TransportCacheMetrics struct {
	Hits   int64
	Misses int64
	Size   int
}

// GetTransportCacheMetrics returns current transport cache performance statistics
func GetTransportCacheMetrics() TransportCacheMetrics {
	transportCache.RLock()
	defer transportCache.RUnlock()

	return TransportCacheMetrics{
		Hits:   transportCacheHits.Load(),
		Misses: transportCacheMisses.Load(),
		Size:   len(transports),
	}
}

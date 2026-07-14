package wormhole

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	defaultIdempotencyTTL    = 24 * time.Hour
	idempotencySweepInterval = 5 * time.Minute
)

type idempotencyEntry struct {
	ready     chan struct{}
	expiresAt time.Time
	payload   []byte
	value     any
	err       error
}

func executeTrackedRequest[T any](ctx context.Context, p *Wormhole, operation string, request any, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if !p.trackRequest() {
		return zero, fmt.Errorf("client is shutting down")
	}
	defer p.untrackRequest()

	if !p.hasIdempotency() {
		return fn(ctx)
	}

	cacheKey, ok := p.idempotencyCacheKey(operation, request)
	if !ok {
		return fn(ctx)
	}

	ttl := p.idempotencyTTL()
	now := time.Now()

	entry, created := p.loadOrCreateIdempotencyEntry(cacheKey, now, ttl)
	if !created {
		select {
		case <-entry.ready:
		case <-ctx.Done():
			return zero, ctx.Err()
		}
		return cachedIdempotentValue[T](entry)
	}

	result, err := fn(ctx)
	entry.err = err
	if err == nil {
		entry.value = result
		if payload, marshalErr := json.Marshal(result); marshalErr == nil {
			entry.payload = payload
		}
	}
	close(entry.ready)

	p.idempotencyMu.Lock()
	if entry.err != nil {
		delete(p.idempotencyCache, cacheKey)
	}
	p.idempotencyMu.Unlock()

	return result, err
}

func cachedIdempotentValue[T any](entry *idempotencyEntry) (T, error) {
	var zero T
	if entry == nil {
		return zero, fmt.Errorf("missing idempotent cache entry")
	}
	if entry.err != nil {
		return zero, entry.err
	}
	if len(entry.payload) > 0 {
		var cloned T
		if err := json.Unmarshal(entry.payload, &cloned); err == nil {
			return cloned, nil
		}
	}
	if entry.value != nil {
		if value, ok := entry.value.(T); ok {
			return value, nil
		}
	}
	return zero, fmt.Errorf("cached idempotent response type mismatch")
}

func (p *Wormhole) hasIdempotency() bool {
	return p.config.Idempotency != nil && p.config.Idempotency.Key != ""
}

func (p *Wormhole) idempotencyTTL() time.Duration {
	if p.config.Idempotency == nil || p.config.Idempotency.TTL <= 0 {
		return defaultIdempotencyTTL
	}
	return p.config.Idempotency.TTL
}

func (p *Wormhole) idempotencyCacheKey(operation string, request any) (string, bool) {
	if !p.hasIdempotency() {
		return "", false
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", false
	}
	h := sha256.New()
	h.Write(payload)
	// ProviderOptions carries json:"-" so json.Marshal(request) above excludes it;
	// fold it in separately so requests differing only in provider-specific options
	// don't collide on the same idempotency key. Mirrors DefaultCacheKeyGenerator
	// (middleware/cache.go).
	if po, ok := request.(interface{ GetProviderOptions() map[string]any }); ok {
		if opts := po.GetProviderOptions(); len(opts) > 0 {
			if ob, err := json.Marshal(opts); err == nil {
				h.Write(ob)
			}
		}
	}
	return p.config.Idempotency.Key + ":" + operation + ":" + hex.EncodeToString(h.Sum(nil)), true
}

func (p *Wormhole) loadOrCreateIdempotencyEntry(cacheKey string, now time.Time, ttl time.Duration) (*idempotencyEntry, bool) {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()

	if entry, exists := p.idempotencyCache[cacheKey]; exists {
		if now.Before(entry.expiresAt) {
			return entry, false
		}
		delete(p.idempotencyCache, cacheKey)
	}

	entry := &idempotencyEntry{
		ready:     make(chan struct{}),
		expiresAt: now.Add(ttl),
	}
	p.idempotencyCache[cacheKey] = entry
	return entry, true
}

// startIdempotencySweeper starts a background goroutine that periodically
// removes expired entries from the idempotency cache, bounding its growth.
func (p *Wormhole) startIdempotencySweeper() {
	p.idempotencySweepWg.Add(1)
	go func() {
		defer p.idempotencySweepWg.Done()
		ticker := time.NewTicker(idempotencySweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.sweepIdempotencyCache()
			case <-p.shutdownChan:
				return
			}
		}
	}()
}

// sweepIdempotencyCache evicts idempotency entries whose TTL has expired.
func (p *Wormhole) sweepIdempotencyCache() {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	now := time.Now()
	for key, entry := range p.idempotencyCache {
		if now.After(entry.expiresAt) {
			delete(p.idempotencyCache, key)
		}
	}
}

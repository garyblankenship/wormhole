package wormhole

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

const (
	defaultIdempotencyTTL        = 24 * time.Hour
	defaultIdempotencyMaxEntries = 10_000
	idempotencySweepInterval     = 5 * time.Minute
)

type idempotencyState uint8

const (
	idempotencyInFlight idempotencyState = iota
	idempotencyCompleted
	idempotencyAbandoned
)

type idempotencyEntry struct {
	ready     chan struct{}
	state     idempotencyState
	expiresAt time.Time
	payload   []byte
	value     any
	err       error
	cacheKey  string
	orderElem *list.Element
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

	for {
		entry, created, admissionErr := p.loadOrCreateIdempotencyEntry(cacheKey, now)
		if admissionErr != nil {
			return zero, admissionErr
		}
		if !created {
			select {
			case <-entry.ready:
			case <-ctx.Done():
				return zero, ctx.Err()
			}
			if entry.state == idempotencyAbandoned {
				if err := ctx.Err(); err != nil {
					return zero, err
				}
				now = time.Now()
				continue
			}
			return cachedIdempotentValue[T](entry)
		}

		result, err := fn(ctx)
		if ctx.Err() != nil {
			p.abandonIdempotencyEntry(cacheKey, entry)
			return result, err
		}
		entry.err = err
		if err == nil {
			entry.value = result
			if payload, marshalErr := json.Marshal(result); marshalErr == nil {
				entry.payload = payload
			}
			p.completeIdempotencyEntry(entry, time.Now(), ttl)
		} else {
			p.removeIdempotencyEntry(cacheKey, entry)
		}
		close(entry.ready)
		return result, err
	}
}

// completeIdempotencyEntry starts the response cache TTL after a successful
// provider result is available. It deliberately updates the entry even if an
// operator cleared the cache: existing duplicate waiters still own this entry
// and must be released, while the cache map remains untouched.
func (p *Wormhole) completeIdempotencyEntry(entry *idempotencyEntry, now time.Time, ttl time.Duration) {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	if p.idempotencyOrder == nil {
		p.idempotencyOrder = list.New()
	}

	entry.state = idempotencyCompleted
	entry.expiresAt = now.Add(ttl)
	if p.idempotencyCache[entry.cacheKey] == entry && entry.orderElem == nil {
		entry.orderElem = p.idempotencyOrder.PushBack(entry)
	}
}

// removeIdempotencyEntry removes entry only when it remains the cache's current
// value. A completed request must not delete a newer replacement entry.
func (p *Wormhole) removeIdempotencyEntry(cacheKey string, entry *idempotencyEntry) {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()

	if p.idempotencyCache[cacheKey] == entry {
		p.deleteIdempotencyEntryLocked(cacheKey, entry)
	}
}

func (p *Wormhole) abandonIdempotencyEntry(cacheKey string, entry *idempotencyEntry) {
	p.idempotencyMu.Lock()
	if p.idempotencyCache[cacheKey] == entry {
		p.deleteIdempotencyEntryLocked(cacheKey, entry)
	}
	entry.state = idempotencyAbandoned
	close(entry.ready)
	p.idempotencyMu.Unlock()
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

func (p *Wormhole) idempotencyMaxEntries() int {
	if p.config.Idempotency == nil || p.config.Idempotency.MaxEntries <= 0 {
		return defaultIdempotencyMaxEntries
	}
	return p.config.Idempotency.MaxEntries
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

func (p *Wormhole) loadOrCreateIdempotencyEntry(cacheKey string, now time.Time) (*idempotencyEntry, bool, error) {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	if p.idempotencyOrder == nil {
		p.idempotencyOrder = list.New()
	}

	if entry, exists := p.idempotencyCache[cacheKey]; exists {
		if entry.state == idempotencyInFlight || now.Before(entry.expiresAt) {
			return entry, false, nil
		}
		p.deleteIdempotencyEntryLocked(cacheKey, entry)
	}

	if len(p.idempotencyCache) >= p.idempotencyMaxEntries() {
		oldest := p.idempotencyOrder.Front()
		if oldest == nil {
			p.idempotencyRejections++
			return nil, false, types.ErrIdempotencyCapacity
		}
		entry := oldest.Value.(*idempotencyEntry)
		p.deleteIdempotencyEntryLocked(entry.cacheKey, entry)
		p.idempotencyEvictions++
	}

	entry := &idempotencyEntry{
		ready:    make(chan struct{}),
		state:    idempotencyInFlight,
		cacheKey: cacheKey,
	}
	p.idempotencyCache[cacheKey] = entry
	return entry, true, nil
}

func (p *Wormhole) deleteIdempotencyEntryLocked(cacheKey string, entry *idempotencyEntry) {
	if p.idempotencyCache[cacheKey] != entry {
		return
	}
	delete(p.idempotencyCache, cacheKey)
	if entry.orderElem != nil && p.idempotencyOrder != nil {
		p.idempotencyOrder.Remove(entry.orderElem)
		entry.orderElem = nil
	}
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

// sweepIdempotencyCache evicts completed idempotency entries whose response TTL
// has expired. In-flight requests do not have a response TTL.
func (p *Wormhole) sweepIdempotencyCache() {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	now := time.Now()
	for key, entry := range p.idempotencyCache {
		if entry.state == idempotencyCompleted && now.After(entry.expiresAt) {
			p.deleteIdempotencyEntryLocked(key, entry)
		}
	}
}

// IdempotencyCacheStats reports bounded-cache state and capacity pressure.
type IdempotencyCacheStats struct {
	Entries            int
	Capacity           int
	CapacityEvictions  uint64
	CapacityRejections uint64
}

// GetIdempotencyCacheStats returns a consistent idempotency cache snapshot.
func (p *Wormhole) GetIdempotencyCacheStats() IdempotencyCacheStats {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	return IdempotencyCacheStats{
		Entries:            len(p.idempotencyCache),
		Capacity:           p.idempotencyMaxEntries(),
		CapacityEvictions:  p.idempotencyEvictions,
		CapacityRejections: p.idempotencyRejections,
	}
}

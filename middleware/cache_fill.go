package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
)

// cacheFillGroup coordinates only concurrent misses; the cache remains the
// storage owner. The leader runs on its own context, without a background
// worker. If it fails or is canceled, waiters retry independently.
type cacheFillGroup struct {
	mu      sync.Mutex
	flights map[string]*cacheFill
}

type cacheFill struct {
	done  chan struct{}
	value any // detached from the leader's response
	ok    bool
}

func (g *cacheFillGroup) join(key, identity string, limit int) (*cacheFill, bool) {
	// Never retain the caller's identity (which may contain sensitive data).
	key = fmt.Sprintf("%x:%s", sha256.Sum256([]byte(identity)), key)
	g.mu.Lock()
	defer g.mu.Unlock()
	if flight := g.flights[key]; flight != nil {
		return flight, false
	}
	if len(g.flights) >= limit {
		return nil, false
	}
	if g.flights == nil {
		g.flights = make(map[string]*cacheFill)
	}
	flight := &cacheFill{done: make(chan struct{})}
	g.flights[key] = flight
	return flight, true
}

func (g *cacheFillGroup) finish(key, identity string, flight *cacheFill, value any, ok bool) {
	key = fmt.Sprintf("%x:%s", sha256.Sum256([]byte(identity)), key)
	g.mu.Lock()
	flight.value, flight.ok = value, ok
	delete(g.flights, key)
	close(flight.done)
	g.mu.Unlock()
}

func waitCacheFill(ctx context.Context, flight *cacheFill) (any, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		return flight.value, flight.ok, nil
	}
}

package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// settleIdempotencyPanic wakes followers without replaying an uncertain effect.
// It runs directly as a deferred call so the original owner panic is preserved.
func (p *Wormhole) settleIdempotencyPanic(entry *idempotencyEntry, ttl time.Duration) {
	if payload := recover(); payload != nil {
		select {
		case <-entry.ready:
		default:
			entry.err = types.NewWormholeError(types.ErrorCodeUnknown, "idempotent request owner panicked", false)
			p.completeIdempotencyEntry(entry, time.Now(), ttl)
			close(entry.ready)
		}
		panic(payload)
	}
}

package wormhole

import (
	"container/ring"
	"context"
	"time"
)

// resetTracking clears old samples after capacity change
func (s *ProviderAdaptiveState) resetTracking() {
	s.latencies = make([]time.Duration, 0, len(s.latencies))
	s.latencyRing = ring.New(s.latencyRing.Len())
	s.totalLatency = 0
	s.latencySamples = 0
	s.pidController.Reset()
}

// carryOccupancy reserves room on newLimiter for operations still in
// flight on oldLimiter when capacity shrinks, then releases those
// reservations as oldLimiter drains. Without this, requests already
// running against oldLimiter keep occupying real resources while
// newLimiter -- created empty -- hands out up to its full capacity on
// top of them, so actual concurrency can run to oldInFlight+newCapacity
// instead of the intended newCapacity ceiling.
func carryOccupancy(oldLimiter, newLimiter *ConcurrencyLimiter) {
	inFlight := oldLimiter.InUse()
	if inFlight <= 0 {
		return
	}

	reserve := min(inFlight, newLimiter.Capacity())
	acquired := 0
	for ; acquired < reserve; acquired++ {
		if !newLimiter.Acquire(context.Background()) {
			break
		}
	}
	if acquired == 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.Now().Add(5 * time.Minute)
		released := 0
		for released < acquired && time.Now().Before(deadline) {
			<-ticker.C
			drained := inFlight - oldLimiter.InUse()
			for drained > released && released < acquired {
				newLimiter.Release()
				released++
			}
		}
		for released < acquired {
			newLimiter.Release()
			released++
		}
	}()
}

package wormhole

import (
	"container/ring"
	"time"
)

// resetTracking clears old samples after capacity change
func (s *ProviderAdaptiveState) resetTracking() {
	s.latencies = make([]time.Duration, 0, len(s.latencies))
	s.latencyRing = ring.New(s.latencyRing.Len())
	s.totalLatency = 0
	s.latencySamples = 0
	s.pidController.reset()
}

// resizeLocked replaces the semaphore while retaining all admitted operations.
// Caller holds s.mu. Blocked acquisitions wake and retry the new generation.
func (s *ProviderAdaptiveState) resizeLocked(capacity int) {
	old := s.limiter
	for s.reserved > 0 {
		old.Release()
		s.reserved--
	}
	close(s.generationChanged)
	s.generationChanged = make(chan struct{})
	s.limiter = NewConcurrencyLimiter(capacity)
	s.currentCapacity = capacity
	s.updateReservationsLocked()
}

// updateReservationsLocked keeps room reserved for every retired generation.
// A deep shrink stays fully reserved until actual retired occupancy drops below
// the new capacity, rather than admitting one call for every retired release.
func (s *ProviderAdaptiveState) updateReservationsLocked() {
	retired := 0
	for limiter, count := range s.activeByLimiter {
		if limiter != s.limiter {
			retired += count
		}
	}
	want := min(retired, s.currentCapacity)
	for s.reserved > want {
		s.limiter.Release()
		s.reserved--
	}
	for s.reserved < want {
		// Reservations grow only on resize, when the new semaphore is empty.
		s.limiter.sem <- struct{}{}
		s.reserved++
	}
}

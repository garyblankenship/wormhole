package wormhole

import (
	"context"
	"sync"
	"time"
)

// Capacity returns current capacity
func (s *ProviderAdaptiveState) Capacity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentCapacity
}

// AcquireToken attempts to acquire a slot and returns a release function.
// The release function captures the specific limiter instance used for acquire,
// preventing a race condition if AdjustCapacity swaps the limiter between
// acquire and release.
func (s *ProviderAdaptiveState) AcquireToken(ctx context.Context) (release func(), ok bool) {
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		s.mu.RLock()
		limiter, changed := s.limiter, s.generationChanged
		s.mu.RUnlock()

		select {
		case <-ctx.Done():
			return nil, false
		case <-changed:
			continue
		case limiter.sem <- struct{}{}:
		}

		s.mu.Lock()
		if ctx.Err() != nil || limiter != s.limiter {
			limiter.Release()
			s.mu.Unlock()
			continue
		}
		s.activeByLimiter[limiter]++
		s.lastSeen = time.Now()
		s.mu.Unlock()

		var once sync.Once
		return func() {
			once.Do(func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				limiter.Release()
				s.activeByLimiter[limiter]--
				if s.activeByLimiter[limiter] == 0 {
					delete(s.activeByLimiter, limiter)
				}
				s.updateReservationsLocked()
			})
		}, true
	}
}

// LastSeen returns the last time this state observed activity.
func (s *ProviderAdaptiveState) LastSeen() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSeen
}

// InUse returns the current number of acquired slots for this state.
func (s *ProviderAdaptiveState) InUse() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active := 0
	for _, count := range s.activeByLimiter {
		active += count
	}
	return active
}

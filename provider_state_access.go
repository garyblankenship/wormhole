package wormhole

import (
	"context"
	"time"
)

// Capacity returns current capacity
func (s *ProviderAdaptiveState) Capacity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentCapacity
}

// Limiter returns the current concurrency limiter.
//
// Deprecated: Use AcquireToken instead to prevent race conditions when
// AdjustCapacity swaps the limiter between acquire and release.
func (s *ProviderAdaptiveState) Limiter() *ConcurrencyLimiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limiter
}

// AcquireToken attempts to acquire a slot and returns a release function.
// The release function captures the specific limiter instance used for acquire,
// preventing a race condition if AdjustCapacity swaps the limiter between
// acquire and release.
func (s *ProviderAdaptiveState) AcquireToken(ctx context.Context) (release func(), ok bool) {
	s.mu.RLock()
	limiter := s.limiter
	s.mu.RUnlock()

	if !limiter.Acquire(ctx) {
		return nil, false
	}

	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
	return limiter.Release, true
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
	if s.limiter == nil {
		return 0
	}
	return s.limiter.InUse()
}

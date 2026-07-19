package wormhole

import (
	"math"
	"time"
)

// AdjustCapacity recalculates optimal capacity using PID control
func (s *ProviderAdaptiveState) AdjustCapacity() (newCapacity int, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.latencySamples == 0 {
		return s.currentCapacity, false
	}

	now := time.Now()
	dt := now.Sub(s.lastAdjustment)
	if dt <= 0 {
		dt = 30 * time.Second // Default interval
	}

	// Calculate average latency
	avgLatency := s.totalLatency / time.Duration(s.latencySamples)

	// Calculate error rate
	errorRate := 0.0
	if s.totalSamples > 0 {
		errorRate = float64(s.totalErrors) / float64(s.totalSamples)
	}

	// Compute PID control signal based on latency
	controlSignal := s.pidController.Compute(s.targetLatency, avgLatency, dt)

	// Apply error rate penalty if above threshold
	const errorRateThreshold = 0.1  // 10%
	const errorRateMultiplier = 2.0 // Double sensitivity when errors high

	if errorRate > errorRateThreshold {
		errorRatePenalty := errorRateMultiplier * (errorRate - errorRateThreshold)
		// More aggressive reduction when error rates are high
		controlSignal *= (1.0 + errorRatePenalty)
	}

	// Calculate new capacity
	adjustment := controlSignal
	proposedCapacity := float64(s.currentCapacity) * (1.0 - adjustment)

	// Round to nearest integer and clamp
	newCapacity = int(math.Round(proposedCapacity))
	newCapacity = max(s.minCapacity, min(s.maxCapacity, newCapacity))

	if newCapacity != s.currentCapacity {
		oldCapacity := s.currentCapacity
		oldLimiter := s.limiter
		newLimiter := NewConcurrencyLimiter(newCapacity)
		if newCapacity < oldCapacity {
			carryOccupancy(oldLimiter, newLimiter)
		}
		s.limiter = newLimiter
		s.currentCapacity = newCapacity
		s.lastAdjustment = now

		// Reset tracking after significant change (20% or more)
		if math.Abs(float64(newCapacity-oldCapacity)) > float64(oldCapacity)*0.2 {
			s.resetTracking()
		}

		return newCapacity, true
	}

	s.lastAdjustment = now
	return s.currentCapacity, false
}

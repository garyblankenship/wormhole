package wormhole

import (
	"container/ring"
	"math"
	"sort"
	"sync"
	"time"
)

// ProviderKey uniquely identifies a provider/model combination
type ProviderKey struct {
	Provider string
	Model    string // Empty string for provider-level only
}

// String returns a string representation for map key
func (k ProviderKey) String() string {
	if k.Model == "" {
		return k.Provider
	}
	return k.Provider + ":" + k.Model
}

// ProviderAdaptiveState tracks adaptive control state for a provider/model
type ProviderAdaptiveState struct {
	mu sync.RWMutex

	key ProviderKey

	// Current concurrency limiter
	limiter *ConcurrencyLimiter
	currentCapacity int

	// Latency tracking
	latencies      []time.Duration // Slice for percentiles
	latencyRing    *ring.Ring      // Ring for moving average
	totalLatency   time.Duration
	latencySamples int

	// Error tracking
	errorRates   *ring.Ring // Ring of bools (true = error)
	totalErrors  int64
	totalSamples int64

	// PID controller
	pidController *PIDController

	// Performance targets
	targetLatency time.Duration
	minCapacity   int
	maxCapacity   int

	// Timing
	lastAdjustment time.Time
	lastQueryTime  time.Time

	// Performance metrics
	avgLatency      time.Duration
	errorRate       float64
	p50Latency      time.Duration
	p90Latency      time.Duration
	p99Latency      time.Duration
}

// NewProviderAdaptiveState creates a new state tracker
func NewProviderAdaptiveState(key ProviderKey, targetLatency time.Duration,
	minCapacity, maxCapacity, initialCapacity, windowSize int) *ProviderAdaptiveState {

	if initialCapacity < minCapacity {
		initialCapacity = minCapacity
	}
	if initialCapacity > maxCapacity {
		initialCapacity = maxCapacity
	}

	return &ProviderAdaptiveState{
		key:           key,
		limiter:       NewConcurrencyLimiter(initialCapacity),
		currentCapacity: initialCapacity,
		latencies:     make([]time.Duration, 0, windowSize),
		latencyRing:   ring.New(windowSize),
		errorRates:    ring.New(windowSize),
		pidController: NewPIDController(DefaultPIDConfig()),
		targetLatency: targetLatency,
		minCapacity:   minCapacity,
		maxCapacity:   maxCapacity,
	}
}

// RecordLatency records a completed operation's latency
func (s *ProviderAdaptiveState) RecordLatency(latency time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update latency ring buffer (for moving average)
	if old := s.latencyRing.Value; old != nil {
		s.totalLatency -= old.(time.Duration)
		s.latencySamples--
	}

	s.latencyRing.Value = latency
	s.totalLatency += latency
	s.latencySamples++
	s.latencyRing = s.latencyRing.Next()

	// Update latency slice (for percentiles)
	if len(s.latencies) < cap(s.latencies) {
		s.latencies = append(s.latencies, latency)
	} else {
		// Replace oldest entry (simple FIFO for percentile calculation)
		s.latencies = s.latencies[1:]
		s.latencies = append(s.latencies, latency)
	}

	// Update error ring buffer
	isError := err != nil
	s.totalSamples++
	if isError {
		s.totalErrors++
	}

	if old := s.errorRates.Value; old != nil {
		if old.(bool) {
			s.totalErrors--
		}
	}

	s.errorRates.Value = isError
	s.errorRates = s.errorRates.Next()
}

// GetMetrics returns current performance metrics
func (s *ProviderAdaptiveState) GetMetrics() (avgLatency time.Duration, errorRate float64,
	p50, p90, p99 time.Duration) {

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Calculate average latency
	if s.latencySamples > 0 {
		avgLatency = s.totalLatency / time.Duration(s.latencySamples)
	}

	// Calculate error rate
	if s.totalSamples > 0 {
		errorRate = float64(s.totalErrors) / float64(s.totalSamples)
	}

	// Calculate percentiles if we have enough data
	if len(s.latencies) > 0 {
		sortedLatencies := make([]time.Duration, len(s.latencies))
		copy(sortedLatencies, s.latencies)
		sort.Slice(sortedLatencies, func(i, j int) bool {
			return sortedLatencies[i] < sortedLatencies[j]
		})

		p50Index := int(float64(len(sortedLatencies)) * 0.5)
		p90Index := int(float64(len(sortedLatencies)) * 0.9)
		p99Index := int(float64(len(sortedLatencies)) * 0.99)

		if p50Index < len(sortedLatencies) {
			p50 = sortedLatencies[p50Index]
		}
		if p90Index < len(sortedLatencies) {
			p90 = sortedLatencies[p90Index]
		}
		if p99Index < len(sortedLatencies) {
			p99 = sortedLatencies[p99Index]
		}
	}

	return avgLatency, errorRate, p50, p90, p99
}

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
	const errorRateThreshold = 0.1 // 10%
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
		s.limiter = NewConcurrencyLimiter(newCapacity)
		s.currentCapacity = newCapacity
		s.lastAdjustment = now

		// Reset tracking after significant change (20% or more)
		if math.Abs(float64(newCapacity-s.currentCapacity)) > float64(s.currentCapacity)*0.2 {
			s.resetTracking()
		}

		return newCapacity, true
	}

	s.lastAdjustment = now
	return s.currentCapacity, false
}

// resetTracking clears old samples after capacity change
func (s *ProviderAdaptiveState) resetTracking() {
	s.latencies = make([]time.Duration, 0, len(s.latencies))
	s.latencyRing = ring.New(s.latencyRing.Len())
	s.totalLatency = 0
	s.latencySamples = 0
	s.pidController.Reset()
}

// Capacity returns current capacity
func (s *ProviderAdaptiveState) Capacity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentCapacity
}

// Limiter returns the current concurrency limiter
func (s *ProviderAdaptiveState) Limiter() *ConcurrencyLimiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limiter
}
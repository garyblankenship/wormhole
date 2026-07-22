package wormhole

import (
	"container/ring"
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
	// pins is protected by the owning EnhancedAdaptiveLimiter.mu. It keeps a
	// state map-owned while admission, release, or latency mutation is active.
	pins int

	key ProviderKey

	// Current concurrency limiter
	limiter         *ConcurrencyLimiter
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
	lastSeen       time.Time
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
		key:             key,
		limiter:         NewConcurrencyLimiter(initialCapacity),
		currentCapacity: initialCapacity,
		latencies:       make([]time.Duration, 0, windowSize),
		latencyRing:     ring.New(windowSize),
		errorRates:      ring.New(windowSize),
		pidController:   NewPIDController(DefaultPIDConfig()),
		targetLatency:   targetLatency,
		minCapacity:     minCapacity,
		maxCapacity:     maxCapacity,
		lastSeen:        time.Now(),
	}
}

// RecordLatency records a completed operation's latency
func (s *ProviderAdaptiveState) RecordLatency(latency time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeen = time.Now()

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

	// Update error ring buffer. totalSamples/totalErrors are windowed to
	// match errorRates' capacity: each new sample evicts the oldest one
	// from the window, so the error rate reflects recent behavior instead
	// of decaying toward 0 as totalSamples grows unbounded over uptime.
	isError := err != nil
	s.totalSamples++
	if isError {
		s.totalErrors++
	}

	if old := s.errorRates.Value; old != nil {
		s.totalSamples--
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

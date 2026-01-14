package wormhole

import (
	"context"
	"container/ring"
	"sync"
	"time"
)

// AdaptiveConfig holds configuration for adaptive concurrency limiting.
type AdaptiveConfig struct {
	// TargetLatency is the desired average latency for operations.
	// If actual latency exceeds this, capacity will be reduced.
	TargetLatency time.Duration

	// MinCapacity is the minimum allowed concurrent operations.
	MinCapacity int

	// MaxCapacity is the maximum allowed concurrent operations.
	MaxCapacity int

	// InitialCapacity is the starting capacity.
	InitialCapacity int

	// AdjustmentInterval is how often to evaluate and adjust capacity.
	AdjustmentInterval time.Duration

	// LatencyWindowSize is the number of recent latencies to consider.
	LatencyWindowSize int
}

// DefaultAdaptiveConfig returns a sensible default adaptive configuration.
func DefaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		TargetLatency:      500 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        100,
		InitialCapacity:    10,
		AdjustmentInterval: 30 * time.Second,
		LatencyWindowSize:  100,
	}
}

// AdaptiveLimiter implements concurrency limiting with automatic capacity
// adjustment based on observed operation latencies.
type AdaptiveLimiter struct {
	mu      sync.RWMutex
	limiter *ConcurrencyLimiter
	config  AdaptiveConfig

	latencies *ring.Ring // ring buffer of recent latencies
	totalLatency time.Duration
	sampleCount int

	stopChan  chan struct{}
	stopOnce  sync.Once
}

// NewAdaptiveLimiter creates a new adaptive limiter with the given configuration.
func NewAdaptiveLimiter(config AdaptiveConfig) *AdaptiveLimiter {
	if config.InitialCapacity < config.MinCapacity {
		config.InitialCapacity = config.MinCapacity
	}
	if config.InitialCapacity > config.MaxCapacity {
		config.InitialCapacity = config.MaxCapacity
	}

	al := &AdaptiveLimiter{
		limiter:    NewConcurrencyLimiter(config.InitialCapacity),
		config:     config,
		latencies:  ring.New(config.LatencyWindowSize),
		stopChan:   make(chan struct{}),
	}

	// Start adjustment goroutine
	go al.adjustmentLoop()

	return al
}

// Acquire attempts to acquire a slot in the limiter.
// Returns true if acquired, false if context expired or cancelled.
func (al *AdaptiveLimiter) Acquire(ctx context.Context) bool {
	al.mu.RLock()
	limiter := al.limiter
	al.mu.RUnlock()

	return limiter.Acquire(ctx)
}

// Release releases a slot in the limiter.
func (al *AdaptiveLimiter) Release() {
	al.mu.RLock()
	limiter := al.limiter
	al.mu.RUnlock()

	limiter.Release()
}

// RecordLatency records the latency of a completed operation.
// Call this after Release() with the total operation duration.
func (al *AdaptiveLimiter) RecordLatency(latency time.Duration) {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Add to ring buffer, removing old latency if needed
	if old := al.latencies.Value; old != nil {
		al.totalLatency -= old.(time.Duration)
		al.sampleCount--
	}

	al.latencies.Value = latency
	al.totalLatency += latency
	al.sampleCount++
	al.latencies = al.latencies.Next()
}

// adjustmentLoop periodically evaluates performance and adjusts capacity.
func (al *AdaptiveLimiter) adjustmentLoop() {
	ticker := time.NewTicker(al.config.AdjustmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			al.adjustCapacity()
		case <-al.stopChan:
			return
		}
	}
}

// adjustCapacity evaluates recent latencies and adjusts capacity if needed.
func (al *AdaptiveLimiter) adjustCapacity() {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.sampleCount == 0 {
		return // No data yet
	}

	averageLatency := al.totalLatency / time.Duration(al.sampleCount)
	currentCapacity := al.limiter.Capacity() // Need to expose capacity method

	newCapacity := currentCapacity

	// Simple proportional control: if latency > target, reduce capacity; if less, increase
	if averageLatency > al.config.TargetLatency {
		// Reduce capacity by 1, but not below min
		newCapacity = max(al.config.MinCapacity, currentCapacity-1)
	} else {
		// Increase capacity by 1, but not above max
		newCapacity = min(al.config.MaxCapacity, currentCapacity+1)
	}

	if newCapacity != currentCapacity {
		// Create new limiter with adjusted capacity
		al.limiter = NewConcurrencyLimiter(newCapacity)
		// Reset latency tracking to avoid reacting to old data
		al.latencies = ring.New(al.config.LatencyWindowSize)
		al.totalLatency = 0
		al.sampleCount = 0
	}
}

// Stop stops the adjustment goroutine.
func (al *AdaptiveLimiter) Stop() {
	al.stopOnce.Do(func() {
		close(al.stopChan)
	})
}

// helper functions (Go 1.21+ has these in cmp package)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
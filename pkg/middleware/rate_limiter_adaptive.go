package middleware

import (
	"math"
	"sync"
	"time"
)

// HealthMetrics represents provider health metrics for adaptive rate limiting.
type HealthMetrics struct {
	CircuitState     CircuitState
	Healthy          bool
	ErrorRate        float64
	ResponseTime     time.Duration
	ConsecutiveFails int
	LastCheck        time.Time
}

// AdaptiveRateLimiter adjusts rate based on response times and health metrics.
type AdaptiveRateLimiter struct {
	*RateLimiter
	mu             sync.RWMutex
	minRate        int
	maxRate        int
	targetLatency  time.Duration
	latencyWindow  []time.Duration
	windowSize     int
	totalLatency   time.Duration
	adjustInterval time.Duration
	lastAdjustment time.Time
	healthMetrics  *HealthMetrics
	useHealthAware bool
}

type adaptiveLimiterConfig struct {
	initialRate    int
	minRate        int
	maxRate        int
	targetLatency  time.Duration
	useHealthAware bool
}

// NewAdaptiveRateLimiter creates a rate limiter that adjusts based on latency.
func NewAdaptiveRateLimiter(initialRate, minRate, maxRate int, targetLatency time.Duration) *AdaptiveRateLimiter {
	return newAdaptiveRateLimiter(adaptiveLimiterConfig{
		initialRate:   initialRate,
		minRate:       minRate,
		maxRate:       maxRate,
		targetLatency: targetLatency,
	})
}

// NewHealthAwareAdaptiveRateLimiter creates a rate limiter that adjusts based on both latency and health metrics.
func NewHealthAwareAdaptiveRateLimiter(initialRate, minRate, maxRate int, targetLatency time.Duration) *AdaptiveRateLimiter {
	return newAdaptiveRateLimiter(adaptiveLimiterConfig{
		initialRate:    initialRate,
		minRate:        minRate,
		maxRate:        maxRate,
		targetLatency:  targetLatency,
		useHealthAware: true,
	})
}

func newAdaptiveRateLimiter(config adaptiveLimiterConfig) *AdaptiveRateLimiter {
	limiter := &AdaptiveRateLimiter{
		RateLimiter:    NewRateLimiter(config.initialRate),
		minRate:        config.minRate,
		maxRate:        config.maxRate,
		targetLatency:  config.targetLatency,
		latencyWindow:  make([]time.Duration, 0, 100),
		windowSize:     100,
		adjustInterval: 10 * time.Second,
		lastAdjustment: time.Now(),
		useHealthAware: config.useHealthAware,
	}
	if config.useHealthAware {
		limiter.healthMetrics = &HealthMetrics{}
	}
	return limiter
}

// RecordLatency records a request latency and adjusts rate if needed.
func (arl *AdaptiveRateLimiter) RecordLatency(latency time.Duration) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	if len(arl.latencyWindow) >= arl.windowSize {
		arl.totalLatency -= arl.latencyWindow[0]
		copy(arl.latencyWindow, arl.latencyWindow[1:])
		arl.latencyWindow = arl.latencyWindow[:len(arl.latencyWindow)-1]
	}
	arl.latencyWindow = append(arl.latencyWindow, latency)
	arl.totalLatency += latency

	if time.Since(arl.lastAdjustment) < arl.adjustInterval {
		return
	}
	if len(arl.latencyWindow) < arl.windowSize/2 {
		return
	}

	arl.adjustRate(arl.totalLatency / time.Duration(len(arl.latencyWindow)))
	arl.lastAdjustment = time.Now()
}

// RecordHealthMetrics updates the health metrics for the adaptive rate limiter.
func (arl *AdaptiveRateLimiter) RecordHealthMetrics(metrics *HealthMetrics) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	if arl.healthMetrics != nil {
		arl.healthMetrics = metrics
	}
}

func (arl *AdaptiveRateLimiter) adjustRate(avgLatency time.Duration) {
	adjustmentFactor := 1.0

	if avgLatency > arl.targetLatency*120/100 {
		adjustmentFactor *= 0.9
	} else if avgLatency < arl.targetLatency*80/100 {
		adjustmentFactor *= 1.1
	}

	if arl.useHealthAware && arl.healthMetrics != nil {
		adjustmentFactor *= arl.calculateHealthAdjustment()
	}

	newRate := int(float64(arl.rate) * adjustmentFactor)
	if newRate < arl.minRate {
		newRate = arl.minRate
	} else if newRate > arl.maxRate {
		newRate = arl.maxRate
	}

	arl.rate = newRate
}

func (arl *AdaptiveRateLimiter) calculateHealthAdjustment() float64 {
	if arl.healthMetrics == nil {
		return 1.0
	}

	healthScore := arl.calculateHealthScore()
	if healthScore < 0.3 {
		return 0.5
	} else if healthScore < 0.6 {
		return 0.75
	} else if healthScore < 0.8 {
		return 0.9
	}
	return 1.0
}

func (arl *AdaptiveRateLimiter) calculateHealthScore() float64 {
	if arl.healthMetrics == nil {
		return 1.0
	}

	score := 0.0
	totalWeight := 0

	circuitWeight := 4
	switch arl.healthMetrics.CircuitState {
	case StateClosed:
		score += 1.0 * float64(circuitWeight)
	case StateHalfOpen:
		score += 0.5 * float64(circuitWeight)
	case StateOpen:
		score += 0.0
	}
	totalWeight += circuitWeight

	healthWeight := 3
	if arl.healthMetrics.Healthy {
		score += 1.0 * float64(healthWeight)
	}
	totalWeight += healthWeight

	errorWeight := 2
	errorScore := 1.0 - arl.healthMetrics.ErrorRate
	if errorScore < 0 {
		errorScore = 0
	}
	score += errorScore * float64(errorWeight)
	totalWeight += errorWeight

	failWeight := 1
	failScore := 1.0
	if arl.healthMetrics.ConsecutiveFails > 0 {
		failScore = 1.0 / math.Pow(2.0, float64(arl.healthMetrics.ConsecutiveFails))
	}
	score += failScore * float64(failWeight)
	totalWeight += failWeight

	if totalWeight > 0 {
		return score / float64(totalWeight)
	}
	return 1.0
}

// Close releases adaptive rate limiter resources.
func (arl *AdaptiveRateLimiter) Close() error {
	return arl.RateLimiter.Close()
}

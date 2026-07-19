package wormhole

import (
	"sync/atomic"
)

// Stop stops the adjustment goroutines
func (l *EnhancedAdaptiveLimiter) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopChan)
		l.wg.Wait()
	})
}

// GetStats returns statistics about the limiter's operation
func (l *EnhancedAdaptiveLimiter) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := make(map[string]interface{})

	// Global stats
	avgLatency, errorRate, p50, p90, p99 := l.globalState.GetMetrics()
	stats["global"] = map[string]interface{}{
		"capacity":    l.globalState.Capacity(),
		"avg_latency": avgLatency.String(),
		"error_rate":  errorRate,
		"p50_latency": p50.String(),
		"p90_latency": p90.String(),
		"p99_latency": p99.String(),
		"adjustments": atomic.LoadInt64(&l.totalAdjustments),
	}

	// Provider stats
	providerStats := make(map[string]interface{})
	for provider, state := range l.providerStates {
		avgLatency, errorRate, p50, p90, p99 := state.GetMetrics()
		providerStats[provider] = map[string]interface{}{
			"capacity":    state.Capacity(),
			"avg_latency": avgLatency.String(),
			"error_rate":  errorRate,
			"p50_latency": p50.String(),
			"p90_latency": p90.String(),
			"p99_latency": p99.String(),
		}
	}
	stats["providers"] = providerStats

	// Model stats (if enabled)
	if l.config.EnableModelLevel {
		modelStats := make(map[string]interface{})
		for key, state := range l.modelStates {
			avgLatency, errorRate, p50, p90, p99 := state.GetMetrics()
			modelStats[key] = map[string]interface{}{
				"capacity":    state.Capacity(),
				"avg_latency": avgLatency.String(),
				"error_rate":  errorRate,
				"p50_latency": p50.String(),
				"p90_latency": p90.String(),
				"p99_latency": p99.String(),
			}
		}
		stats["models"] = modelStats
	}

	return stats
}

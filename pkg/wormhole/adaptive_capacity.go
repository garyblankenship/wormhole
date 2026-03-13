package wormhole

import (
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
)

// capacityManager handles capacity adjustments and state eviction
type capacityManager struct {
	config  EnhancedAdaptiveConfig
	limiter *EnhancedAdaptiveLimiter
}

func (c *capacityManager) Start(stopChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(c.config.AdjustmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.adjustAllCapacities()
		case <-stopChan:
			return
		}
	}
}

// adjustAllCapacities adjusts capacity for all tracked states
func (c *capacityManager) adjustAllCapacities() {
	l := c.limiter
	l.mu.RLock()

	// Adjust global state
	globalCapacity, globalChanged := l.globalState.AdjustCapacity()
	if globalChanged {
		atomic.AddInt64(&l.totalAdjustments, 1)
	}

	// Adjust provider states
	providerAdjustments := make(map[string]int)
	for provider, state := range l.providerStates {
		newCapacity, changed := state.AdjustCapacity()
		if changed {
			atomic.AddInt64(&l.totalAdjustments, 1)
			providerAdjustments[provider] = newCapacity
		}
	}

	// Adjust model states (if enabled)
	modelAdjustments := make(map[string]int)
	if c.config.EnableModelLevel {
		for key, state := range l.modelStates {
			newCapacity, changed := state.AdjustCapacity()
			if changed {
				atomic.AddInt64(&l.totalAdjustments, 1)
				modelAdjustments[key] = newCapacity
			}
		}
	}

	l.mu.RUnlock()

	// Log adjustments if any occurred
	if len(providerAdjustments) > 0 || len(modelAdjustments) > 0 {
		c.logAdjustments(globalCapacity, providerAdjustments, modelAdjustments)
	}

	c.evictIdleStates()
}

func (c *capacityManager) evictIdleStates() {
	if c.config.IdleStateTTL <= 0 {
		return
	}

	cutoff := time.Now().Add(-c.config.IdleStateTTL)
	l := c.limiter

	l.mu.Lock()
	defer l.mu.Unlock()

	for provider, state := range l.providerStates {
		if _, pinned := c.config.ProviderSettings[provider]; pinned {
			continue
		}
		if state.InUse() > 0 || state.LastSeen().After(cutoff) {
			continue
		}
		delete(l.providerStates, provider)
	}

	if !c.config.EnableModelLevel {
		return
	}

	for key, state := range l.modelStates {
		if state.InUse() > 0 || state.LastSeen().After(cutoff) {
			continue
		}
		delete(l.modelStates, key)
	}

	if c.config.MaxModelStates <= 0 || len(l.modelStates) <= c.config.MaxModelStates {
		return
	}

	type stateInfo struct {
		key      string
		lastSeen time.Time
	}

	candidates := make([]stateInfo, 0, len(l.modelStates))
	for key, state := range l.modelStates {
		if state.InUse() > 0 {
			continue
		}
		candidates = append(candidates, stateInfo{key: key, lastSeen: state.LastSeen()})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastSeen.Before(candidates[j].lastSeen)
	})

	for len(l.modelStates) > c.config.MaxModelStates && len(candidates) > 0 {
		candidate := candidates[0]
		candidates = candidates[1:]
		delete(l.modelStates, candidate.key)
	}
}

// logAdjustments logs capacity adjustments
func (c *capacityManager) logAdjustments(globalCapacity int,
	providerAdjustments map[string]int, modelAdjustments map[string]int) {

	log.Printf("[EnhancedAdaptiveLimiter] Adjustments - Global: %d", globalCapacity)

	for provider, capacity := range providerAdjustments {
		log.Printf("[EnhancedAdaptiveLimiter] Provider %s: %d", provider, capacity)
	}

	for model, capacity := range modelAdjustments {
		log.Printf("[EnhancedAdaptiveLimiter] Model %s: %d", model, capacity)
	}
}

// metricsObserver handles observing external metrics
type metricsObserver struct {
	config           EnhancedAdaptiveConfig
	metricsCollector *middleware.EnhancedMetricsCollector
	limiter          *EnhancedAdaptiveLimiter
}

func (m *metricsObserver) Start(stopChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	if m.config.QueryInterval <= 0 {
		return
	}

	ticker := time.NewTicker(m.config.QueryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.queryExternalMetrics()
		case <-stopChan:
			return
		}
	}
}

// queryExternalMetrics queries external metrics for enhanced control
func (m *metricsObserver) queryExternalMetrics() {
	if m.metricsCollector == nil {
		return
	}

	// Get all metrics from the collector
	allStats := m.metricsCollector.GetAllStats()

	// Extract per-provider metrics if available
	if perLabelStats, ok := allStats["per_label"].(map[string]interface{}); ok {
		for labelKey, stats := range perLabelStats {
			// Parse provider and model from label key
			// Label format: "provider:model:method:errorType"
			parts := splitLabelKey(labelKey)
			if len(parts) >= 2 {
				provider := parts[0]
				model := parts[1]

				// Get state for this provider/model
				state := m.limiter.getState(provider, model)
				if state == nil {
					continue
				}

				// Extract metrics and potentially adjust PID parameters
				if statsMap, ok := stats.(map[string]interface{}); ok {
					m.enhanceControlWithMetrics(state, statsMap)
				}
			}
		}
	}
}

// splitLabelKey splits a label key into its components
func splitLabelKey(key string) []string {
	// Format: "provider:model:method:errorType"
	return strings.SplitN(key, ":", 4)
}

// enhanceControlWithMetrics enhances control with external metrics
func (m *metricsObserver) enhanceControlWithMetrics(_ *ProviderAdaptiveState,
	stats map[string]interface{}) {

	// Extract error rate from metrics
	if errors, ok := stats["errors"].(int64); ok {
		if requests, ok := stats["requests"].(int64); ok && requests > 0 {
			errorRate := float64(errors) / float64(requests)

			// If error rate is persistently high, we might adjust PID parameters
			if errorRate > m.config.ErrorRateThreshold {
				// Could adjust PID gains here based on error rate
				// For now, we just record it
				log.Printf("[EnhancedAdaptiveLimiter] High error rate detected: %.2f", errorRate)
			}
		}
	}
}

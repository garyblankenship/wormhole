package wormhole

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
)

// EnhancedAdaptiveConfig extends AdaptiveConfig with provider awareness
type EnhancedAdaptiveConfig struct {
	// Base configuration
	AdaptiveConfig

	// Provider-specific settings (override base config)
	ProviderSettings map[string]ProviderSetting

	// Error rate handling
	ErrorRateThreshold float64 // e.g., 0.1 = 10%
	ErrorRatePenalty   float64 // e.g., 2.0 = double sensitivity
	MinSamplesForError int     // Minimum samples before considering error rates

	// Metrics integration
	MetricsCollector *middleware.EnhancedMetricsCollector
	QueryInterval    time.Duration // How often to query external metrics

	// PID tuning
	PIDConfig PIDConfig

	// State management
	EnableModelLevel bool   // Track per-model vs per-provider only
	PersistenceFile  string // Optional: save/load state
	IdleStateTTL     time.Duration
	MaxModelStates   int
}

// ProviderSetting holds provider-specific configuration
type ProviderSetting struct {
	TargetLatency   time.Duration
	MinCapacity     int
	MaxCapacity     int
	InitialCapacity int
	// Optional provider-specific PID tuning
	PIDConfig *PIDConfig // nil = use global PIDConfig
}

// DefaultEnhancedAdaptiveConfig returns sensible defaults
func DefaultEnhancedAdaptiveConfig() EnhancedAdaptiveConfig {
	return EnhancedAdaptiveConfig{
		AdaptiveConfig:     DefaultAdaptiveConfig(),
		ProviderSettings:   make(map[string]ProviderSetting),
		ErrorRateThreshold: 0.1, // 10%
		ErrorRatePenalty:   2.0, // Double sensitivity
		MinSamplesForError: 20,
		QueryInterval:      15 * time.Second,
		PIDConfig:          DefaultPIDConfig(),
		EnableModelLevel:   false, // Start with provider-level only
		IdleStateTTL:       time.Hour,
		MaxModelStates:     1024,
	}
}

// EnhancedAdaptiveLimiter implements provider-aware adaptive concurrency control
type EnhancedAdaptiveLimiter struct {
	mu     sync.RWMutex
	config EnhancedAdaptiveConfig

	// Provider state management
	providerStates map[string]*ProviderAdaptiveState // key: provider
	modelStates    map[string]*ProviderAdaptiveState // key: provider:model

	// Global fallback (for requests without provider info)
	globalState *ProviderAdaptiveState

	// Metrics collector reference
	metricsCollector *middleware.EnhancedMetricsCollector

	// Control goroutines
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	// Statistics
	totalAdjustments int64
}

// NewEnhancedAdaptiveLimiter creates a new provider-aware adaptive limiter
func NewEnhancedAdaptiveLimiter(config EnhancedAdaptiveConfig) *EnhancedAdaptiveLimiter {
	if config.MetricsCollector == nil {
		// Create a default metrics collector if none provided
		config.MetricsCollector = middleware.NewEnhancedMetricsCollector(nil)
	}

	limiter := &EnhancedAdaptiveLimiter{
		config:           config,
		providerStates:   make(map[string]*ProviderAdaptiveState),
		modelStates:      make(map[string]*ProviderAdaptiveState),
		metricsCollector: config.MetricsCollector,
		stopChan:         make(chan struct{}),
	}

	// Initialize global state
	limiter.globalState = NewProviderAdaptiveState(
		ProviderKey{Provider: "global"},
		config.TargetLatency,
		config.MinCapacity,
		config.MaxCapacity,
		config.InitialCapacity,
		config.LatencyWindowSize,
	)

	// Initialize provider-specific settings
	for provider, setting := range config.ProviderSettings {
		pidConfig := config.PIDConfig
		if setting.PIDConfig != nil {
			pidConfig = *setting.PIDConfig
		}

		state := NewProviderAdaptiveState(
			ProviderKey{Provider: provider},
			setting.TargetLatency,
			setting.MinCapacity,
			setting.MaxCapacity,
			setting.InitialCapacity,
			config.LatencyWindowSize,
		)
		state.pidController = NewPIDController(pidConfig)
		limiter.providerStates[provider] = state
	}

	// Initialize sub-managers
	metricsObserver := &metricsObserver{
		config:           config,
		metricsCollector: limiter.metricsCollector,
		limiter:          limiter,
	}
	capacityManager := &capacityManager{
		config:  config,
		limiter: limiter,
	}

	// Start adjustment goroutines
	limiter.wg.Add(2)
	go capacityManager.Start(limiter.stopChan, &limiter.wg)
	if config.QueryInterval > 0 {
		go metricsObserver.Start(limiter.stopChan, &limiter.wg)
	} else {
		limiter.wg.Done()
	}

	return limiter
}

// Acquire acquires a slot from the global limiter (backward compatible).
//
// Deprecated: Use AcquireToken instead to prevent race conditions when
// capacity adjustment swaps the limiter between acquire and release.
// This method now delegates to AcquireToken internally for safety, but
// callers must still pair with Release() which may hit a different limiter.
func (l *EnhancedAdaptiveLimiter) Acquire(ctx context.Context) bool {
	_, ok := l.globalState.AcquireToken(ctx)
	return ok
}

// AcquireWithProvider acquires a slot with provider/model awareness.
//
// Deprecated: Use AcquireTokenWithProvider instead to prevent race conditions.
// This method now delegates to AcquireToken internally for safety, but
// callers must still pair with ReleaseWithProvider() which may hit a different limiter.
func (l *EnhancedAdaptiveLimiter) AcquireWithProvider(ctx context.Context, provider, model string) bool {
	state := l.getOrCreateState(provider, model)
	_, ok := state.AcquireToken(ctx)
	return ok
}

// Release releases a slot to the global limiter.
//
// Deprecated: Use the release function returned by AcquireToken instead.
func (l *EnhancedAdaptiveLimiter) Release() {
	l.globalState.Limiter().Release()
}

// ReleaseWithProvider releases a slot with provider/model awareness.
//
// Deprecated: Use the release function returned by AcquireTokenWithProvider instead.
func (l *EnhancedAdaptiveLimiter) ReleaseWithProvider(provider, model string) {
	state := l.getState(provider, model)
	if state != nil {
		state.Limiter().Release()
	} else {
		l.globalState.Limiter().Release()
	}
}

// AcquireToken acquires a slot from the global limiter and returns a release function.
// The release function captures the specific limiter instance, preventing race conditions
// if capacity adjustment swaps the limiter between acquire and release.
func (l *EnhancedAdaptiveLimiter) AcquireToken(ctx context.Context) (release func(), ok bool) {
	return l.globalState.AcquireToken(ctx)
}

// AcquireTokenWithProvider acquires a slot with provider/model awareness and returns
// a release function. The release function captures the specific limiter instance,
// preventing race conditions if capacity adjustment swaps the limiter.
func (l *EnhancedAdaptiveLimiter) AcquireTokenWithProvider(ctx context.Context, provider, model string) (release func(), ok bool) {
	state := l.getOrCreateState(provider, model)
	return state.AcquireToken(ctx)
}

// RecordLatency records latency for global limiter
func (l *EnhancedAdaptiveLimiter) RecordLatency(latency time.Duration) {
	l.globalState.RecordLatency(latency, nil)
}

// RecordLatencyWithProvider records latency with provider/model and error info
func (l *EnhancedAdaptiveLimiter) RecordLatencyWithProvider(latency time.Duration,
	provider, model string, err error) {

	state := l.getOrCreateState(provider, model)
	state.RecordLatency(latency, err)

	// Also update global state for backward compatibility
	l.globalState.RecordLatency(latency, err)
}

// getOrCreateState gets or creates state for provider/model.
// Uses double-checked locking to prevent duplicate state creation races.
func (l *EnhancedAdaptiveLimiter) getOrCreateState(provider, model string) *ProviderAdaptiveState {
	// Check provider-level state first (if model-level is disabled)
	if !l.config.EnableModelLevel && model != "" {
		// Use provider-level state for all models of this provider
		l.mu.RLock()
		state, exists := l.providerStates[provider]
		l.mu.RUnlock()

		if exists {
			return state
		}

		// Create new provider state with default config
		return l.createProviderState(provider)
	}

	// Create key for model-level tracking
	key := ProviderKey{Provider: provider, Model: model}
	mapKey := key.String()

	l.mu.RLock()
	state, exists := l.modelStates[mapKey]
	l.mu.RUnlock()

	if exists {
		return state
	}

	// Resolve settings outside the lock (config is immutable after init)
	providerSetting, hasProviderSetting := l.config.ProviderSettings[provider]

	var targetLatency time.Duration
	var minCapacity, maxCapacity, initialCapacity int

	if hasProviderSetting {
		targetLatency = providerSetting.TargetLatency
		minCapacity = providerSetting.MinCapacity
		maxCapacity = providerSetting.MaxCapacity
		initialCapacity = providerSetting.InitialCapacity
	} else {
		targetLatency = l.config.TargetLatency
		minCapacity = l.config.MinCapacity
		maxCapacity = l.config.MaxCapacity
		initialCapacity = l.config.InitialCapacity
	}

	pidConfig := l.config.PIDConfig
	if hasProviderSetting && providerSetting.PIDConfig != nil {
		pidConfig = *providerSetting.PIDConfig
	}

	// Create new state
	newState := NewProviderAdaptiveState(
		key,
		targetLatency,
		minCapacity,
		maxCapacity,
		initialCapacity,
		l.config.LatencyWindowSize,
	)
	newState.pidController = NewPIDController(pidConfig)

	// Double-checked locking: re-check under write lock before inserting
	l.mu.Lock()
	if existing, exists := l.modelStates[mapKey]; exists {
		l.mu.Unlock()
		return existing
	}
	l.modelStates[mapKey] = newState
	l.mu.Unlock()

	return newState
}

// createProviderState creates a new provider-level state
func (l *EnhancedAdaptiveLimiter) createProviderState(provider string) *ProviderAdaptiveState {
	// Check if we have provider-specific settings
	providerSetting, hasProviderSetting := l.config.ProviderSettings[provider]

	var targetLatency time.Duration
	var minCapacity, maxCapacity, initialCapacity int

	if hasProviderSetting {
		targetLatency = providerSetting.TargetLatency
		minCapacity = providerSetting.MinCapacity
		maxCapacity = providerSetting.MaxCapacity
		initialCapacity = providerSetting.InitialCapacity
	} else {
		targetLatency = l.config.TargetLatency
		minCapacity = l.config.MinCapacity
		maxCapacity = l.config.MaxCapacity
		initialCapacity = l.config.InitialCapacity
	}

	// Create PID config
	pidConfig := l.config.PIDConfig
	if hasProviderSetting && providerSetting.PIDConfig != nil {
		pidConfig = *providerSetting.PIDConfig
	}

	// Create new state
	state := NewProviderAdaptiveState(
		ProviderKey{Provider: provider},
		targetLatency,
		minCapacity,
		maxCapacity,
		initialCapacity,
		l.config.LatencyWindowSize,
	)
	state.pidController = NewPIDController(pidConfig)

	// Double-checked locking: re-check under write lock before inserting
	l.mu.Lock()
	if existing, exists := l.providerStates[provider]; exists {
		l.mu.Unlock()
		return existing
	}
	l.providerStates[provider] = state
	l.mu.Unlock()

	return state
}

// getState gets existing state for provider/model
func (l *EnhancedAdaptiveLimiter) getState(provider, model string) *ProviderAdaptiveState {
	if !l.config.EnableModelLevel && model != "" {
		l.mu.RLock()
		state, ok := l.providerStates[provider]
		l.mu.RUnlock()
		if !ok {
			return nil
		}
		return state
	}

	key := ProviderKey{Provider: provider, Model: model}
	mapKey := key.String()

	l.mu.RLock()
	state, exists := l.modelStates[mapKey]
	l.mu.RUnlock()

	if exists {
		return state
	}

	// Fallback to provider-level state
	l.mu.RLock()
	state = l.providerStates[provider]
	l.mu.RUnlock()

	return state
}

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

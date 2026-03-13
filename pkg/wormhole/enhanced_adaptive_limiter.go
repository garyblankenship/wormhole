package wormhole

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
)

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
	targetLatency, minCapacity, maxCapacity, initialCapacity, pidConfig := l.resolveProviderSettings(provider)

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
	targetLatency, minCapacity, maxCapacity, initialCapacity, pidConfig := l.resolveProviderSettings(provider)

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

// resolveProviderSettings returns capacity/latency settings for a provider,
// falling back to global config defaults when no provider-specific setting exists.
func (l *EnhancedAdaptiveLimiter) resolveProviderSettings(provider string) (
	targetLatency time.Duration, minCapacity, maxCapacity, initialCapacity int, pidConfig PIDConfig,
) {
	providerSetting, hasProviderSetting := l.config.ProviderSettings[provider]

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

	pidConfig = l.config.PIDConfig
	if hasProviderSetting && providerSetting.PIDConfig != nil {
		pidConfig = *providerSetting.PIDConfig
	}

	return
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

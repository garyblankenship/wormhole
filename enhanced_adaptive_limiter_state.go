package wormhole

import (
	"time"
)

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

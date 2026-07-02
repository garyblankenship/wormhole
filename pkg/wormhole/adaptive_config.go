package wormhole

import (
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

// normalizeEnhancedAdaptiveConfig fills zero-valued tuning fields with safe
// defaults. A caller who partially populates EnhancedAdaptiveConfig (leaving
// AdjustmentInterval unset, for example) would otherwise panic in
// capacityManager.Start's time.NewTicker, or silently fall back to
// ConcurrencyLimiter's 1024-permit unlimited default via a zero capacity.
func normalizeEnhancedAdaptiveConfig(config EnhancedAdaptiveConfig) EnhancedAdaptiveConfig {
	defaults := DefaultEnhancedAdaptiveConfig()
	if config.TargetLatency == 0 {
		config.TargetLatency = defaults.TargetLatency
	}
	if config.MinCapacity == 0 {
		config.MinCapacity = defaults.MinCapacity
	}
	if config.MaxCapacity == 0 {
		config.MaxCapacity = defaults.MaxCapacity
	}
	if config.InitialCapacity == 0 {
		config.InitialCapacity = defaults.InitialCapacity
	}
	if config.AdjustmentInterval == 0 {
		config.AdjustmentInterval = defaults.AdjustmentInterval
	}
	if config.LatencyWindowSize == 0 {
		config.LatencyWindowSize = defaults.LatencyWindowSize
	}
	if config.PIDConfig == (PIDConfig{}) {
		config.PIDConfig = defaults.PIDConfig
	}
	return config
}

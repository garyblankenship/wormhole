package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
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
		EnableModelLevel:   false, // Start with provider-level only
		IdleStateTTL:       time.Hour,
		MaxModelStates:     1024,
	}
}

// normalizeAdaptiveConfig resolves an AdaptiveConfig into a safe capacity
// range. Exported constructors use it so their zero values have the same
// behavior as the configuration path on Wormhole.
func normalizeAdaptiveConfig(config AdaptiveConfig) AdaptiveConfig {
	defaults := DefaultAdaptiveConfig()
	if config.TargetLatency <= 0 {
		config.TargetLatency = defaults.TargetLatency
	}
	if config.MinCapacity <= 0 {
		config.MinCapacity = defaults.MinCapacity
	}
	if config.MaxCapacity <= 0 {
		config.MaxCapacity = defaults.MaxCapacity
	}
	if config.InitialCapacity <= 0 {
		config.InitialCapacity = defaults.InitialCapacity
	}
	if config.AdjustmentInterval <= 0 {
		config.AdjustmentInterval = defaults.AdjustmentInterval
	}
	if config.LatencyWindowSize <= 0 {
		config.LatencyWindowSize = defaults.LatencyWindowSize
	}
	if config.MaxCapacity < config.MinCapacity {
		config.MaxCapacity = config.MinCapacity
	}
	if config.InitialCapacity < config.MinCapacity {
		config.InitialCapacity = config.MinCapacity
	}
	if config.InitialCapacity > config.MaxCapacity {
		config.InitialCapacity = config.MaxCapacity
	}
	return config
}

// normalizeEnhancedAdaptiveConfig resolves the base configuration and every
// provider setting into safe values. It copies ProviderSettings before writing
// resolved entries so callers can reuse their input configuration.
func normalizeEnhancedAdaptiveConfig(config EnhancedAdaptiveConfig) EnhancedAdaptiveConfig {
	config.AdaptiveConfig = normalizeAdaptiveConfig(config.AdaptiveConfig)
	if config.ProviderSettings == nil {
		return config
	}

	providerSettings := make(map[string]ProviderSetting, len(config.ProviderSettings))
	for provider, setting := range config.ProviderSettings {
		if setting.TargetLatency <= 0 {
			setting.TargetLatency = config.TargetLatency
		}
		if setting.MinCapacity <= 0 {
			setting.MinCapacity = config.MinCapacity
		}
		if setting.MaxCapacity <= 0 {
			setting.MaxCapacity = config.MaxCapacity
		}
		if setting.InitialCapacity <= 0 {
			setting.InitialCapacity = config.InitialCapacity
		}
		if setting.MaxCapacity < setting.MinCapacity {
			setting.MaxCapacity = setting.MinCapacity
		}
		if setting.InitialCapacity < setting.MinCapacity {
			setting.InitialCapacity = setting.MinCapacity
		}
		if setting.InitialCapacity > setting.MaxCapacity {
			setting.InitialCapacity = setting.MaxCapacity
		}

		providerSettings[provider] = setting
	}
	config.ProviderSettings = providerSettings
	return config
}

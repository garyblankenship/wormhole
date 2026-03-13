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

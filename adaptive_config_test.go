package wormhole

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeEnhancedAdaptiveConfigFillsPartialPIDDefaults(t *testing.T) {
	t.Parallel()

	normalized := normalizeEnhancedAdaptiveConfig(EnhancedAdaptiveConfig{
		AdaptiveConfig: AdaptiveConfig{
			TargetLatency:      250 * time.Millisecond,
			MinCapacity:        2,
			MaxCapacity:        20,
			InitialCapacity:    4,
			AdjustmentInterval: time.Second,
			LatencyWindowSize:  8,
		},
		PIDConfig: PIDConfig{
			Kp: 1.25,
		},
	})

	defaults := DefaultPIDConfig()
	assert.Equal(t, 1.25, normalized.PIDConfig.Kp)
	assert.Equal(t, defaults.Ki, normalized.PIDConfig.Ki)
	assert.Equal(t, defaults.Kd, normalized.PIDConfig.Kd)
	assert.Equal(t, defaults.MaxIntegral, normalized.PIDConfig.MaxIntegral)
	assert.Equal(t, defaults.MinIntegral, normalized.PIDConfig.MinIntegral)
	assert.Equal(t, defaults.MaxOutput, normalized.PIDConfig.MaxOutput)
	assert.Equal(t, defaults.MinOutput, normalized.PIDConfig.MinOutput)
}

func TestNormalizeEnhancedAdaptiveConfigFillsProviderPartialPIDDefaults(t *testing.T) {
	t.Parallel()

	normalized := normalizeEnhancedAdaptiveConfig(EnhancedAdaptiveConfig{
		AdaptiveConfig: AdaptiveConfig{
			TargetLatency:      250 * time.Millisecond,
			MinCapacity:        2,
			MaxCapacity:        20,
			InitialCapacity:    4,
			AdjustmentInterval: time.Second,
			LatencyWindowSize:  8,
		},
		PIDConfig: PIDConfig{
			Ki: 0.2,
		},
		ProviderSettings: map[string]ProviderSetting{
			"openai": {
				PIDConfig: &PIDConfig{
					Kp: 3.0,
				},
			},
		},
	})

	providerConfig := normalized.ProviderSettings["openai"].PIDConfig
	require.NotNil(t, providerConfig)
	defaults := DefaultPIDConfig()
	assert.Equal(t, 3.0, providerConfig.Kp)
	assert.Equal(t, 0.2, providerConfig.Ki)
	assert.Equal(t, defaults.Kd, providerConfig.Kd)
	assert.Equal(t, defaults.MaxIntegral, providerConfig.MaxIntegral)
	assert.Equal(t, defaults.MinIntegral, providerConfig.MinIntegral)
	assert.Equal(t, defaults.MaxOutput, providerConfig.MaxOutput)
	assert.Equal(t, defaults.MinOutput, providerConfig.MinOutput)
}

func TestNormalizeEnhancedAdaptiveConfigResolvesProviderInheritanceAndClamps(t *testing.T) {
	t.Parallel()

	normalized := normalizeEnhancedAdaptiveConfig(EnhancedAdaptiveConfig{
		AdaptiveConfig: AdaptiveConfig{
			TargetLatency:      250 * time.Millisecond,
			MinCapacity:        4,
			MaxCapacity:        12,
			InitialCapacity:    6,
			AdjustmentInterval: time.Second,
			LatencyWindowSize:  8,
		},
		PIDConfig: PIDConfig{Ki: 0.2},
		ProviderSettings: map[string]ProviderSetting{
			"pid-only": {PIDConfig: &PIDConfig{Kp: 3}},
			"conflict": {MinCapacity: 9, MaxCapacity: 3, InitialCapacity: 99},
			"explicit": {TargetLatency: 20 * time.Millisecond, MinCapacity: 2, MaxCapacity: 5, InitialCapacity: 3},
		},
	})

	pidOnly := normalized.ProviderSettings["pid-only"]
	assert.Equal(t, 250*time.Millisecond, pidOnly.TargetLatency)
	assert.Equal(t, 4, pidOnly.MinCapacity)
	assert.Equal(t, 12, pidOnly.MaxCapacity)
	assert.Equal(t, 6, pidOnly.InitialCapacity)
	require.NotNil(t, pidOnly.PIDConfig)
	assert.Equal(t, 3.0, pidOnly.PIDConfig.Kp)
	assert.Equal(t, 0.2, pidOnly.PIDConfig.Ki)

	conflict := normalized.ProviderSettings["conflict"]
	assert.Equal(t, 9, conflict.MinCapacity)
	assert.Equal(t, 9, conflict.MaxCapacity)
	assert.Equal(t, 9, conflict.InitialCapacity)

	explicit := normalized.ProviderSettings["explicit"]
	assert.Equal(t, 20*time.Millisecond, explicit.TargetLatency)
	assert.Equal(t, 2, explicit.MinCapacity)
	assert.Equal(t, 5, explicit.MaxCapacity)
	assert.Equal(t, 3, explicit.InitialCapacity)
}

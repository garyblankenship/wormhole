package wormhole

import (
	"testing"
	"time"
)

func TestNormalizeAdaptiveConfigDefaultsAndClamps(t *testing.T) {
	t.Parallel()

	defaults := DefaultAdaptiveConfig()
	for _, tc := range []struct {
		name   string
		config AdaptiveConfig
		want   int
	}{
		{"zero", AdaptiveConfig{}, defaults.InitialCapacity},
		{"negative", AdaptiveConfig{TargetLatency: -time.Second, MinCapacity: -1, MaxCapacity: -2, InitialCapacity: -3, AdjustmentInterval: -time.Second, LatencyWindowSize: -1}, defaults.InitialCapacity},
		{"conflicting", AdaptiveConfig{TargetLatency: time.Millisecond, MinCapacity: 7, MaxCapacity: 3, InitialCapacity: 99, AdjustmentInterval: time.Hour, LatencyWindowSize: 1}, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			normalized := normalizeAdaptiveConfig(tc.config)
			if normalized.InitialCapacity != tc.want || normalized.MaxCapacity < normalized.MinCapacity || normalized.InitialCapacity < normalized.MinCapacity || normalized.InitialCapacity > normalized.MaxCapacity {
				t.Fatalf("normalized config = %#v", normalized)
			}
			if normalized.TargetLatency <= 0 || normalized.AdjustmentInterval <= 0 || normalized.LatencyWindowSize <= 0 {
				t.Fatalf("unresolved config = %#v", normalized)
			}
		})
	}
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
		ProviderSettings: map[string]ProviderSetting{
			"inherited": {},
			"conflict":  {MinCapacity: 9, MaxCapacity: 3, InitialCapacity: 99},
			"explicit":  {TargetLatency: 20 * time.Millisecond, MinCapacity: 2, MaxCapacity: 5, InitialCapacity: 3},
		},
	})

	inherited := normalized.ProviderSettings["inherited"]
	if inherited.TargetLatency != 250*time.Millisecond || inherited.MinCapacity != 4 || inherited.MaxCapacity != 12 || inherited.InitialCapacity != 6 {
		t.Fatalf("inherited provider setting = %#v", inherited)
	}
	conflict := normalized.ProviderSettings["conflict"]
	if conflict.MinCapacity != 9 || conflict.MaxCapacity != 9 || conflict.InitialCapacity != 9 {
		t.Fatalf("clamped provider setting = %#v", conflict)
	}
	explicit := normalized.ProviderSettings["explicit"]
	if explicit.TargetLatency != 20*time.Millisecond || explicit.MinCapacity != 2 || explicit.MaxCapacity != 5 || explicit.InitialCapacity != 3 {
		t.Fatalf("explicit provider setting = %#v", explicit)
	}
}

package wormhole

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/garyblankenship/wormhole/v3/middleware"
)

func TestPIDControllerComputeAndReset(t *testing.T) {
	t.Parallel()

	config := defaultPIDConfig()
	pid := newPIDController(config)
	if output := pid.compute(100*time.Millisecond, 200*time.Millisecond, time.Second); output != 0 {
		t.Fatalf("first output = %f, want 0", output)
	}
	if output := pid.compute(100*time.Millisecond, 200*time.Millisecond, time.Second); output <= 0 {
		t.Fatalf("high-latency output = %f, want positive", output)
	}
	if output := pid.compute(100*time.Millisecond, 50*time.Millisecond, time.Second); output >= 0 {
		t.Fatalf("low-latency output = %f, want negative", output)
	}

	config.maxOutput = 0.1
	config.minOutput = -0.1
	pid = newPIDController(config)
	_ = pid.compute(100*time.Millisecond, time.Second, time.Second)
	if output := pid.compute(100*time.Millisecond, time.Second, time.Second); output != 0.1 {
		t.Fatalf("clamped output = %f, want 0.1", output)
	}
	pid.reset()
	if pid.initialized || pid.integralError != 0 || pid.lastError != 0 {
		t.Fatalf("reset controller = %#v", pid)
	}
}

func TestProviderAdaptiveStateMetricsAndCapacity(t *testing.T) {
	t.Parallel()

	state := NewProviderAdaptiveState(ProviderKey{Provider: "test", Model: "model1"}, 100*time.Millisecond, 1, 10, 5, 10)
	state.RecordLatency(50*time.Millisecond, nil)
	state.RecordLatency(150*time.Millisecond, nil)
	state.RecordLatency(200*time.Millisecond, fmt.Errorf("request failed"))

	avg, errorRate, _, _, _ := state.GetMetrics()
	if avg == 0 || errorRate <= 0 {
		t.Fatalf("metrics = average %s, error rate %f", avg, errorRate)
	}
	_, _ = state.AdjustCapacity()
	capacity, _ := state.AdjustCapacity()
	if capacity < 1 || capacity > 10 {
		t.Fatalf("capacity = %d, want within [1, 10]", capacity)
	}
}

func TestProviderAdaptiveStateDirectConstructorNormalizesInputs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                                    string
		target                                  time.Duration
		min, max, initial, window, wantCapacity int
	}{
		{"zero", 0, 0, 0, 0, 0, DefaultAdaptiveConfig().InitialCapacity},
		{"negative", -time.Second, -1, -2, -3, -4, DefaultAdaptiveConfig().InitialCapacity},
		{"conflicting", time.Millisecond, 7, 3, 99, 1, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := NewProviderAdaptiveState(ProviderKey{Provider: tc.name}, tc.target, tc.min, tc.max, tc.initial, tc.window)
			if got := state.Capacity(); got != tc.wantCapacity {
				t.Fatalf("state capacity = %d, want %d", got, tc.wantCapacity)
			}
			state.RecordLatency(time.Millisecond, nil)
		})
	}
}

func TestEnhancedAdaptiveLimiterNormalizesZeroAndPartialConfig(t *testing.T) {
	t.Parallel()

	limiter := NewEnhancedAdaptiveLimiter(EnhancedAdaptiveConfig{
		ProviderSettings: map[string]ProviderSetting{"partial": {}},
		QueryInterval:    0,
	})
	t.Cleanup(limiter.Stop)

	if limiter.globalState.Capacity() <= 0 {
		t.Fatalf("global capacity = %d", limiter.globalState.Capacity())
	}
	release, ok := limiter.AcquireTokenWithProvider(context.Background(), "partial", "model")
	if !ok {
		t.Fatal("AcquireTokenWithProvider failed")
	}
	release()
	limiter.RecordLatencyWithProvider(time.Millisecond, "partial", "model", nil)
	setting := limiter.config.ProviderSettings["partial"]
	if setting.TargetLatency <= 0 || setting.MinCapacity <= 0 || setting.MaxCapacity < setting.MinCapacity || setting.InitialCapacity < setting.MinCapacity || setting.InitialCapacity > setting.MaxCapacity {
		t.Fatalf("partial provider setting was not resolved: %#v", setting)
	}
}

func TestEnhancedAdaptiveLimiterGlobalAndProviderTokens(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.AdjustmentInterval = time.Hour
	config.QueryInterval = 0
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	globalRelease, ok := limiter.AcquireToken(context.Background())
	if !ok || globalRelease == nil {
		t.Fatal("AcquireToken failed")
	}
	globalRelease()
	providerRelease, ok := limiter.AcquireTokenWithProvider(context.Background(), "openai", "gpt-4")
	if !ok || providerRelease == nil {
		t.Fatal("AcquireTokenWithProvider failed")
	}
	providerRelease()
	limiter.RecordLatencyWithProvider(200*time.Millisecond, "openai", "gpt-4", nil)
	if stats := limiter.GetStats(); stats == nil {
		t.Fatal("GetStats returned nil")
	}
}

func TestEnhancedAdaptiveLimiterProviderSpecificSettings(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	config.ProviderSettings = map[string]ProviderSetting{
		"openai":    {TargetLatency: 300 * time.Millisecond, MinCapacity: 5, MaxCapacity: 50, InitialCapacity: 15},
		"anthropic": {TargetLatency: 500 * time.Millisecond, MinCapacity: 3, MaxCapacity: 30, InitialCapacity: 10},
	}
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	for provider, model := range map[string]string{"openai": "gpt-4", "anthropic": "claude"} {
		release, ok := limiter.AcquireTokenWithProvider(context.Background(), provider, model)
		if !ok {
			t.Fatalf("AcquireTokenWithProvider(%q) failed", provider)
		}
		release()
	}
	if got := limiter.getState("openai", "gpt-4").Capacity(); got != 15 {
		t.Fatalf("OpenAI initial capacity = %d, want 15", got)
	}
	if got := limiter.getState("anthropic", "claude").Capacity(); got != 10 {
		t.Fatalf("Anthropic initial capacity = %d, want 10", got)
	}
}

func TestEnhancedAdaptiveLimiterMetrics(t *testing.T) {
	t.Parallel()

	collector := middleware.NewEnhancedMetricsCollector(middleware.DefaultEnhancedMetricsConfig())
	config := DefaultEnhancedAdaptiveConfig()
	config.MetricsCollector = collector
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)
	limiter.RecordLatencyWithProvider(150*time.Millisecond, "openai", "gpt-4", nil)
	collector.RecordRequest(&middleware.RequestLabels{Provider: "openai", Model: "gpt-4", Method: "text"}, 150*time.Millisecond, nil, 0, 100, 200)

	observer := &metricsObserver{config: limiter.config, metricsCollector: collector, limiter: limiter}
	assert.NotPanics(t, observer.queryExternalMetrics)
	if stats := collector.GetStats(&middleware.RequestLabels{Provider: "openai", Model: "gpt-4", Method: "text"}); stats["requests"] != int64(1) {
		t.Fatalf("metrics requests = %#v, want 1", stats["requests"])
	}
}

func TestEnhancedAdaptiveLimiterModelLevelStats(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	limiter.RecordLatencyWithProvider(100*time.Millisecond, "openai", "gpt-4", nil)
	limiter.RecordLatencyWithProvider(200*time.Millisecond, "openai", "gpt-3.5", nil)
	limiter.RecordLatencyWithProvider(300*time.Millisecond, "anthropic", "claude", nil)
	models, ok := limiter.GetStats()["models"].(map[string]interface{})
	if !ok || len(models) != 3 {
		t.Fatalf("model stats = %#v, want three model entries", models)
	}
}

func TestEnhancedAdaptiveLimiterAcquireTokenContextCanceled(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.InitialCapacity = 1
	config.MaxCapacity = 1
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	release, ok := limiter.AcquireToken(context.Background())
	if !ok {
		t.Fatal("AcquireToken failed")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if release, ok := limiter.AcquireToken(canceled); ok || release != nil {
		t.Fatalf("canceled AcquireToken returned a release function or success: %t", ok)
	}
	release()
}

func TestEnhancedAdaptiveLimiterAcquireTokenSurvivesLimiterSwap(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.InitialCapacity = 5
	config.MinCapacity = 1
	config.MaxCapacity = 20
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	release, ok := limiter.AcquireTokenWithProvider(context.Background(), "openai", "gpt-4")
	if !ok {
		t.Fatal("AcquireTokenWithProvider failed")
	}
	state := limiter.getState("openai", "gpt-4")
	state.mu.RLock()
	oldLimiter := state.limiter
	state.mu.RUnlock()
	limiter.RecordLatencyWithProvider(2*time.Second, "openai", "gpt-4", nil)
	manager := &capacityManager{config: config, limiter: limiter}
	manager.adjustAllCapacities() // initializes the private PID controller
	manager.adjustAllCapacities() // swaps the provider limiter
	state.mu.RLock()
	swapped := state.limiter != oldLimiter
	state.mu.RUnlock()
	if !swapped {
		t.Fatal("capacity adjustment did not swap the provider limiter")
	}
	release()
	if got := oldLimiter.InUse(); got != 0 {
		t.Fatalf("captured limiter still has %d permits after release", got)
	}
}

func TestEnhancedAdaptiveLimiterEvictsIdleModelStates(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	config.IdleStateTTL = time.Second
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	stale := limiter.getOrCreateState("openai", "gpt-4")
	fresh := limiter.getOrCreateState("anthropic", "claude")
	stale.mu.Lock()
	stale.lastSeen = time.Now().Add(-time.Hour)
	stale.mu.Unlock()
	fresh.mu.Lock()
	fresh.lastSeen = time.Now()
	fresh.mu.Unlock()
	manager := &capacityManager{config: config, limiter: limiter}
	manager.evictIdleStates()
	assert.Nil(t, limiter.getState("openai", "gpt-4"))
	assert.NotNil(t, limiter.getState("anthropic", "claude"))
}

func TestEnhancedAdaptiveLimiterPinnedStateSurvivesEviction(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	config.IdleStateTTL = time.Nanosecond
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	state := limiter.pinState("openai", "gpt-4")
	state.mu.Lock()
	state.lastSeen = time.Time{}
	state.mu.Unlock()
	manager := &capacityManager{config: config, limiter: limiter}
	manager.evictIdleStates()
	assert.Same(t, state, limiter.getState("openai", "gpt-4"))
	limiter.unpinState(state)
	manager.evictIdleStates()
	assert.Nil(t, limiter.getState("openai", "gpt-4"))
}

func TestEnhancedAdaptiveLimiterTokenPinsStateUntilIdempotentRelease(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	config.IdleStateTTL = time.Nanosecond
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	release, ok := limiter.AcquireTokenWithProvider(context.Background(), "openai", "gpt-4")
	if !ok {
		t.Fatal("AcquireTokenWithProvider failed")
	}
	state := limiter.getState("openai", "gpt-4")
	limiter.mu.RLock()
	pins := state.pins
	limiter.mu.RUnlock()
	assert.Equal(t, 1, pins)
	state.mu.Lock()
	state.lastSeen = time.Time{}
	state.mu.Unlock()
	manager := &capacityManager{config: config, limiter: limiter}
	manager.evictIdleStates()
	assert.Same(t, state, limiter.getState("openai", "gpt-4"))
	release()
	release()
	limiter.mu.RLock()
	pins = state.pins
	limiter.mu.RUnlock()
	assert.Equal(t, 0, pins)
	manager.evictIdleStates()
	assert.Nil(t, limiter.getState("openai", "gpt-4"))
}

func TestEnhancedAdaptiveLimiterLatencyMutationPinsState(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	limiter := NewEnhancedAdaptiveLimiter(config)
	t.Cleanup(limiter.Stop)

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			limiter.RecordLatencyWithProvider(time.Millisecond, "openai", "gpt-4", nil)
		}()
	}
	wg.Wait()
	state := limiter.getState("openai", "gpt-4")
	if state == nil {
		t.Fatal("latency mutation lost its state")
	}
	limiter.mu.RLock()
	pins := state.pins
	limiter.mu.RUnlock()
	assert.Equal(t, 0, pins)
}

func TestEnhancedAdaptiveLimiterRecordLatency(t *testing.T) {
	t.Parallel()

	limiter := NewEnhancedAdaptiveLimiter(EnhancedAdaptiveConfig{QueryInterval: 0})
	t.Cleanup(limiter.Stop)
	assert.NotPanics(t, func() { limiter.RecordLatency(50 * time.Millisecond) })
}

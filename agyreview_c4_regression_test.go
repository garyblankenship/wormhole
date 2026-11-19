package wormhole

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-07: metricsObserver must aggregate total requests and errors across
// error_type labels for the same (provider, model, method) before computing error rate.
func TestC407AggregateExternalMetricsAcrossErrorTypes(t *testing.T) {
	t.Parallel()

	collector := middleware.NewEnhancedMetricsCollector(&middleware.EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10, 100},
		EnableLabels:            true,
		LabelAggregation:        true,
	})
	labels := &middleware.RequestLabels{Provider: "openai", Model: "gpt-5.6", Method: "text"}
	for range 99 {
		collector.RecordRequest(labels, 10*time.Millisecond, nil, 0, 0, 0)
	}
	collector.RecordRequest(labels, 10*time.Millisecond, errors.New("context deadline exceeded"), 0, 0, 0)

	perLabel := collector.GetAllStats()["per_label"].(map[string]interface{})
	aggs := aggregateExternalMetrics(perLabel)

	key := metricsAggregateKey{provider: "openai", model: "gpt-5.6", method: "text"}
	agg, ok := aggs[key]
	if !ok {
		t.Fatalf("missing aggregate for %+v in %+v", key, aggs)
	}
	if agg.requests != 100 || agg.errors != 1 {
		t.Fatalf("aggregated = %+v, want requests=100 errors=1 (1%% error rate, not 100%%)", agg)
	}
}

// C4-08: ProviderAdaptiveState.AdjustCapacity must honor configured
// ErrorRateThreshold, ErrorRatePenalty, and MinSamplesForError.
func TestC408AdjustCapacityUsesConfiguredErrorControls(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedAdaptiveConfig()
	cfg.TargetLatency = time.Second
	cfg.MinCapacity = 1
	cfg.MaxCapacity = 100
	cfg.InitialCapacity = 20
	cfg.ErrorRateThreshold = 0.25
	cfg.ErrorRatePenalty = 3.0
	cfg.MinSamplesForError = 10
	cfg.AdjustmentInterval = time.Hour
	cfg.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(cfg)
	t.Cleanup(limiter.Stop)

	state := limiter.getOrCreateState("openai", "gpt-5.6")
	// Record 5 errors (< MinSamplesForError=10): error penalty must NOT fire.
	for range 5 {
		state.RecordLatency(time.Millisecond, errors.New("boom"))
	}
	capBefore, _ := state.AdjustCapacity()
	if capBefore < 20 {
		t.Fatalf("capacity reduced to %d before MinSamplesForError=10 was reached", capBefore)
	}

	// Record 5 more errors (total 10 >= MinSamplesForError=10, errorRate=1.0 > 0.25): penalty must reduce capacity.
	for range 5 {
		state.RecordLatency(time.Millisecond, errors.New("boom"))
	}
	capAfter, changed := state.AdjustCapacity()
	if !changed || capAfter >= capBefore {
		t.Fatalf("capacity=%d changed=%v after meeting MinSamplesForError, want reduction from %d", capAfter, changed, capBefore)
	}
}

// C4-09: evictIdleStates must still enforce MaxModelStates when IdleStateTTL <= 0.
func TestC409EvictIdleStatesEnforcesMaxModelStatesWithoutTTL(t *testing.T) {
	t.Parallel()

	cfg := DefaultEnhancedAdaptiveConfig()
	cfg.EnableModelLevel = true
	cfg.IdleStateTTL = 0
	cfg.MaxModelStates = 2
	cfg.AdjustmentInterval = time.Hour
	cfg.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(cfg)
	t.Cleanup(limiter.Stop)

	s1 := limiter.getOrCreateState("openai", "m1")
	s2 := limiter.getOrCreateState("openai", "m2")
	s3 := limiter.getOrCreateState("openai", "m3")

	now := time.Now()
	s1.mu.Lock()
	s1.lastSeen = now.Add(-3 * time.Minute)
	s1.mu.Unlock()
	s2.mu.Lock()
	s2.lastSeen = now.Add(-2 * time.Minute)
	s2.mu.Unlock()
	s3.mu.Lock()
	s3.lastSeen = now.Add(-1 * time.Minute)
	s3.mu.Unlock()

	mgr := &capacityManager{config: cfg, limiter: limiter}
	mgr.evictIdleStates()

	limiter.mu.RLock()
	count := len(limiter.modelStates)
	limiter.mu.RUnlock()
	if count != 2 {
		t.Fatalf("modelStates count = %d, want 2", count)
	}
	if limiter.getState("openai", "m1") != nil {
		t.Fatalf("oldest model state m1 was not evicted")
	}
}

// C4-11: A panicking provider factory must not deadlock waiters in provider_cache.go.
func TestC411PanickingProviderFactoryDoesNotDeadlockWaiters(t *testing.T) {
	t.Parallel()

	client := New(
		WithDiscovery(false),
		WithCustomProvider("panicky", func(types.ProviderConfig) (types.Provider, error) {
			panic("factory exploded")
		}),
		WithProviderConfig("panicky", types.ProviderConfig{}),
	)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.getOrCreateCachedProviderContext(ctx, "panicky", false, false)
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("err = %v, want panic error without deadlock", err)
	}
}

// pools.go: putEmbeddingsRequest must clear req.Input elements before returning to sync.Pool.
func TestPutEmbeddingsRequestClearsInputElements(t *testing.T) {
	t.Parallel()

	req := getEmbeddingsRequest()
	req.Input = append(req.Input, "secret-1", "secret-2")
	backing := req.Input[:2]

	putEmbeddingsRequest(req)

	if backing[0] != "" || backing[1] != "" {
		t.Fatalf("backing slice retained strings after putEmbeddingsRequest: %q", backing)
	}
}

package wormhole

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/stretchr/testify/assert"
)

func TestPIDController(t *testing.T) {
	config := DefaultPIDConfig()
	pid := NewPIDController(config)

	// Test initialization
	output := pid.Compute(100*time.Millisecond, 200*time.Millisecond, time.Second)
	if output != 0.0 {
		t.Errorf("Expected 0.0 on first call, got %f", output)
	}

	// Test control output - high latency should give positive output (reduction signal)
	output = pid.Compute(100*time.Millisecond, 200*time.Millisecond, time.Second)
	if output <= 0.0 {
		t.Errorf("Expected positive output for high latency, got %f", output)
	}

	// Test low latency should give negative output (increase signal)
	output = pid.Compute(100*time.Millisecond, 50*time.Millisecond, time.Second)
	if output >= 0.0 {
		t.Errorf("Expected negative output for low latency, got %f", output)
	}

	// Test output clamping
	config.MaxOutput = 0.1
	config.MinOutput = -0.1
	pid = NewPIDController(config)
	pid.Compute(100*time.Millisecond, 200*time.Millisecond, time.Second) // First call
	output = pid.Compute(100*time.Millisecond, 200*time.Millisecond, time.Second)
	if output > 0.1 || output < -0.1 {
		t.Errorf("Output %f outside clamped range [-0.1, 0.1]", output)
	}
}

func TestProviderAdaptiveState(t *testing.T) {
	key := ProviderKey{Provider: "test", Model: "model1"}
	state := NewProviderAdaptiveState(key, 100*time.Millisecond, 1, 10, 5, 10)

	// Record some latencies
	state.RecordLatency(50*time.Millisecond, nil)                  // Good
	state.RecordLatency(150*time.Millisecond, nil)                 // Bad
	state.RecordLatency(200*time.Millisecond, fmt.Errorf("error")) // Bad with error

	// Get metrics
	avgLatency, errorRate, p50, p90, p99 := state.GetMetrics()
	if avgLatency == 0 {
		t.Error("Expected non-zero average latency")
	}
	if errorRate <= 0 {
		t.Error("Expected non-zero error rate")
	}
	// Percentiles might be zero if not enough data
	_ = p50
	_ = p90
	_ = p99

	// Adjust capacity - first call initializes PID controller
	_, _ = state.AdjustCapacity()
	// Second call should produce change
	newCapacity, changed := state.AdjustCapacity()
	if !changed {
		t.Error("Expected capacity change with mixed latencies")
	}
	if newCapacity < 1 || newCapacity > 10 {
		t.Errorf("Capacity %d outside bounds [1, 10]", newCapacity)
	}
}

func TestEnhancedAdaptiveLimiterBasic(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.AdjustmentInterval = 100 * time.Millisecond // Fast for testing
	config.QueryInterval = 0                           // Disable metrics query for test

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	// Test backward compatibility
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	acquired := limiter.Acquire(ctx)
	if !acquired {
		t.Error("Expected to acquire slot")
	}
	limiter.Release()

	// Test provider-aware limiting
	acquired = limiter.AcquireWithProvider(ctx, "openai", "gpt-4")
	if !acquired {
		t.Error("Expected to acquire provider slot")
	}
	limiter.ReleaseWithProvider("openai", "gpt-4")

	// Record latency
	limiter.RecordLatencyWithProvider(200*time.Millisecond, "openai", "gpt-4", nil)

	// Wait for adjustment
	time.Sleep(200 * time.Millisecond)

	// Get stats
	stats := limiter.GetStats()
	if stats == nil {
		t.Error("Expected non-nil stats")
	}
}

func TestEnhancedAdaptiveLimiterProviderSpecific(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.AdjustmentInterval = 100 * time.Millisecond
	config.QueryInterval = 0

	// Add provider-specific settings
	config.ProviderSettings = map[string]ProviderSetting{
		"openai": {
			TargetLatency:   300 * time.Millisecond,
			MinCapacity:     5,
			MaxCapacity:     50,
			InitialCapacity: 15,
		},
		"anthropic": {
			TargetLatency:   500 * time.Millisecond,
			MinCapacity:     3,
			MaxCapacity:     30,
			InitialCapacity: 10,
		},
	}

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	// Test OpenAI specific limits
	ctx := context.Background()
	acquired := limiter.AcquireWithProvider(ctx, "openai", "gpt-4")
	if !acquired {
		t.Error("Expected to acquire OpenAI slot")
	}
	limiter.ReleaseWithProvider("openai", "gpt-4")

	// Test Anthropic specific limits
	acquired = limiter.AcquireWithProvider(ctx, "anthropic", "claude-3")
	if !acquired {
		t.Error("Expected to acquire Anthropic slot")
	}
	limiter.ReleaseWithProvider("anthropic", "claude-3")

	// Verify provider states exist
	stats := limiter.GetStats()
	providerStats, ok := stats["providers"].(map[string]interface{})
	if !ok {
		t.Error("Expected providers stats")
	}

	// Check OpenAI settings
	if openaiStats, ok := providerStats["openai"].(map[string]interface{}); ok {
		if cap, ok := openaiStats["capacity"].(int); ok {
			if cap != 15 {
				t.Errorf("Expected OpenAI initial capacity 15, got %d", cap)
			}
		}
	}
}

func TestEnhancedAdaptiveLimiterWithMetrics(t *testing.T) {
	// Create a metrics collector
	metricsConfig := middleware.DefaultEnhancedMetricsConfig()
	metricsCollector := middleware.NewEnhancedMetricsCollector(metricsConfig)

	config := DefaultEnhancedAdaptiveConfig()
	config.MetricsCollector = metricsCollector
	config.QueryInterval = 100 * time.Millisecond // Fast query for test
	config.AdjustmentInterval = 100 * time.Millisecond

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	// Record some metrics
	labels := &middleware.RequestLabels{
		Provider:  "openai",
		Model:     "gpt-4",
		Method:    "text",
		ErrorType: "",
	}
	metricsCollector.RecordRequest(labels, 150*time.Millisecond, nil, 0, 100, 200)

	// Wait for metrics query
	time.Sleep(200 * time.Millisecond)

	// Limiter should have queried metrics
	stats := limiter.GetStats()
	if stats == nil {
		t.Error("Expected stats")
	}
}

func TestEnhancedAdaptiveLimiterModelLevel(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.AdjustmentInterval = 100 * time.Millisecond
	config.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	// Record latencies for different models of same provider
	limiter.RecordLatencyWithProvider(100*time.Millisecond, "openai", "gpt-4", nil)
	limiter.RecordLatencyWithProvider(200*time.Millisecond, "openai", "gpt-3.5", nil)
	limiter.RecordLatencyWithProvider(300*time.Millisecond, "anthropic", "claude-3", nil)

	// Wait for adjustment
	time.Sleep(200 * time.Millisecond)

	stats := limiter.GetStats()
	if stats == nil {
		t.Error("Expected stats")
	}

	// Check model-level stats
	if modelStats, ok := stats["models"].(map[string]interface{}); ok {
		// Should have entries for openai:gpt-4, openai:gpt-3.5, anthropic:claude-3
		if len(modelStats) < 3 {
			t.Errorf("Expected at least 3 model entries, got %d", len(modelStats))
		}
	}
}

func TestAcquireTokenBasic(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.AdjustmentInterval = 100 * time.Millisecond
	config.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Test global AcquireToken
	release, ok := limiter.AcquireToken(ctx)
	if !ok {
		t.Fatal("Expected to acquire global token")
	}
	if release == nil {
		t.Fatal("Expected non-nil release function")
	}
	release() // Should not panic

	// Test provider-aware AcquireToken
	release, ok = limiter.AcquireTokenWithProvider(ctx, "openai", "gpt-4")
	if !ok {
		t.Fatal("Expected to acquire provider token")
	}
	if release == nil {
		t.Fatal("Expected non-nil release function")
	}
	release() // Should not panic
}

func TestAcquireTokenContextCanceled(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.InitialCapacity = 1
	config.MaxCapacity = 1
	config.AdjustmentInterval = 100 * time.Millisecond
	config.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Acquire the only slot
	release, ok := limiter.AcquireToken(ctx)
	if !ok {
		t.Fatal("Expected to acquire slot")
	}

	// Try to acquire again with canceled context — should fail
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok = limiter.AcquireToken(canceledCtx)
	if ok {
		t.Fatal("Expected acquire to fail with canceled context")
	}

	release() // Free the slot
}

func TestAcquireTokenSurvivesLimiterSwap(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.InitialCapacity = 5
	config.MinCapacity = 1
	config.MaxCapacity = 20
	config.AdjustmentInterval = 50 * time.Millisecond
	config.QueryInterval = 0

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Acquire a token — the release function captures the current limiter instance
	release, ok := limiter.AcquireTokenWithProvider(ctx, "openai", "gpt-4")
	if !ok {
		t.Fatal("Expected to acquire token")
	}

	// Record high latencies to trigger capacity adjustment
	for i := 0; i < 20; i++ {
		limiter.RecordLatencyWithProvider(2*time.Second, "openai", "gpt-4", nil)
	}

	// Wait for adjustment loop to swap the limiter
	time.Sleep(150 * time.Millisecond)

	// Release should still work — it targets the original limiter instance
	release() // Must not panic
}

func TestEnhancedAdaptiveLimiterEvictsIdleModelStates(t *testing.T) {
	config := DefaultEnhancedAdaptiveConfig()
	config.EnableModelLevel = true
	config.QueryInterval = 0
	config.AdjustmentInterval = time.Hour
	config.IdleStateTTL = 10 * time.Millisecond

	limiter := NewEnhancedAdaptiveLimiter(config)
	defer limiter.Stop()

	stale := limiter.getOrCreateState("openai", "gpt-4")
	fresh := limiter.getOrCreateState("anthropic", "claude-3")

	stale.mu.Lock()
	stale.lastSeen = time.Now().Add(-time.Hour)
	stale.mu.Unlock()

	fresh.mu.Lock()
	fresh.lastSeen = time.Now()
	fresh.mu.Unlock()

	limiter.evictIdleStates()

	assert.Nil(t, limiter.getState("openai", "gpt-4"))
	assert.NotNil(t, limiter.getState("anthropic", "claude-3"))
}

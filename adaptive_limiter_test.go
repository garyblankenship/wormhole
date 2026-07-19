package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdaptiveLimiter_ZeroAdjustmentIntervalDoesNotPanic(t *testing.T) {
	t.Parallel()
	al := NewAdaptiveLimiter(AdaptiveConfig{})
	defer al.Stop()
	assert.Equal(t, DefaultAdaptiveConfig().AdjustmentInterval, al.config.AdjustmentInterval,
		"zero AdjustmentInterval must default to avoid time.NewTicker(0) panic")
}

func TestAdaptiveLimiter_AcquireAndRelease(t *testing.T) {
	t.Parallel()

	cfg := AdaptiveConfig{
		TargetLatency:      100 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        5,
		InitialCapacity:    2,
		AdjustmentInterval: 10 * time.Second,
		LatencyWindowSize:  5,
	}
	al := NewAdaptiveLimiter(cfg)
	defer al.Stop()

	ctx := context.Background()

	// Acquire first slot
	rel1, ok1 := al.AcquireToken(ctx)
	require.True(t, ok1)
	require.NotNil(t, rel1)

	// Acquire second slot
	rel2, ok2 := al.AcquireToken(ctx)
	require.True(t, ok2)
	require.NotNil(t, rel2)

	// Third acquire should block/timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	rel3, ok3 := al.AcquireToken(ctxTimeout)
	assert.False(t, ok3)
	assert.Nil(t, rel3)

	// Deprecated Acquire method
	assert.False(t, al.Acquire(ctxTimeout))

	// Release slot
	rel1()
	al.RecordLatency(50 * time.Millisecond)

	// Now acquire should succeed
	rel4, ok4 := al.AcquireToken(ctx)
	assert.True(t, ok4)
	assert.NotNil(t, rel4)

	rel2()
	rel4()
	al.Release() // test deprecated Release
}

func TestAdaptiveLimiter_CapacityAdjustment(t *testing.T) {
	t.Parallel()

	cfg := AdaptiveConfig{
		TargetLatency:      100 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        5,
		InitialCapacity:    2,
		AdjustmentInterval: 100 * time.Hour, // don't auto-run
		LatencyWindowSize:  3,
	}
	al := NewAdaptiveLimiter(cfg)
	defer al.Stop()

	// Record high latencies (> 100ms)
	al.RecordLatency(200 * time.Millisecond)
	al.RecordLatency(300 * time.Millisecond)
	al.RecordLatency(250 * time.Millisecond)

	// Overfill ring buffer to test eviction of old values
	al.RecordLatency(150 * time.Millisecond)

	// Force adjust capacity (latency > target -> decrease capacity)
	al.adjustCapacity()

	al.mu.RLock()
	capAfterHighLatency := al.limiter.Capacity()
	al.mu.RUnlock()
	assert.Equal(t, 1, capAfterHighLatency)

	// Record low latencies (< 100ms)
	al.RecordLatency(10 * time.Millisecond)
	al.RecordLatency(20 * time.Millisecond)

	// Force adjust capacity (latency < target -> increase capacity)
	al.adjustCapacity()

	al.mu.RLock()
	capAfterLowLatency := al.limiter.Capacity()
	al.mu.RUnlock()
	assert.Equal(t, 2, capAfterLowLatency)
}

func TestAdaptiveLimiter_StopIdempotency(t *testing.T) {
	t.Parallel()
	al := NewAdaptiveLimiter(AdaptiveConfig{AdjustmentInterval: time.Hour})
	al.Stop()
	assert.NotPanics(t, func() {
		al.Stop()
	})
}

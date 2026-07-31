package wormhole

import (
	"context"
	"runtime"
	"sync"
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

func TestAdaptiveLimiterNormalizesZeroNegativeAndConflictingConfig(t *testing.T) {
	t.Parallel()

	defaults := DefaultAdaptiveConfig()
	for _, tc := range []struct {
		name string
		cfg  AdaptiveConfig
		want int
	}{
		{"zero", AdaptiveConfig{}, defaults.InitialCapacity},
		{"negative", AdaptiveConfig{TargetLatency: -time.Second, MinCapacity: -1, MaxCapacity: -2, InitialCapacity: -3, AdjustmentInterval: -time.Second, LatencyWindowSize: -1}, defaults.InitialCapacity},
		{"conflicting", AdaptiveConfig{TargetLatency: time.Millisecond, MinCapacity: 7, MaxCapacity: 3, InitialCapacity: 99, AdjustmentInterval: time.Hour, LatencyWindowSize: 1}, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limiter := NewAdaptiveLimiter(tc.cfg)
			t.Cleanup(limiter.Stop)
			if got := limiter.limiter.Capacity(); got != tc.want {
				t.Fatalf("capacity = %d, want %d", got, tc.want)
			}
			if limiter.config.TargetLatency <= 0 || limiter.config.LatencyWindowSize <= 0 || limiter.config.AdjustmentInterval <= 0 {
				t.Fatalf("unresolved config = %#v", limiter.config)
			}
			release, ok := limiter.AcquireToken(context.Background())
			if !ok {
				t.Fatal("AcquireToken failed")
			}
			release()
			limiter.RecordLatency(time.Millisecond)
		})
	}
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

func TestAdaptiveLimiterDoesNotReplaceLimiterWithOutstandingPermit(t *testing.T) {
	t.Parallel()

	al := NewAdaptiveLimiter(AdaptiveConfig{
		TargetLatency:      100 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        2,
		InitialCapacity:    1,
		AdjustmentInterval: time.Hour,
		LatencyWindowSize:  2,
	})
	t.Cleanup(al.Stop)
	release, ok := al.AcquireToken(context.Background())
	if !ok {
		t.Fatal("AcquireToken failed")
	}
	al.RecordLatency(time.Millisecond)
	al.adjustCapacity()
	if got := al.limiter.Capacity(); got != 1 {
		t.Fatalf("capacity with outstanding permit = %d, want 1", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := al.AcquireToken(ctx); ok {
		t.Fatal("replacement limiter admitted a second handler")
	}
	release()
	al.adjustCapacity()
	if got := al.limiter.Capacity(); got != 2 {
		t.Fatalf("capacity after permit release = %d, want 2", got)
	}
}

func TestAdaptiveLimiterDoesNotReplaceLimiterWithPendingAcquire(t *testing.T) {
	t.Parallel()

	al := NewAdaptiveLimiter(AdaptiveConfig{
		TargetLatency:      100 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        2,
		InitialCapacity:    1,
		AdjustmentInterval: time.Hour,
		LatencyWindowSize:  2,
	})
	t.Cleanup(al.Stop)
	release, ok := al.AcquireToken(context.Background())
	if !ok {
		t.Fatal("initial AcquireToken failed")
	}
	release = sync.OnceFunc(release)
	t.Cleanup(release)
	al.RecordLatency(time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pendingDone := make(chan bool, 1)
	go func() {
		_, acquired := al.AcquireToken(ctx)
		pendingDone <- acquired
	}()
	deadline := time.Now().Add(time.Second)
	for {
		al.mu.RLock()
		pending := al.pendingAcquires
		al.mu.RUnlock()
		if pending == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for pending acquire")
		}
		runtime.Gosched()
	}

	al.adjustCapacity()
	if got := al.limiter.Capacity(); got != 1 {
		t.Fatalf("capacity with pending acquire = %d, want 1", got)
	}
	cancel()
	if <-pendingDone {
		t.Fatal("canceled pending acquire succeeded")
	}
	release()
	al.adjustCapacity()
	if got := al.limiter.Capacity(); got != 2 {
		t.Fatalf("capacity after pending acquire cleared = %d, want 2", got)
	}
}

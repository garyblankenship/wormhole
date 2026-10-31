package wormhole

import (
	"errors"
	"testing"
	"time"
)

func TestProviderAdaptiveStateLatencyWraparound(t *testing.T) {
	t.Parallel()

	state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 10, 5, 4)
	for i := 1; i <= 7; i++ {
		var err error
		if i == 7 {
			err = errors.New("failure")
		}
		state.RecordLatency(time.Duration(i)*time.Millisecond, err)
	}
	if state.latencyNext != 3 || len(state.latencies) != 4 {
		t.Fatalf("window next=%d len=%d, want 3 and 4", state.latencyNext, len(state.latencies))
	}
	avg, errorRate, p50, p90, p99 := state.GetMetrics()
	if avg != 5500*time.Microsecond || errorRate != 0.25 ||
		p50 != 6*time.Millisecond || p90 != 7*time.Millisecond || p99 != 7*time.Millisecond {
		t.Fatalf("metrics=(%s, %g, %s, %s, %s), want (5.5ms, 0.25, 6ms, 7ms, 7ms)",
			avg, errorRate, p50, p90, p99)
	}

	state.mu.Lock()
	state.resetTracking()
	state.mu.Unlock()
	if state.latencyNext != 0 || len(state.latencies) != 0 || cap(state.latencies) != 4 {
		t.Fatalf("reset window next=%d len=%d cap=%d", state.latencyNext, len(state.latencies), cap(state.latencies))
	}
	state.RecordLatency(8*time.Millisecond, nil)
	avg, _, p50, _, _ = state.GetMetrics()
	if avg != 8*time.Millisecond || p50 != 8*time.Millisecond {
		t.Fatalf("metrics after reset: avg=%s p50=%s", avg, p50)
	}
}

func TestProviderAdaptiveStateDefaultLatencyWindowWraparound(t *testing.T) {
	t.Parallel()

	state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 10, 5, 0)
	window := DefaultAdaptiveConfig().LatencyWindowSize
	for i := 1; i <= window+1; i++ {
		state.RecordLatency(time.Duration(i)*time.Millisecond, nil)
	}
	if len(state.latencies) != window || cap(state.latencies) != window || state.latencyNext != 1 {
		t.Fatalf("default window len=%d cap=%d next=%d, want %d/%d/1",
			len(state.latencies), cap(state.latencies), state.latencyNext, window, window)
	}
	avg, _, p50, _, _ := state.GetMetrics()
	wantP50 := time.Duration(window/2+2) * time.Millisecond
	if avg != time.Duration(window+3)*time.Millisecond/2 || p50 != wantP50 {
		t.Fatalf("default window avg=%s p50=%s, want avg=%s p50=%s",
			avg, p50, time.Duration(window+3)*time.Millisecond/2, wantP50)
	}
}

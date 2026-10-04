package wormhole

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C1-01: fast failures must penalize rather than expand capacity.
func TestRemediationC101FastErrorsReduceCapacity(t *testing.T) {
	t.Parallel()
	state := NewProviderAdaptiveState(ProviderKey{Provider: "test"}, time.Second, 1, 100, 20, 10)
	for range 10 {
		state.RecordLatency(time.Millisecond, errors.New("provider failed"))
	}
	capacity, changed := state.AdjustCapacity()
	if !changed || capacity >= 20 {
		t.Fatalf("capacity=%d changed=%v, want reduction", capacity, changed)
	}
}

// C1-03: both percentile storage and moving averages use the configured window.
func TestRemediationC103LatencyWindow(t *testing.T) {
	t.Parallel()
	state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 10, 5, 7)
	for i := 1; i <= 1000; i++ {
		state.RecordLatency(time.Duration(i)*time.Millisecond, nil)
	}
	if len(state.latencies) != 7 || cap(state.latencies) != 7 {
		t.Fatalf("history len/cap=%d/%d", len(state.latencies), cap(state.latencies))
	}
	avg, _, p50, _, _ := state.GetMetrics()
	if avg != 997*time.Millisecond || p50 != 997*time.Millisecond {
		t.Fatalf("avg=%s p50=%s", avg, p50)
	}
}

// C1-04: empty and named models share the provider state when model control is off.
func TestRemediationC104EmptyModelProviderState(t *testing.T) {
	t.Parallel()
	limiter := NewEnhancedAdaptiveLimiter(EnhancedAdaptiveConfig{EnableModelLevel: false})
	t.Cleanup(limiter.Stop)
	empty := limiter.getOrCreateState("test", "")
	named := limiter.pinState("test", "model")
	limiter.unpinState(named)
	if empty != named || len(limiter.modelStates) != 0 {
		t.Fatal("model split provider admission state")
	}
}

func remediationResize(state *ProviderAdaptiveState, capacity int) {
	state.mu.Lock()
	state.resizeLocked(capacity)
	state.mu.Unlock()
}

// Must be called inside a synctest bubble: observe blocked admission without sleeps.
func remediationTryToken(t *testing.T, state *ProviderAdaptiveState) (func(), bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		release func()
		ok      bool
	}
	out := make(chan result, 1)
	go func() { release, ok := state.AcquireToken(ctx); out <- result{release, ok} }()
	synctest.Wait()
	select {
	case got := <-out:
		cancel()
		return got.release, got.ok
	default:
		cancel()
		synctest.Wait()
		got := <-out
		if got.ok {
			got.release()
			t.Fatal("blocked admission succeeded after cancellation")
		}
		return nil, false
	}
}

// C1-02: expansion, deep shrink and successive resizes reserve all generations.
func TestRemediationC102GenerationOccupancy(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 100, 5, 10)
		var old []func()
		for range 5 {
			release, ok := state.AcquireToken(context.Background())
			if !ok {
				t.Fatal("initial admission")
			}
			old = append(old, release)
		}
		remediationResize(state, 7)
		var middle []func()
		for range 2 {
			release, ok := remediationTryToken(t, state)
			if !ok {
				t.Fatal("expansion did not admit spare slot")
			}
			middle = append(middle, release)
		}
		if release, ok := remediationTryToken(t, state); ok {
			release()
			t.Fatal("expansion admitted beyond actual occupancy")
		}
		remediationResize(state, 2)
		// Occupancy does not expire merely because old work is long-running.
		<-time.After(10 * time.Minute)
		// Releasing three of seven retired operations leaves four: still above cap.
		for _, release := range old[:3] {
			release()
		}
		if release, ok := remediationTryToken(t, state); ok {
			release()
			t.Fatal("deep shrink admitted before retired occupancy fell below cap")
		}
		remediationResize(state, 4)
		if release, ok := remediationTryToken(t, state); ok {
			release()
			t.Fatal("successive expansion lost prior generations")
		}
		old[3]()
		release, ok := remediationTryToken(t, state)
		if !ok {
			t.Fatal("draining prior generation did not free a slot")
		}
		release()
		release() // Releases are idempotent.
		old[4]()
		for _, release := range middle {
			release()
		}
		if state.InUse() != 0 || state.limiter.InUse() != 0 {
			t.Fatal("reservations remained after all real work completed")
		}
	})
}

// C1-02: an acquisition waiting on a retired semaphore migrates to the live one.
func TestRemediationC102BlockedAcquireResizes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 100, 1, 10)
		first, ok := state.AcquireToken(context.Background())
		if !ok {
			t.Fatal("initial admission")
		}
		out := make(chan func(), 1)
		go func() {
			release, ok := state.AcquireToken(context.Background())
			if ok {
				out <- release
			}
		}()
		synctest.Wait()
		remediationResize(state, 2)
		synctest.Wait()
		select {
		case release := <-out:
			release()
		default:
			t.Fatal("waiter remained on retired generation")
		}
		first()
	})
}

// C1-02: concurrent releases/acquisitions and generation swaps preserve accounting.
func TestRemediationC102ConcurrentResize(t *testing.T) {
	t.Parallel()
	state := NewProviderAdaptiveState(ProviderKey{}, time.Second, 1, 100, 8, 10)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				release, ok := state.AcquireToken(context.Background())
				if !ok {
					return
				}
				release()
				release()
			}
		}()
	}
	for i := 0; i < 100; i++ {
		remediationResize(state, 1+i%12)
	}
	wg.Wait()
	if state.InUse() != 0 || state.limiter.InUse() != 0 {
		t.Fatal("occupancy leaked across concurrent generation changes")
	}
}

// C1-07/C1-R03: nil options are ignored; enablement and shutdown share admission.
func TestRemediationC107EnableShutdownNilOptions(t *testing.T) {
	t.Parallel()
	client := New(nil, WithDiscovery(false))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				client.EnableAdaptiveConcurrency(nil)
			}
		}()
	}
	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	before := client.GetAdaptiveLimiter()
	client.EnableAdaptiveConcurrency(nil)
	if client.GetAdaptiveLimiter() != before {
		t.Fatal("post-shutdown enablement replaced stopped limiter")
	}
	if before != nil {
		select {
		case <-before.stopChan:
		default:
			t.Fatal("shutdown left limiter running")
		}
	}
}

type remediationReentrantProvider struct {
	*lifecycleProvider
	onClose func()
}

func (p *remediationReentrantProvider) Close() error { p.onClose(); return p.lifecycleProvider.Close() }

// C1-I01: provider Close can inspect the cache without holding its write lock.
func TestRemediationC1I01ProviderCloseOutsideLock(t *testing.T) {
	t.Parallel()
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "shutdown"}[shutdown], func(t *testing.T) {
			t.Parallel()
			var client *Wormhole
			provider := &remediationReentrantProvider{lifecycleProvider: newLifecycleProvider("test"), onClose: func() {
				if client.GetCacheMetrics().Size != 0 {
					t.Error("provider not detached before Close")
				}
			}}
			client = New(WithDiscovery(false), WithCustomProvider("test", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("test", types.ProviderConfig{}))
			t.Cleanup(func() { _ = client.Close() })
			handle, err := client.ProviderWithHandle("test")
			if err != nil {
				t.Fatal(err)
			}
			_ = handle.Close()
			if shutdown {
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				client.CleanupStaleProviders(0, 0)
			}
			if provider.closeCount.Load() != 1 {
				t.Fatal("provider not closed exactly once")
			}
		})
	}
}

// C1-I02: surplus releases never create negative counts or clobber acquisitions.
func TestRemediationC1I02PositiveReleaseCounter(t *testing.T) {
	t.Parallel()
	cp := &cachedProvider{}
	client := &Wormhole{providers: map[string]*cachedProvider{"test": cp}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				atomic.AddInt32(&cp.refCount, 1)
				client.releaseProvider("test")
				client.releaseProvider("test")
			}
		}()
	}
	wg.Wait()
	if count := atomic.LoadInt32(&cp.refCount); count < 0 {
		t.Fatalf("negative ref count=%d", count)
	}
	atomic.StoreInt32(&cp.refCount, 1)
	client.releaseProvider("test")
	client.releaseProvider("test")
	if count := atomic.LoadInt32(&cp.refCount); count != 0 {
		t.Fatalf("surplus release count=%d", count)
	}
}

package discovery

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type c4CountingFetcher struct {
	name   string
	calls  atomic.Int32
	models []*types.ModelInfo
}

func (f *c4CountingFetcher) Name() string { return f.name }

func (f *c4CountingFetcher) FetchModels(_ context.Context) ([]*types.ModelInfo, error) {
	f.calls.Add(1)
	return f.models, nil
}

// C4-01: StartBackgroundRefresh after Stop must not panic/race with wg.Wait,
// and negative RefreshInterval must be normalized/treated as disabled.
func TestC401StartBackgroundRefreshAfterStopAndNegativeInterval(t *testing.T) {
	t.Parallel()

	t.Run("negative refresh interval is disabled", func(t *testing.T) {
		t.Parallel()
		cfg := NormalizeConfig(DiscoveryConfig{
			RefreshInterval:  -5 * time.Second,
			DisableFileCache: true,
		})
		if cfg.RefreshInterval != 0 || !cfg.DisableBackgroundRefresh {
			t.Fatalf("NormalizeConfig negative interval = %+v, want RefreshInterval=0 DisableBackgroundRefresh=true", cfg)
		}

		svc := NewDiscoveryService(DiscoveryConfig{
			RefreshInterval:  -time.Second,
			DisableFileCache: true,
		})
		t.Cleanup(func() { _ = svc.Stop() })

		// Must not panic in time.NewTicker with <= 0 duration.
		svc.StartBackgroundRefresh(context.Background())
	})

	t.Run("start after stop does not race or panic", func(t *testing.T) {
		t.Parallel()
		svc := NewDiscoveryService(DiscoveryConfig{
			RefreshInterval:  10 * time.Millisecond,
			DisableFileCache: true,
		})
		if err := svc.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		// Must be a safe no-op after Stop.
		svc.StartBackgroundRefresh(context.Background())
	})
}

// C4-02: OfflineMode must prevent network refreshes in both GetModelsWithStatus
// (on stale/fallback hits) and RefreshModels.
func TestC402OfflineModeSkipsNetworkRefresh(t *testing.T) {
	t.Parallel()

	fetcher := &c4CountingFetcher{
		name:   "openai",
		models: []*types.ModelInfo{{ID: "gpt-5.6", Provider: "openai"}},
	}
	svc := NewDiscoveryService(DiscoveryConfig{
		OfflineMode:              true,
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
		CacheTTL:                 time.Nanosecond,
	}, fetcher)
	t.Cleanup(func() { _ = svc.Stop() })

	// RefreshModels must return nil without invoking fetchers.
	if err := svc.RefreshModels(context.Background()); err != nil {
		t.Fatalf("RefreshModels in offline mode: %v", err)
	}
	if got := fetcher.calls.Load(); got != 0 {
		t.Fatalf("RefreshModels made %d fetcher calls in offline mode, want 0", got)
	}

	// Seed a stale cache entry (expired immediately due to 1ns TTL) and read it.
	svc.cache.Set(svc.cacheKey("openai"), []*types.ModelInfo{{ID: "cached-model", Provider: "openai"}})
	time.Sleep(time.Millisecond)

	res, err := svc.GetModelsWithStatus(context.Background(), "openai")
	if err != nil {
		t.Fatalf("GetModelsWithStatus: %v", err)
	}
	if !res.Stale || len(res.Models) == 0 {
		t.Fatalf("expected stale/fallback models, got: %+v", res)
	}

	// Wait briefly to ensure no background refreshProvider goroutine ran.
	time.Sleep(10 * time.Millisecond)
	if got := fetcher.calls.Load(); got != 0 {
		t.Fatalf("GetModelsWithStatus triggered %d fetcher calls in offline mode, want 0", got)
	}
}

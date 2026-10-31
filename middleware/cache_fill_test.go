package middleware

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type fillWaitContext struct {
	context.Context
	waiting chan struct{}
}

func (c fillWaitContext) Done() <-chan struct{} {
	select {
	case c.waiting <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

func fillConfig(cache Cache) CacheConfig {
	return CacheConfig{
		Cache: cache, TTL: time.Hour, CoordinateFills: true, MaxConcurrentFills: 1,
		FillIdentity: func(context.Context, any) string { return "tenant-and-effective-credential" },
	}
}

func TestCacheFillDefaultMissesRunIndependently(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(4)
	t.Cleanup(func() { _ = cache.Close() })
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	var calls atomic.Int32
	handler := NewTypedCacheMiddleware(CacheConfig{Cache: cache, TTL: time.Hour}).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return &types.TextResponse{Text: "independent"}, nil
	})
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := handler(t.Context(), testTextRequest("same")); results <- err }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("independent misses did not both reach downstream")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("downstream calls = %d, want 2", calls.Load())
	}
	closeRelease()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCacheFillOverlapSharesDetachedResponse(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(4)
	t.Cleanup(func() { _ = cache.Close() })
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	mw := NewTypedCacheMiddleware(fillConfig(cache))
	handler := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		calls.Add(1)
		close(entered)
		<-release
		return &types.TextResponse{Text: "ok", Metadata: map[string]any{"nested": map[string]any{"value": "fresh"}}}, nil
	})
	type result struct {
		response *types.TextResponse
		err      error
	}
	first := make(chan result, 1)
	second := make(chan result, 1)
	go func() { r, err := handler(t.Context(), testTextRequest("same")); first <- result{r, err} }()
	<-entered
	waiting := make(chan struct{}, 1)
	go func() {
		r, err := handler(fillWaitContext{t.Context(), waiting}, testTextRequest("same"))
		second <- result{r, err}
	}()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not wait")
	}
	close(release)
	a, b := <-first, <-second
	if a.err != nil || b.err != nil || calls.Load() != 1 || a.response == b.response {
		t.Fatalf("responses=%p/%p errors=%v/%v calls=%d", a.response, b.response, a.err, b.err, calls.Load())
	}
	b.response.Metadata["nested"].(map[string]any)["value"] = "changed"
	if a.response.Metadata["nested"].(map[string]any)["value"] != "fresh" {
		t.Fatal("follower mutated leader response")
	}
}

func TestCacheFillFailureAndLeaderCancellationRetry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		cancelLeader bool
	}{{"failure", false}, {"leader canceled", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cache := NewMemoryCache(4)
			t.Cleanup(func() { _ = cache.Close() })
			entered := make(chan struct{})
			release := make(chan struct{})
			var calls atomic.Int32
			handler := NewTypedCacheMiddleware(fillConfig(cache)).ApplyText(func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
					if tc.cancelLeader {
						return nil, ctx.Err()
					}
					return nil, errors.New("provider failed")
				}
				return &types.TextResponse{Text: "retry"}, nil
			})
			leaderCtx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			leader := make(chan error, 1)
			go func() { _, err := handler(leaderCtx, testTextRequest("same")); leader <- err }()
			<-entered
			waiting := make(chan struct{}, 1)
			follower := make(chan *types.TextResponse, 1)
			go func() { r, _ := handler(fillWaitContext{t.Context(), waiting}, testTextRequest("same")); follower <- r }()
			select {
			case <-waiting:
			case <-time.After(5 * time.Second):
				t.Fatal("follower did not wait")
			}
			if tc.cancelLeader {
				cancel()
			}
			close(release)
			if err := <-leader; err == nil {
				t.Fatal("leader error lost")
			}
			if r := <-follower; r == nil || r.Text != "retry" {
				t.Fatalf("follower retry = %#v", r)
			}
			if calls.Load() != 2 {
				t.Fatalf("calls = %d, want 2", calls.Load())
			}
		})
	}
}

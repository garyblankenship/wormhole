package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestCacheFillRequiresIdentityAndBypassesAtCapacity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		identity     func(context.Context, any) string
		secondPrompt string
	}{
		{"missing identity", nil, "same"},
		{"empty identity", func(context.Context, any) string { return "" }, "same"},
		{"capacity", func(context.Context, any) string { return "tenant" }, "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cache := NewMemoryCache(4)
			t.Cleanup(func() { _ = cache.Close() })
			config := fillConfig(cache)
			config.FillIdentity = tc.identity
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			var calls atomic.Int32
			handler := NewTypedCacheMiddleware(config).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
				calls.Add(1)
				entered <- struct{}{}
				<-release
				return &types.TextResponse{Text: "ok"}, nil
			})
			results := make(chan error, 2)
			go func() { _, err := handler(t.Context(), testTextRequest("same")); results <- err }()
			<-entered
			go func() { _, err := handler(t.Context(), testTextRequest(tc.secondPrompt)); results <- err }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("second miss did not bypass coordination")
			}
			unblock()
			for range 2 {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("calls = %d, want 2", calls.Load())
			}
		})
	}
}

func TestCacheFillIdentityAlsoIsolatesStoredResponses(t *testing.T) {
	t.Parallel()
	type tenantKey struct{}
	cache := NewMemoryCache(4)
	t.Cleanup(func() { _ = cache.Close() })
	config := fillConfig(cache)
	config.FillIdentity = func(ctx context.Context, _ any) string {
		identity, _ := ctx.Value(tenantKey{}).(string)
		return identity
	}
	var calls atomic.Int32
	handler := NewTypedCacheMiddleware(config).ApplyText(func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
		calls.Add(1)
		identity, _ := ctx.Value(tenantKey{}).(string)
		return &types.TextResponse{Text: identity}, nil
	})
	for _, identity := range []string{"tenant-a", "tenant-b", "tenant-a", "tenant-b"} {
		ctx := context.WithValue(t.Context(), tenantKey{}, identity)
		response, err := handler(ctx, testTextRequest("same"))
		if err != nil || response == nil || response.Text != identity {
			t.Fatalf("identity %q received response %#v, error %v", identity, response, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("downstream calls = %d, want one per identity", calls.Load())
	}
}

func TestCacheFillCanceledFollowerDoesNotCancelLeader(t *testing.T) {
	t.Parallel()
	cache := NewMemoryCache(4)
	t.Cleanup(func() { _ = cache.Close() })
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var calls atomic.Int32
	handler := NewTypedCacheMiddleware(fillConfig(cache)).ApplyText(func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return &types.TextResponse{Text: "ok"}, nil
		}
	})
	leader := make(chan error, 1)
	go func() { _, err := handler(t.Context(), testTextRequest("same")); leader <- err }()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	waiting := make(chan struct{}, 1)
	follower := make(chan error, 1)
	go func() { _, err := handler(fillWaitContext{ctx, waiting}, testTextRequest("same")); follower <- err }()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not wait")
	}
	cancel()
	if err := <-follower; err != context.Canceled {
		t.Fatalf("follower error = %v", err)
	}
	unblock()
	if err := <-leader; err != nil {
		t.Fatalf("leader error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

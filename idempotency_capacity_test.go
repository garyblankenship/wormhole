package wormhole

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestIdempotencyCapacityEvictsOnlyCompletedEntries(t *testing.T) {
	t.Parallel()

	client := New(
		WithIdempotencyKey("capacity"),
		WithIdempotencyMaxEntries(2),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })

	inFlight, created, err := client.loadOrCreateIdempotencyEntry("in-flight", time.Now())
	if err != nil || !created {
		t.Fatalf("create in-flight: created=%v err=%v", created, err)
	}
	completed, created, err := client.loadOrCreateIdempotencyEntry("completed", time.Now())
	if err != nil || !created {
		t.Fatalf("create completed: created=%v err=%v", created, err)
	}
	client.completeIdempotencyEntry(completed, time.Now(), time.Hour)

	replacement, created, err := client.loadOrCreateIdempotencyEntry("replacement", time.Now())
	if err != nil || !created {
		t.Fatalf("create replacement: created=%v err=%v", created, err)
	}
	if replacement == completed {
		t.Fatal("replacement reused the completed entry")
	}
	client.idempotencyMu.Lock()
	gotInFlight := client.idempotencyCache["in-flight"]
	_, retainedCompleted := client.idempotencyCache["completed"]
	client.idempotencyMu.Unlock()
	if gotInFlight != inFlight {
		t.Fatal("capacity eviction removed the in-flight entry")
	}
	if retainedCompleted {
		t.Fatal("capacity eviction retained the oldest completed entry")
	}
	stats := client.GetIdempotencyCacheStats()
	if stats.Entries != 2 || stats.Capacity != 2 || stats.CapacityEvictions != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestIdempotencyKeyIncludesProviderOptionsForEveryTypedRequest(t *testing.T) {
	t.Parallel()
	client := New(WithIdempotencyKey("provider-options"), WithDiscovery(false))
	requests := []struct {
		name  string
		left  any
		right any
	}{
		{"text", types.TextRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "left"}}}, types.TextRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "right"}}}},
		{"structured", types.StructuredRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "left"}}}, types.StructuredRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"mode": "right"}}}},
		{"embeddings", types.EmbeddingsRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.EmbeddingsRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"rerank", types.RerankRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.RerankRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"image", types.ImageRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.ImageRequest{ProviderOptions: map[string]any{"mode": "right"}}},
		{"audio", types.AudioRequest{ProviderOptions: map[string]any{"mode": "left"}}, types.AudioRequest{ProviderOptions: map[string]any{"mode": "right"}}},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			left, ok := client.idempotencyCacheKey(request.name, request.left)
			if !ok {
				t.Fatal("left idempotency key was not generated")
			}
			right, ok := client.idempotencyCacheKey(request.name, request.right)
			if !ok {
				t.Fatal("right idempotency key was not generated")
			}
			if left == right {
				t.Fatal("provider options did not affect the idempotency key")
			}
		})
	}
}

func TestIdempotencyAllInFlightCapacityRejectsBeforeExecution(t *testing.T) {
	t.Parallel()

	client := New(
		WithIdempotencyKey("capacity"),
		WithIdempotencyMaxEntries(1),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })

	type request struct{ Value string }
	first := request{"first"}
	leaderStarted := make(chan struct{})
	releaseLeader := make(chan struct{})
	var releaseLeaderOnce sync.Once
	t.Cleanup(func() { releaseLeaderOnce.Do(func() { close(releaseLeader) }) })
	type outcome struct {
		value string
		err   error
	}
	leaderDone := make(chan outcome, 1)
	followerDone := make(chan outcome, 1)
	var calls atomic.Int32
	go func() {
		value, err := executeTrackedRequest(context.Background(), client, "text", first, func(context.Context) (string, error) {
			calls.Add(1)
			close(leaderStarted)
			<-releaseLeader
			return "shared", nil
		})
		leaderDone <- outcome{value, err}
	}()
	<-leaderStarted
	go func() {
		value, err := executeTrackedRequest(context.Background(), client, "text", first, func(context.Context) (string, error) {
			calls.Add(1)
			return "duplicate executed", nil
		})
		followerDone <- outcome{value, err}
	}()

	_, err := executeTrackedRequest(context.Background(), client, "text", request{"second"}, func(context.Context) (string, error) {
		calls.Add(1)
		return "capacity bypassed", nil
	})
	if !errors.Is(err, types.ErrIdempotencyCapacity) {
		t.Fatalf("error = %v, want ErrIdempotencyCapacity", err)
	}
	wormholeErr, ok := types.AsWormholeError(err)
	if !ok || wormholeErr.Code != types.ErrorCodeMiddleware || !wormholeErr.Retryable ||
		wormholeErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity error = %#v", wormholeErr)
	}
	if got := client.GetIdempotencyCacheStats().CapacityRejections; got != 1 {
		t.Fatalf("capacity rejections = %d, want 1", got)
	}
	releaseLeaderOnce.Do(func() { close(releaseLeader) })
	for name, got := range map[string]outcome{"leader": <-leaderDone, "follower": <-followerDone} {
		if got.err != nil || got.value != "shared" {
			t.Fatalf("%s = (%q, %v), want shared nil", name, got.value, got.err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("execution calls = %d, want 1 coalesced call", calls.Load())
	}
}

func TestIdempotencyLiveFollowerReplacesCanceledOwner(t *testing.T) {
	t.Parallel()

	client := New(WithIdempotencyKey("replace"), WithDiscovery(false))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	if got := client.GetIdempotencyCacheStats().Capacity; got != defaultIdempotencyMaxEntries {
		t.Fatalf("default capacity = %d, want %d", got, defaultIdempotencyMaxEntries)
	}

	type request struct{ Prompt string }
	req := request{Prompt: "same"}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderStarted := make(chan struct{})
	leaderDone := make(chan error, 1)
	var calls atomic.Int32
	go func() {
		_, err := executeTrackedRequest(leaderCtx, client, "text", req, func(ctx context.Context) (string, error) {
			calls.Add(1)
			close(leaderStarted)
			<-ctx.Done()
			return "", ctx.Err()
		})
		leaderDone <- err
	}()
	<-leaderStarted

	followerDone := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		value, err := executeTrackedRequest(context.Background(), client, "text", req, func(context.Context) (string, error) {
			calls.Add(1)
			return "replacement", nil
		})
		followerDone <- struct {
			value string
			err   error
		}{value, err}
	}()

	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v", err)
	}
	follower := <-followerDone
	if follower.err != nil || follower.value != "replacement" {
		t.Fatalf("follower = (%q, %v)", follower.value, follower.err)
	}
	if calls.Load() != 2 {
		t.Fatalf("execution calls = %d, want 2", calls.Load())
	}
}

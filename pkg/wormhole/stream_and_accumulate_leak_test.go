package wormhole

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"

	whtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// This test intentionally does not call t.Parallel(): goleak's goroutine
// snapshot/diff is unreliable when other t.Parallel() tests in this package
// are actively spawning/tearing down goroutines concurrently.
func TestStreamAndAccumulateNoGoroutineLeakOnAbandonedConsumer(t *testing.T) {
	mock := whtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Text: "one"},
		{Text: "two"},
		{Text: "three"},
	})
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	// Snapshot goroutines AFTER client construction: New() spawns a
	// persistent startIdempotencySweeper goroutine that must be part of the
	// baseline, not flagged as a false-positive leak from this test.
	opt := goleak.IgnoreCurrent()

	ctx, cancel := context.WithCancel(context.Background())

	accumulated, _, err := client.Text().Model("test-model").Prompt("hi").StreamAndAccumulate(ctx)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-accumulated:
	case <-time.After(time.Second):
		t.Fatal("did not receive first chunk")
	}

	// Abandon the consumer without draining further, then cancel — the
	// internal accumulation goroutine must exit instead of leaking blocked
	// on accumulated <- chunk.
	cancel()

	goleak.VerifyNone(t, opt)
}

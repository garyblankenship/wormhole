package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestDrainStreamStopsAtBound(t *testing.T) {
	t.Parallel()

	stream := make(chan types.StreamChunk)
	done := make(chan struct{})
	go func() {
		drainStream(context.Background(), stream, 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain survived its configured bound")
	}
}

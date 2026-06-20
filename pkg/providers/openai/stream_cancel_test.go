package openai

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accumulatingStream guards its forward-send with the context: once ctx is
// canceled, the goroutine must exit WITHOUT delivering the pending chunk and
// must still close its output channel. The non-blocking ctx.Done() pre-check is
// what makes "cancel wins" deterministic even when a receiver is already waiting
// on out — a bare (send / ctx.Done) select would otherwise pick pseudo-randomly.
//
// Not t.Parallel(): a synctest bubble must contain all of its goroutines and
// forbids t.Parallel inside the bubble.
func TestAccumulatingStreamHonorsCanceledContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		in := make(chan types.TextChunk, 1)
		in <- types.TextChunk{Text: "must-not-be-delivered"}
		close(in)

		cancel() // canceled before the goroutine reaches the send guard

		out := (&Provider{}).accumulatingStream(ctx, in)

		// A receiver is ready and waiting on out. Without the pre-check guard,
		// select could still hand the chunk to it; the guard must make ctx win.
		type recv struct {
			chunk types.TextChunk
			ok    bool
		}
		got := make(chan recv, 1)
		go func() {
			c, ok := <-out
			got <- recv{chunk: c, ok: ok}
		}()

		synctest.Wait() // all bubble goroutines durably blocked or exited

		r := <-got
		assert.False(t, r.ok, "canceled ctx must close out without delivering a chunk")
		assert.Empty(t, r.chunk.Text)

		// out is closed: any further receive is also closed, never a value.
		_, ok := <-out
		require.False(t, ok)
	})
}

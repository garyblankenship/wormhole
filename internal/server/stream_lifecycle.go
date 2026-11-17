package server

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/types"
)

// receiveStreamChunk preserves upstream ordering and stops delivery on client
// cancellation, including when a buffered chunk and cancellation are both ready.
// Callers check ctx before emitting their endpoint's normal completion frames.
// Upstream errors remain chunks so each endpoint can encode its own failure.
func receiveStreamChunk(ctx context.Context, stream <-chan types.TextChunk) (types.TextChunk, bool) {
	if ctx.Err() != nil {
		return types.TextChunk{}, false
	}
	select {
	case <-ctx.Done():
		return types.TextChunk{}, false
	case chunk, open := <-stream:
		if ctx.Err() != nil {
			return types.TextChunk{}, false
		}
		return chunk, open
	}
}

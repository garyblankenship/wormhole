package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/types"
)

// StreamAndCollect streams chunks while collecting a complete TextResponse.
// Drain the returned channel, or cancel ctx, before calling the result function:
// it waits for collection to end. Each result is detached from earlier results
// and the delivered chunks. Interrupted streams return their partial response
// and error. Finish markers do not discard trailing usage or errors.
//
// Collection retains fields present in TextChunk, including metadata and citation
// snapshots; Created cannot be reconstructed. Native metadata is merged by key,
// with later values replacing earlier values. Terminal reference snapshots replace
// prior additions. Thinking uses the existing scalar representation, so distinct
// block boundaries and multiple signatures cannot be retained.
func (b *TextRequestBuilder) StreamAndCollect(ctx context.Context) (<-chan types.StreamChunk, func() (*types.TextResponse, error), error) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := b.Stream(streamCtx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	out := make(chan types.StreamChunk)
	done := make(chan struct{})
	collector := textResponseCollector{response: types.TextResponse{Model: b.request.Model}}
	var streamErr error
	// Exit on source closure, a stream error, or cancellation on receive/send.
	go func() {
		defer close(out)
		defer close(done)
		defer cancel()
		streamErr = collectTextStream(streamCtx, stream, out, &collector)
		collector.finish()
	}()

	return out, func() (*types.TextResponse, error) {
		<-done
		return types.CloneTextResponse(&collector.response), streamErr
	}, nil
}

func collectTextStream(ctx context.Context, source <-chan types.StreamChunk, out chan<- types.StreamChunk, collector *textResponseCollector) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-source:
			if !ok {
				return ctx.Err()
			}
			collector.add(chunk)
			select {
			case out <- types.CloneTextChunk(chunk):
			case <-ctx.Done():
				if chunk.Error != nil {
					return chunk.Error
				}
				return ctx.Err()
			}
			if chunk.Error != nil {
				return chunk.Error
			}
		}
	}
}

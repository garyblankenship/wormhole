package server

import (
	"context"
	"errors"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestReceiveStreamChunkPreservesOrderAndFailure(t *testing.T) {
	failure := errors.New("upstream failed")
	stream := make(chan types.TextChunk, 3)
	stream <- types.TextChunk{Text: "first"}
	stream <- types.TextChunk{Text: "second"}
	stream <- types.TextChunk{Error: failure}
	close(stream)
	for _, want := range []string{"first", "second"} {
		chunk, open := receiveStreamChunk(context.Background(), stream)
		if !open || chunk.Content() != want {
			t.Fatalf("received %q, open=%v, want %q", chunk.Content(), open, want)
		}
	}
	chunk, open := receiveStreamChunk(context.Background(), stream)
	if !open || chunk.Error != failure {
		t.Fatalf("failure lost: chunk=%+v, open=%v", chunk, open)
	}
	if _, open := receiveStreamChunk(context.Background(), stream); open {
		t.Fatal("closed stream reported open")
	}
}

func TestReceiveStreamChunkCancellationPrecedesBufferedDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := make(chan types.TextChunk, 1)
	stream <- types.TextChunk{Text: "buffered"}
	cancel()
	if _, open := receiveStreamChunk(ctx, stream); open {
		t.Fatal("delivered chunk after cancellation")
	}
	if len(stream) != 1 {
		t.Fatal("consumed buffered chunk after cancellation")
	}
}

package stream

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type auditBlockedBody struct {
	reading   chan struct{}
	closed    chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
	closes    atomic.Int32
}

func (b *auditBlockedBody) Read([]byte) (int, error) {
	b.readOnce.Do(func() { close(b.reading) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *auditBlockedBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

type auditEOFBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *auditEOFBody) Close() error { b.closes.Add(1); return nil }

func TestAuditStreamReaderCancellationAndEOF(t *testing.T) {
	t.Parallel()
	for name, process := range map[string]func(context.Context, io.ReadCloser, func([]byte) (*types.TextChunk, error), int) <-chan types.TextChunk{
		"SSE":    ProcessSSE,
		"NDJSON": ProcessNDJSON,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			transform := func([]byte) (*types.TextChunk, error) { return &types.TextChunk{}, nil }
			body := &auditBlockedBody{reading: make(chan struct{}), closed: make(chan struct{})}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			out := process(ctx, body, transform, 0)
			<-body.reading
			cancel()
			for chunk := range out {
				t.Fatalf("cancellation emitted a frame: %#v", chunk)
			}
			if n := body.closes.Load(); n != 1 {
				t.Fatalf("cancellation closed body %d times", n)
			}
			eof := &auditEOFBody{Reader: strings.NewReader("")}
			for range process(context.Background(), eof, transform, 0) {
			}
			if n := eof.closes.Load(); n != 1 {
				t.Fatalf("EOF closed body %d times", n)
			}
		})
	}
}

func TestStreamReadersRespectProtocolFinish(t *testing.T) {
	stop := types.FinishReasonStop
	decode := func(data []byte) (*types.TextChunk, error) {
		if string(data) == "finish" {
			return &types.TextChunk{FinishReason: &stop}, nil
		}
		return &types.TextChunk{Usage: &types.Usage{TotalTokens: 3}}, nil
	}
	for _, tc := range []struct {
		name      string
		input     string
		process   func(context.Context, io.ReadCloser, func([]byte) (*types.TextChunk, error), int) <-chan types.TextChunk
		wantUsage bool
	}{
		{"SSE", "data: finish\n\ndata: usage\n\ndata: [DONE]\n\n", ProcessSSE, true},
		{"NDJSON", "finish\nusage\n", ProcessNDJSON, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &auditEOFBody{Reader: strings.NewReader(tc.input)}
			out := tc.process(context.Background(), body, decode, 0)
			if chunk := <-out; !chunk.IsDone() {
				t.Fatalf("first chunk = %#v, want finish", chunk)
			}
			if tc.wantUsage {
				if chunk := <-out; chunk.Usage == nil || chunk.Usage.TotalTokens != 3 {
					t.Fatalf("trailing chunk = %#v, want usage", chunk)
				}
			}
			if chunk, open := <-out; open {
				t.Fatalf("unexpected trailing chunk %#v", chunk)
			}
			if n := body.closes.Load(); n != 1 {
				t.Fatalf("observed output closure with %d body closes", n)
			}
		})
	}
}

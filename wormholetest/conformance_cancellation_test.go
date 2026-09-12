package wormholetest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type cancellationAfterStartProvider struct {
	*types.BaseProvider
	stream func(context.Context) (<-chan types.TextChunk, error)
}

func newCancellationAfterStartProvider(stream func(context.Context) (<-chan types.TextChunk, error)) *cancellationAfterStartProvider {
	return &cancellationAfterStartProvider{
		BaseProvider: types.NewBaseProvider("cancellation-after-start"),
		stream:       stream,
	}
}

func (p *cancellationAfterStartProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	return p.stream(ctx)
}

func TestCheckStreamCancellationAfterStart(t *testing.T) {
	t.Run("close", func(t *testing.T) {
		provider := newCancellationAfterStartProvider(func(ctx context.Context) (<-chan types.TextChunk, error) {
			stream := make(chan types.TextChunk, 1)
			stream <- types.TextChunk{Text: "started"}
			go func() {
				<-ctx.Done()
				close(stream)
			}()
			return stream, nil
		})
		if err := checkStreamCancellationAfterStart(provider, "test", 100*time.Millisecond); err != nil {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v", err)
		}
	})

	t.Run("cancellation then close", func(t *testing.T) {
		provider := newCancellationAfterStartProvider(func(ctx context.Context) (<-chan types.TextChunk, error) {
			stream := make(chan types.TextChunk, 2)
			stream <- types.TextChunk{Text: "started"}
			go func() {
				<-ctx.Done()
				stream <- types.TextChunk{Error: context.Canceled}
				close(stream)
			}()
			return stream, nil
		})
		if err := checkStreamCancellationAfterStart(provider, "test", 100*time.Millisecond); err != nil {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v", err)
		}
	})

	t.Run("cancellation without close", func(t *testing.T) {
		stop, done := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() {
			close(stop)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("fixture producer did not stop")
			}
		})
		provider := newCancellationAfterStartProvider(func(ctx context.Context) (<-chan types.TextChunk, error) {
			stream := make(chan types.TextChunk, 2)
			stream <- types.TextChunk{Text: "started"}
			go func() {
				defer close(done)
				<-ctx.Done()
				stream <- types.TextChunk{Error: context.Canceled}
				<-stop
			}()
			return stream, nil
		})
		err := checkStreamCancellationAfterStart(provider, "test", 10*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "did not close") {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v, want closure failure", err)
		}
	})

	t.Run("never starting", func(t *testing.T) {
		stop, done := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() {
			close(stop)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("fixture producer did not stop")
			}
		})
		provider := newCancellationAfterStartProvider(func(context.Context) (<-chan types.TextChunk, error) {
			stream := make(chan types.TextChunk)
			go func() {
				defer close(done)
				<-stop
			}()
			return stream, nil
		})
		err := checkStreamCancellationAfterStart(provider, "test", 10*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "first chunk") {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v, want first chunk failure", err)
		}
	})

	t.Run("ignored cancellation", func(t *testing.T) {
		stop, done := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() {
			close(stop)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("fixture producer did not stop")
			}
		})
		provider := newCancellationAfterStartProvider(func(context.Context) (<-chan types.TextChunk, error) {
			stream := make(chan types.TextChunk, 1)
			stream <- types.TextChunk{Text: "started"}
			go func() {
				defer close(done)
				<-stop
			}()
			return stream, nil
		})
		err := checkStreamCancellationAfterStart(provider, "test", 10*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "did not close") {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v, want closure failure", err)
		}
	})

	t.Run("blocking startup", func(t *testing.T) {
		provider := newCancellationAfterStartProvider(func(ctx context.Context) (<-chan types.TextChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		err := checkStreamCancellationAfterStart(provider, "test", 10*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "did not start") {
			t.Fatalf("checkStreamCancellationAfterStart() error = %v, want startup timeout", err)
		}
	})
}

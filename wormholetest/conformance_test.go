package wormholetest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestRunProviderConformanceWithMockProvider(t *testing.T) {
	t.Parallel()
	RunProviderConformance(t, ProviderConformanceConfig{
		Provider: NewMockProvider("mock").WithTextResponse(TextResponseWith("hello")),
	})
}

type cancellationConformanceProvider struct {
	*MockProvider
	closeOnCancel bool
	streamErr     error
	chunkErr      error
}

func (p *cancellationConformanceProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	if p.chunkErr != nil {
		stream := make(chan types.TextChunk, 1)
		stream <- types.TextChunk{Error: p.chunkErr}
		close(stream)
		return stream, nil
	}
	stream := make(chan types.TextChunk)
	if p.closeOnCancel {
		go func() {
			<-ctx.Done()
			close(stream)
		}()
	}
	return stream, nil
}

func TestCheckStreamCancellation(t *testing.T) {
	t.Parallel()

	passing := &cancellationConformanceProvider{
		MockProvider:  NewMockProvider("passing"),
		closeOnCancel: true,
	}
	if err := checkStreamCancellation(passing, "test", time.Second); err != nil {
		t.Fatalf("cancellation-aware provider failed: %v", err)
	}
	chunkCancellation := &cancellationConformanceProvider{
		MockProvider: NewMockProvider("chunk-cancellation"),
		chunkErr:     context.Canceled,
	}
	if err := checkStreamCancellation(chunkCancellation, "test", time.Second); err != nil {
		t.Fatalf("cancellation error chunk failed: %v", err)
	}

	stuck := &cancellationConformanceProvider{MockProvider: NewMockProvider("stuck")}
	err := checkStreamCancellation(stuck, "test", 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "neither reported cancellation nor closed") {
		t.Fatalf("stuck provider error = %v", err)
	}

	wrongError := &cancellationConformanceProvider{
		MockProvider: NewMockProvider("wrong-error"),
		streamErr:    errors.New("provider failed"),
	}
	err = checkStreamCancellation(wrongError, "test", time.Second)
	if err == nil || !strings.Contains(err.Error(), "non-cancellation error") {
		t.Fatalf("non-cancellation provider error = %v", err)
	}
}

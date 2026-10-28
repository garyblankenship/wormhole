package middleware

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestEnhancedStreamMetricsLifecycle(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		collector := NewEnhancedMetricsCollector(nil)
		upstream := make(chan types.TextChunk)
		handler := NewTypedEnhancedMetricsMiddleware(collector).ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
			<-time.NewTimer(time.Second).C
			return upstream, nil
		})
		out, err := handler(context.Background(), types.TextRequest{})
		if err != nil {
			t.Fatal(err)
		}
		chunkErr := errors.New("provider stream failed")
		frames := []types.TextChunk{
			{Error: chunkErr},
			{Text: "still forwarding"},
			{Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 3}},
			{Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 5}},
			{Usage: &types.Usage{PromptTokens: 8, CompletionTokens: 4}},
		}
		for _, frame := range frames {
			go func() { upstream <- frame }()
			if got := <-out; !reflect.DeepEqual(got, frame) {
				t.Fatalf("forwarded frame = %#v, want %#v", got, frame)
			}
			stats := collector.GetStats(nil)
			if stats["requests"] != int64(0) || stats["in_flight_requests"] != int64(1) {
				t.Fatalf("settled intermediate frame: %v", stats)
			}
		}
		<-time.NewTimer(2 * time.Second).C
		close(upstream)
		for range out {
			t.Fatal("unexpected extra frame")
		}
		stats := collector.GetStats(nil)
		if stats["requests"] != int64(1) || stats["errors"] != int64(1) || stats["in_flight_requests"] != int64(0) {
			t.Fatalf("settled counters = %v", stats)
		}
		if stats["input_tokens"] != int64(10) || stats["output_tokens"] != int64(5) {
			t.Fatalf("usage snapshots inflated: %v", stats)
		}
		if stats["total_duration_ns"] != int64(3*time.Second) {
			t.Fatalf("duration excludes stream or setup: %v", stats)
		}
	})
}

func TestStreamMetricsCancellationAndSetupFailure(t *testing.T) {
	t.Parallel()
	for _, blockedSend := range []bool{false, true} {
		t.Run(map[bool]string{false: "receive", true: "send"}[blockedSend], func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				collector := NewEnhancedMetricsCollector(nil)
				upstream := make(chan types.TextChunk)
				out, err := NewTypedEnhancedMetricsMiddleware(collector).ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
					return upstream, nil
				})(ctx, types.TextRequest{})
				if err != nil {
					t.Fatal(err)
				}
				if blockedSend {
					upstream <- types.TextChunk{Text: "unconsumed"}
				}
				synctest.Wait()
				cancel()
				for range out {
				}
				stats := collector.GetStats(nil)
				if stats["requests"] != int64(1) || stats["errors"] != int64(1) || stats["in_flight_requests"] != int64(0) {
					t.Fatalf("cancellation settlement = %v", stats)
				}
			})
		})
	}
	collector := NewEnhancedMetricsCollector(nil)
	setupErr := errors.New("setup failed")
	_, err := NewTypedEnhancedMetricsMiddleware(collector).ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return nil, setupErr
	})(context.Background(), types.TextRequest{})
	stats := collector.GetStats(nil)
	if err != setupErr || stats["requests"] != int64(1) || stats["in_flight_requests"] != int64(0) {
		t.Fatalf("setup settlement = (%v, %v)", err, stats)
	}
}

func TestStreamMetricsWrappersObserveChunkErrors(t *testing.T) {
	t.Parallel()
	typed := NewTypedMetrics()
	provider := NewProviderMetricsMiddleware("test")
	for _, wrap := range []func(types.StreamHandler) types.StreamHandler{
		NewTypedMetricsMiddleware(typed).ApplyStream,
		provider.ApplyStream,
	} {
		upstream := make(chan types.TextChunk, 2)
		streamErr := errors.New("chunk error")
		upstream <- types.TextChunk{Error: streamErr}
		upstream <- types.TextChunk{Text: "after error"}
		close(upstream)
		out, err := wrap(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
			return upstream, nil
		})(context.Background(), types.TextRequest{})
		if err != nil {
			t.Fatal(err)
		}
		var count int
		for range out {
			count++
		}
		if count != 2 {
			t.Fatalf("forwarded %d frames", count)
		}
	}
	requests, failures, _ := typed.GetStreamStats()
	if requests != 1 || failures != 1 || provider.GetMetrics().StreamRequests != 1 || provider.GetMetrics().TotalErrors != 1 {
		t.Fatalf("typed/provider stream errors = %d/%d, %#v", requests, failures, provider.GetMetrics())
	}
}

func TestStreamMetricsEstimateCompletionOnce(t *testing.T) {
	t.Parallel()
	collector := NewEnhancedMetricsCollector(nil)
	upstream := make(chan types.TextChunk, 2)
	upstream <- types.TextChunk{Text: "abc"}
	upstream <- types.TextChunk{Text: "def"}
	close(upstream)
	out, err := NewTypedEnhancedMetricsMiddleware(collector).ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return upstream, nil
	})(context.Background(), types.TextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if got := collector.GetStats(nil)["output_tokens"]; got != int64(1) {
		t.Fatalf("completion estimate = %v, want 1", got)
	}
}

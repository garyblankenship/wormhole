package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderMetricsMiddlewareRecordsAllHandlerTypes(t *testing.T) {
	t.Parallel()

	mw := NewProviderMetricsMiddleware("test")
	ctx := context.Background()

	_, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "ok"}, nil
	})(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: "text"}})
	if err != nil {
		t.Fatalf("text handler error: %v", err)
	}

	stream := make(chan types.TextChunk)
	close(stream)
	_, err = mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return stream, nil
	})(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: "stream"}})
	if err != nil {
		t.Fatalf("stream handler error: %v", err)
	}

	_, err = mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Data: map[string]any{"ok": true}}, nil
	})(ctx, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "structured"}})
	if err != nil {
		t.Fatalf("structured handler error: %v", err)
	}

	_, err = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{Embeddings: []types.Embedding{{Embedding: []float64{1}}}}, nil
	})(ctx, types.EmbeddingsRequest{Model: "embeddings", Input: []string{"hello"}})
	if err != nil {
		t.Fatalf("embeddings handler error: %v", err)
	}

	_, err = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{Text: "ok"}, nil
	})(ctx, types.AudioRequest{Model: "audio"})
	if err != nil {
		t.Fatalf("audio handler error: %v", err)
	}

	wantErr := errors.New("image failed")
	_, err = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return nil, wantErr
	})(ctx, types.ImageRequest{Model: "image", Prompt: "draw"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("image error = %v, want %v", err, wantErr)
	}

	metrics := mw.GetMetrics()
	if metrics.TextRequests != 1 ||
		metrics.StreamRequests != 1 ||
		metrics.StructuredRequests != 1 ||
		metrics.EmbeddingsRequests != 1 ||
		metrics.AudioRequests != 1 ||
		metrics.ImageRequests != 1 {
		t.Fatalf("request metrics = %#v", metrics)
	}
	if metrics.TotalErrors != 1 {
		t.Fatalf("TotalErrors = %d, want 1", metrics.TotalErrors)
	}
}

func TestJSONCleaningMiddlewareStructuredResponse(t *testing.T) {
	t.Parallel()

	mw := NewJSONCleaningMiddleware("gemini")
	handler := mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Raw: `{\"ok\":true}`}, nil
	})

	resp, err := handler(context.Background(), types.StructuredRequest{})
	if err != nil {
		t.Fatalf("structured handler error: %v", err)
	}
	if resp.Raw != `{"ok":true}` {
		t.Fatalf("Raw = %q, want cleaned JSON", resp.Raw)
	}

	wantErr := errors.New("provider")
	_, err = mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return nil, wantErr
	})(context.Background(), types.StructuredRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("structured error = %v, want %v", err, wantErr)
	}
}

func TestTypedMetricsMiddlewareRecordsAllHandlerTypes(t *testing.T) {
	t.Parallel()

	metrics := NewTypedMetrics()
	mw := NewTypedMetricsMiddleware(metrics)
	ctx := context.Background()

	_, _ = mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "ok"}, nil
	})(ctx, types.TextRequest{})

	stream := make(chan types.TextChunk)
	close(stream)
	_, _ = mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return stream, nil
	})(ctx, types.TextRequest{})

	wantErr := errors.New("structured failed")
	_, err := mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return nil, wantErr
	})(ctx, types.StructuredRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("structured error = %v, want %v", err, wantErr)
	}

	_, _ = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})(ctx, types.EmbeddingsRequest{})
	_, _ = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(ctx, types.AudioRequest{})
	_, _ = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{}, nil
	})(ctx, types.ImageRequest{})

	if req, errs, _ := metrics.GetTextStats(); req != 1 || errs != 0 {
		t.Fatalf("text stats = (%d, %d), want (1, 0)", req, errs)
	}
	if req, errs, _ := metrics.GetStreamStats(); req != 1 || errs != 0 {
		t.Fatalf("stream stats = (%d, %d), want (1, 0)", req, errs)
	}
	if req, errs, _ := metrics.GetStructuredStats(); req != 1 || errs != 1 {
		t.Fatalf("structured stats = (%d, %d), want (1, 1)", req, errs)
	}
	all := metrics.GetAllStats()
	if len(all) != 6 {
		t.Fatalf("GetAllStats len = %d, want 6", len(all))
	}

	metrics.Reset()
	if req, _, _ := metrics.GetTextStats(); req != 0 {
		t.Fatalf("text requests after Reset = %d, want 0", req)
	}
}

func TestTypedTimeoutMiddleware(t *testing.T) {
	t.Parallel()

	mw := NewTypedTimeoutMiddleware(20 * time.Millisecond)
	ctx := context.Background()

	resp, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "ok"}, nil
	})(ctx, types.TextRequest{})
	if err != nil {
		t.Fatalf("text handler error: %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("text response = %q, want ok", resp.Text)
	}

	timeoutMW := NewTypedTimeoutMiddleware(time.Nanosecond)
	_, err = timeoutMW.ApplyStructured(func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})(ctx, types.StructuredRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want deadline exceeded", err)
	}

	stream := make(chan types.StreamChunk, 1)
	stream <- types.StreamChunk{Text: "chunk"}
	close(stream)
	wrapped, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		return stream, nil
	})(ctx, types.TextRequest{})
	if err != nil {
		t.Fatalf("stream handler error: %v", err)
	}
	chunk, ok := <-wrapped
	if !ok || chunk.Text != "chunk" {
		t.Fatalf("wrapped stream first chunk = (%#v, %t), want chunk", chunk, ok)
	}

	_, _ = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})(ctx, types.EmbeddingsRequest{})
	_, _ = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(ctx, types.AudioRequest{})
	_, _ = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{}, nil
	})(ctx, types.ImageRequest{})
}

func TestTypedEnhancedMetricsMiddlewareRecordsAllHandlerTypes(t *testing.T) {
	t.Parallel()

	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{1, 10, 100},
		EnableLabels:            true,
		LabelAggregation:        true,
	})
	mw := NewTypedEnhancedMetricsMiddleware(collector)
	ctx := context.Background()

	_, _ = mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "response text"}, nil
	})(ctx, types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "text"},
		Messages:    []types.Message{types.NewUserMessage("hello world")},
	})
	stream := make(chan types.TextChunk)
	close(stream)
	_, _ = mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return stream, nil
	})(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: "stream"}})
	_, _ = mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Data: map[string]any{"ok": true}}, nil
	})(ctx, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "structured"}})
	_, _ = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{}, nil
	})(ctx, types.EmbeddingsRequest{Model: "embed", Input: []string{"hello world"}})
	_, _ = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(ctx, types.AudioRequest{Type: types.AudioRequestTypeTTS, Model: "audio", Input: "hello"})
	_, _ = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{}, nil
	})(ctx, types.ImageRequest{Model: "image", Prompt: "draw a square"})

	stats := collector.GetAllStats()
	perLabel, ok := stats["per_label"].(map[string]interface{})
	if !ok || len(perLabel) != 6 {
		t.Fatalf("per-label stats = %#v, want six label buckets", stats["per_label"])
	}
	if estimateTextTokens("") != 0 {
		t.Fatal("estimateTextTokens empty returned non-zero")
	}
	if estimateStructuredOutputTokens(nil) != 100 {
		t.Fatal("estimateStructuredOutputTokens returned unexpected estimate")
	}
}

func TestAdaptiveRateLimiterHealthAdjustment(t *testing.T) {
	t.Parallel()

	limiter := NewHealthAwareAdaptiveRateLimiter(10, 1, 20, 100*time.Millisecond)
	if limiter.healthMetrics == nil {
		t.Fatal("health-aware limiter did not initialize health metrics")
	}

	limiter.RecordHealthMetrics(&HealthMetrics{
		CircuitState:     StateOpen,
		Healthy:          false,
		ErrorRate:        1.2,
		ConsecutiveFails: 3,
	})
	if score := limiter.calculateHealthScore(); score != 0.0125 {
		t.Fatalf("health score = %v, want 0.0125", score)
	}
	if adjustment := limiter.calculateHealthAdjustment(); adjustment != 0.5 {
		t.Fatalf("health adjustment = %v, want 0.5", adjustment)
	}

	limiter.adjustRate(200 * time.Millisecond)
	if limiter.rate != 4 {
		t.Fatalf("adjusted rate = %d, want 4", limiter.rate)
	}

	limiter.healthMetrics = nil
	if limiter.calculateHealthScore() != 1.0 || limiter.calculateHealthAdjustment() != 1.0 {
		t.Fatal("nil health metrics did not return neutral score and adjustment")
	}
	if err := limiter.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestRateLimiterAccessorsAndClose(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(2)
	if got := limiter.GetAvailableTokens(); got <= 0 {
		t.Fatalf("GetAvailableTokens() = %d, want positive", got)
	}
	if err := limiter.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-03: TypedTimeoutMiddleware.ApplyStream must emit an error chunk when the
// timeout fires mid-stream instead of silently closing the stream.
func TestC403TypedTimeoutMiddlewareStreamEmitsTimeoutErrorChunk(t *testing.T) {
	t.Parallel()

	mw := NewTypedTimeoutMiddleware(25 * time.Millisecond)
	upstream := make(chan types.StreamChunk)

	wrapped, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		return upstream, nil
	})(context.Background(), types.TextRequest{})
	if err != nil {
		t.Fatalf("ApplyStream setup error: %v", err)
	}

	var gotErr error
	for chunk := range wrapped {
		if chunk.Error != nil {
			gotErr = chunk.Error
		}
	}
	if !errors.Is(gotErr, context.DeadlineExceeded) {
		t.Fatalf("stream error chunk = %v, want context.DeadlineExceeded", gotErr)
	}
}

// C4-04: ProviderLoggingMiddleware must not panic on nil responses or nil Usage.
func TestC404ProviderLoggingMiddlewareNilGuards(t *testing.T) {
	t.Parallel()

	mw := NewProviderLoggingMiddleware("test", nil)
	ctx := context.Background()

	if _, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "ok", Usage: nil}, nil
	})(ctx, types.TextRequest{}); err != nil {
		t.Fatalf("ApplyText nil Usage: %v", err)
	}
	if _, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return nil, nil
	})(ctx, types.TextRequest{}); err != nil {
		t.Fatalf("ApplyText nil resp: %v", err)
	}
	if _, err := mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return nil, nil
	})(ctx, types.StructuredRequest{}); err != nil {
		t.Fatalf("ApplyStructured nil resp: %v", err)
	}
	if _, err := mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return nil, nil
	})(ctx, types.EmbeddingsRequest{}); err != nil {
		t.Fatalf("ApplyEmbeddings nil resp: %v", err)
	}
	if _, err := mw.ApplyRerank(func(context.Context, types.RerankRequest) (*types.RerankResponse, error) {
		return nil, nil
	})(ctx, types.RerankRequest{}); err != nil {
		t.Fatalf("ApplyRerank nil resp: %v", err)
	}
	if _, err := mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return nil, nil
	})(ctx, types.ImageRequest{}); err != nil {
		t.Fatalf("ApplyImage nil resp: %v", err)
	}
}

// C4-05: withMeasuredStream with nil upstream channel must settle metrics and
// in-flight hooks immediately and return (nil, nil) without spawning a goroutine.
func TestC405MeasuredStreamNilUpstreamSettlesImmediately(t *testing.T) {
	t.Parallel()

	collector := NewEnhancedMetricsCollector(DefaultEnhancedMetricsConfig())
	mw := NewTypedEnhancedMetricsMiddleware(collector)

	stream, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
		return nil, nil
	})(context.Background(), types.TextRequest{})
	if err != nil {
		t.Fatalf("ApplyStream error: %v", err)
	}
	if stream != nil {
		t.Fatalf("expected nil stream when upstream is nil")
	}

	stats := collector.GetStats(nil)
	if stats["requests"] != int64(1) {
		t.Fatalf("requests = %v, want 1", stats["requests"])
	}
	if stats["in_flight_requests"] != int64(0) {
		t.Fatalf("in_flight_requests = %v, want 0", stats["in_flight_requests"])
	}
}

// C4-06: PrometheusExporter must emit cumulative histogram bucket counts.
func TestC406PrometheusHistogramBucketsAreCumulative(t *testing.T) {
	t.Parallel()

	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10, 50, 100},
	})
	collector.RecordRequest(nil, 5*time.Millisecond, nil, 0, 0, 0)   // <= 10ms
	collector.RecordRequest(nil, 25*time.Millisecond, nil, 0, 0, 0)  // <= 50ms
	collector.RecordRequest(nil, 250*time.Millisecond, nil, 0, 0, 0) // +Inf

	out := collector.PrometheusExporter()
	for _, want := range []string{
		`wormhole_duration_bucket{le="10.000000"} 1`,
		`wormhole_duration_bucket{le="50.000000"} 2`,
		`wormhole_duration_bucket{le="100.000000"} 2`,
		`wormhole_duration_bucket{le="+Inf"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("PrometheusExporter missing %q:\n%s", want, out)
		}
	}
}

// C4-10: JSONCleaningMiddleware.ApplyStructured must honor provider allowlist
// and update resp.Data when cleaning succeeds.
func TestC410JSONCleaningMiddlewareAllowlistAndDataUpdate(t *testing.T) {
	t.Parallel()

	mw := NewJSONCleaningMiddleware("gemini")
	raw := `{\"ok\":true}`
	next := func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Raw: raw}, nil
	}

	// Non-matching provider must not be cleaned.
	openaiCtx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	resp, err := mw.ApplyStructured(next)(openaiCtx, types.StructuredRequest{})
	if err != nil {
		t.Fatalf("openai ApplyStructured: %v", err)
	}
	if resp.Raw != raw || resp.Data != nil {
		t.Fatalf("non-allowlisted provider was unexpectedly cleaned: %+v", resp)
	}

	// Matching provider must clean Raw and populate Data.
	geminiCtx := context.WithValue(context.Background(), CtxKeyProvider, "Gemini")
	resp, err = mw.ApplyStructured(next)(geminiCtx, types.StructuredRequest{})
	if err != nil {
		t.Fatalf("gemini ApplyStructured: %v", err)
	}
	if resp.Raw != `{"ok":true}` {
		t.Fatalf("Raw = %q, want cleaned JSON", resp.Raw)
	}
	dataMap, ok := resp.Data.(map[string]any)
	if !ok || dataMap["ok"] != true {
		t.Fatalf("Data = %#v, want map[string]any{\"ok\": true}", resp.Data)
	}

	// Empty allowlist cleans all providers.
	allMW := NewJSONCleaningMiddleware()
	resp, err = allMW.ApplyStructured(next)(context.Background(), types.StructuredRequest{})
	if err != nil {
		t.Fatalf("allMW ApplyStructured: %v", err)
	}
	if resp.Raw != `{"ok":true}` || resp.Data == nil {
		t.Fatalf("empty allowlist did not clean: %+v", resp)
	}
}

// C4-12: TypedLoggingMiddleware.ApplyStream must return err even when LogErrors is false.
func TestC412TypedLoggingMiddlewareStreamReturnsErrorWhenLogErrorsFalse(t *testing.T) {
	t.Parallel()

	mw := NewTypedLoggingMiddleware(LoggingConfig{LogErrors: false})
	wantErr := errors.New("stream setup failed")
	nonNilStream := make(chan types.StreamChunk)

	stream, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) {
		return nonNilStream, wantErr
	})(context.Background(), types.TextRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if stream != nonNilStream {
		t.Fatalf("expected original stream to be returned alongside error")
	}
}

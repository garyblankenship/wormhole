package middleware

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

type countingMessage struct{ calls atomic.Int32 }

func (m *countingMessage) GetRole() types.Role { return types.RoleUser }
func (m *countingMessage) GetContent() any {
	m.calls.Add(1)
	return "must not be inspected"
}

type blockingRecordError struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingRecordError) Error() string {
	e.once.Do(func() { close(e.entered) })
	<-e.release
	return "blocked record"
}

func TestEnhancedMetricsCollector(t *testing.T) {
	t.Parallel()
	t.Run("records basic request metrics", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider:  "openai",
			Model:     "gpt-4",
			Method:    "text",
			ErrorType: "",
		}

		duration := 100 * time.Millisecond
		collector.RecordRequest(labels, duration, nil, 0, 100, 200)

		stats := collector.GetStats(labels)
		assert.Equal(t, int64(1), stats["requests"])
		assert.Equal(t, int64(0), stats["errors"])
		assert.Equal(t, int64(100), stats["input_tokens"])
		assert.Equal(t, int64(200), stats["output_tokens"])
	})

	t.Run("records error requests", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider:  "anthropic",
			Model:     "claude-3",
			Method:    "text",
			ErrorType: "",
		}

		duration := 50 * time.Millisecond
		err := fmt.Errorf("auth error: invalid API key")
		collector.RecordRequest(labels, duration, err, 2, 50, 0)

		stats := collector.GetStats(labels)
		assert.Equal(t, int64(1), stats["requests"])
		assert.Equal(t, int64(1), stats["errors"])
		assert.Equal(t, int64(2), stats["retries"])
	})

	t.Run("detects error types", func(t *testing.T) {
		t.Parallel()
		detector := &ErrorTypeDetector{}

		tests := []struct {
			name     string
			err      error
			expected string
		}{
			{"auth error", fmt.Errorf("auth error: invalid token"), "auth"},
			{"rate limit", fmt.Errorf("rate limit exceeded"), "rate_limit"},
			{"timeout", fmt.Errorf("context deadline exceeded"), "timeout"},
			{"provider error", fmt.Errorf("provider error: model not found"), "provider"},
			{"network error", fmt.Errorf("network error: connection refused"), "network"},
			{"unknown error", fmt.Errorf("some other error"), "unknown"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				result := detector.DetectErrorType(tt.err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("exports Prometheus format", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider:  "google",
			Model:     "gemini-pro",
			Method:    "text",
			ErrorType: "",
		}

		collector.RecordRequest(labels, 150*time.Millisecond, nil, 0, 75, 150)

		prometheusOutput := collector.PrometheusExporter()
		assert.Contains(t, prometheusOutput, "wormhole_requests_total")
		assert.Contains(t, prometheusOutput, "wormhole_duration_total_ns")
		assert.Contains(t, prometheusOutput, "wormhole_input_tokens_total")
		assert.Contains(t, prometheusOutput, "wormhole_output_tokens_total")
	})

	t.Run("exports JSON format", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		labels1 := &RequestLabels{
			Provider:  "openai",
			Model:     "gpt-4",
			Method:    "text",
			ErrorType: "",
		}

		labels2 := &RequestLabels{
			Provider:  "anthropic",
			Model:     "claude-3",
			Method:    "stream",
			ErrorType: "",
		}

		collector.RecordRequest(labels1, 100*time.Millisecond, nil, 0, 50, 100)
		collector.RecordRequest(labels2, 200*time.Millisecond, fmt.Errorf("error"), 1, 75, 0)

		jsonOutput := collector.JSONExporter()
		assert.NotNil(t, jsonOutput)
		assert.Contains(t, jsonOutput, "global")

		globalStats, ok := jsonOutput["global"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(2), globalStats["requests"])
		assert.Equal(t, int64(1), globalStats["errors"])
	})

	t.Run("resets metrics", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider:  "openai",
			Model:     "gpt-4",
			Method:    "text",
			ErrorType: "",
		}

		collector.RecordRequest(labels, 100*time.Millisecond, nil, 0, 50, 100)

		statsBefore := collector.GetStats(labels)
		assert.Equal(t, int64(1), statsBefore["requests"])

		collector.Reset()

		statsAfter := collector.GetStats(labels)
		assert.Equal(t, int64(0), statsAfter["requests"])
	})

	t.Run("handles label aggregation", func(t *testing.T) {
		t.Parallel()
		config := &EnhancedMetricsConfig{
			DefaultHistogramBuckets: []float64{10, 50, 100, 500, 1000},
			EnableLabels:            true,
			LabelAggregation:        true,
		}

		collector := NewEnhancedMetricsCollector(config)

		labels1 := &RequestLabels{
			Provider:  "openai",
			Model:     "gpt-4",
			Method:    "text",
			ErrorType: "",
		}

		labels2 := &RequestLabels{
			Provider:  "anthropic",
			Model:     "claude-3",
			Method:    "text",
			ErrorType: "",
		}

		collector.RecordRequest(labels1, 100*time.Millisecond, nil, 0, 50, 100)
		collector.RecordRequest(labels2, 200*time.Millisecond, nil, 0, 75, 150)

		allStats := collector.GetAllStats()
		assert.Contains(t, allStats, "per_label")

		perLabelStats, ok := allStats["per_label"].(map[string]interface{})
		require.True(t, ok)

		// Check that we have separate metrics for each label
		assert.Contains(t, perLabelStats, "provider=\"openai\",model=\"gpt-4\",method=\"text\",error_type=\"\"")
		assert.Contains(t, perLabelStats, "provider=\"anthropic\",model=\"claude-3\",method=\"text\",error_type=\"\"")
	})

	t.Run("handles nil labels", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)

		// Record without labels
		collector.RecordRequest(nil, 100*time.Millisecond, nil, 0, 0, 0)

		stats := collector.GetStats(nil)
		assert.Equal(t, int64(1), stats["requests"])
	})
}

func TestTypedEnhancedMetricsMiddleware(t *testing.T) {
	t.Parallel()
	t.Run("implements ProviderMiddleware interface", func(t *testing.T) {
		t.Parallel()
		collector := NewEnhancedMetricsCollector(nil)
		middleware := NewTypedEnhancedMetricsMiddleware(collector)

		// Verify it implements the interface by checking methods exist
		assert.NotNil(t, middleware.ApplyText)
		assert.NotNil(t, middleware.ApplyStream)
		assert.NotNil(t, middleware.ApplyStructured)
		assert.NotNil(t, middleware.ApplyEmbeddings)
		assert.NotNil(t, middleware.ApplyAudio)
		assert.NotNil(t, middleware.ApplyImage)
	})

	t.Run("extracts labels from context", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), CtxKeyWormholeProvider, "openai")

		labels := requestLabelsFromContext(ctx, "text", "gpt-4")
		assert.Equal(t, "openai", labels.Provider)
		assert.Equal(t, "gpt-4", labels.Model)
		assert.Equal(t, "text", labels.Method)
	})

	t.Run("falls back to unknown provider", func(t *testing.T) {
		t.Parallel()
		labels := requestLabelsFromContext(context.Background(), "stream", "claude-3")
		assert.Equal(t, "unknown", labels.Provider)
		assert.Equal(t, "claude-3", labels.Model)
		assert.Equal(t, "stream", labels.Method)
	})
}

func TestEnhancedMetricsConfig(t *testing.T) {
	t.Parallel()
	t.Run("default configuration", func(t *testing.T) {
		t.Parallel()
		config := DefaultEnhancedMetricsConfig()

		assert.NotEmpty(t, config.DefaultHistogramBuckets)
		assert.True(t, config.EnableLabels)
		assert.True(t, config.EnableTokenTracking)
		assert.True(t, config.EnableConcurrencyTracking)
		assert.False(t, config.LabelAggregation)
	})

	t.Run("custom configuration", func(t *testing.T) {
		t.Parallel()
		config := &EnhancedMetricsConfig{
			DefaultHistogramBuckets:   []float64{5, 25, 100, 250},
			EnableLabels:              false,
			EnableTokenTracking:       false,
			EnableConcurrencyTracking: false,
			LabelAggregation:          true,
		}

		collector := NewEnhancedMetricsCollector(config)
		assert.NotNil(t, collector)
	})
}

func TestEnhancedMetricsStructuredLabelsAndPrometheusGolden(t *testing.T) {
	t.Parallel()
	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10},
		EnableLabels:            true,
		EnableTokenTracking:     true,
		LabelAggregation:        true,
	})

	// These tuples have the same legacy colon-delimited String form but must
	// remain distinct structured identities.
	collector.RecordRequest(&RequestLabels{Provider: "a:b", Model: "c", Method: "d"}, time.Millisecond, nil, 0, 0, 0)
	collector.RecordRequest(&RequestLabels{Provider: "a", Model: "b:c", Method: "d"}, time.Millisecond, nil, 0, 0, 0)
	perLabel := collector.GetAllStats()["per_label"].(map[string]interface{})
	if len(perLabel) != 2 {
		t.Fatalf("per-label metric count = %d, want 2 distinct label tuples", len(perLabel))
	}

	collector = NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10},
		EnableLabels:            true,
		EnableTokenTracking:     true,
		LabelAggregation:        true,
	})
	collector.RecordRequest(&RequestLabels{Provider: "api:one", Model: `model"two`, Method: "text\\line\n"}, 5*time.Millisecond, nil, 0, 2, 3)
	got := collector.PrometheusExporter()
	want := "" +
		"wormhole_requests_total 0\n" +
		"wormhole_errors_total 0\n" +
		"wormhole_retries_total 0\n" +
		"wormhole_duration_total_ns 0\n" +
		"wormhole_input_tokens_total 0\n" +
		"wormhole_output_tokens_total 0\n" +
		"wormhole_duration_bucket{le=\"10.000000\"} 0\n" +
		"wormhole_duration_bucket{le=\"+Inf\"} 0\n" +
		"wormhole_requests_total{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 1\n" +
		"wormhole_errors_total{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 0\n" +
		"wormhole_retries_total{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 0\n" +
		"wormhole_duration_total_ns{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 5000000\n" +
		"wormhole_input_tokens_total{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 2\n" +
		"wormhole_output_tokens_total{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\"} 3\n" +
		"wormhole_duration_bucket{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\",le=\"10.000000\"} 1\n" +
		"wormhole_duration_bucket{provider=\"api:one\",model=\"model\\\"two\",method=\"text\\\\line\\n\",error_type=\"\",le=\"+Inf\"} 0\n"
	if got != want {
		t.Fatalf("PrometheusExporter() =\n%s\nwant:\n%s", got, want)
	}
}

func TestEnhancedMetricsDisabledTokenTrackingNeverReadsContent(t *testing.T) {
	t.Parallel()
	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10},
		EnableLabels:            true,
		EnableTokenTracking:     false,
	})
	mw := NewTypedEnhancedMetricsMiddleware(collector)
	message := &countingMessage{}
	_, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Text: "must not be inspected"}, nil
	})(context.Background(), types.TextRequest{Messages: []types.Message{message}})
	if err != nil {
		t.Fatalf("text handler: %v", err)
	}
	if got := message.calls.Load(); got != 0 {
		t.Fatalf("GetContent calls with token tracking disabled = %d, want 0", got)
	}
	stats := collector.GetStats(nil)
	if stats["input_tokens"] != int64(0) || stats["output_tokens"] != int64(0) {
		t.Fatalf("disabled token stats = %#v", stats)
	}
}

func TestEnhancedMetricsConcurrentRecordReadExportAndReset(t *testing.T) {
	t.Parallel()
	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10, 100},
		EnableLabels:            true,
		LabelAggregation:        true,
	})
	labels := &RequestLabels{Provider: "provider", Model: "model", Method: "text"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, work := range []func(){
		func() {
			for range 100 {
				collector.RecordRequest(labels, time.Millisecond, nil, 0, 1, 1)
			}
		},
		func() {
			for range 100 {
				_ = collector.GetStats(labels)
				_ = collector.GetAllStats()
			}
		},
		func() {
			for range 100 {
				_ = collector.PrometheusExporter()
			}
		},
		func() {
			for range 100 {
				collector.Reset()
			}
		},
	} {
		wg.Add(1)
		go func(work func()) {
			defer wg.Done()
			<-start
			work()
		}(work)
	}
	close(start)
	wg.Wait()
}

func TestEnhancedMetricsResetWaitsForActiveRecord(t *testing.T) {
	t.Parallel()
	collector := NewEnhancedMetricsCollector(DefaultEnhancedMetricsConfig())
	blockingErr := &blockingRecordError{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	recordDone := make(chan struct{})
	go func() {
		collector.RecordRequest(nil, time.Millisecond, blockingErr, 2, 3, 4)
		close(recordDone)
	}()
	<-blockingErr.entered

	resetStarted := make(chan struct{})
	resetDone := make(chan struct{})
	go func() {
		close(resetStarted)
		collector.Reset()
		close(resetDone)
	}()
	<-resetStarted
	for collector.stateMu.TryRLock() {
		collector.stateMu.RUnlock()
		runtime.Gosched()
	}
	select {
	case <-resetDone:
		t.Fatal("Reset completed while RecordRequest was inside its locked critical section")
	default:
	}
	close(blockingErr.release)
	<-recordDone
	<-resetDone

	stats := collector.GetStats(nil)
	assert.Equal(t, int64(0), stats["requests"])
	assert.Equal(t, int64(0), stats["errors"])
	assert.Equal(t, int64(0), stats["retries"])
	assert.Equal(t, int64(0), stats["total_duration_ns"])
	assert.Equal(t, "0s", stats["avg_duration"])
	assert.Equal(t, int64(0), stats["input_tokens"])
	assert.Equal(t, int64(0), stats["output_tokens"])
	assert.Equal(t, make([]int64, len(stats["histogram_counts"].([]int64))), stats["histogram_counts"])
}

func TestEnhancedMetricsConcurrencyGauge(t *testing.T) {
	t.Parallel()
	newCollector := func(enabled bool) *EnhancedMetricsCollector {
		return NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
			DefaultHistogramBuckets:   []float64{10},
			EnableConcurrencyTracking: enabled,
		})
	}

	t.Run("legacy", func(t *testing.T) {
		collector := newCollector(true)
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		handler := EnhancedMetricsMiddleware(collector)(func(context.Context, any) (any, error) {
			close(started)
			<-release
			return "ok", nil
		})
		go func() {
			_, _ = handler(context.Background(), nil)
			close(done)
		}()
		<-started
		assertActiveGauge(t, collector)
		collector.Reset()
		assertActiveGauge(t, collector)
		close(release)
		<-done
		assert.Equal(t, int64(0), collector.GetStats(nil)["in_flight_requests"])
	})

	t.Run("typed", func(t *testing.T) {
		collector := newCollector(true)
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		handler := NewTypedEnhancedMetricsMiddleware(collector).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
			close(started)
			<-release
			return &types.TextResponse{}, nil
		})
		go func() {
			_, _ = handler(context.Background(), types.TextRequest{})
			close(done)
		}()
		<-started
		assertActiveGauge(t, collector)
		close(release)
		<-done
		assert.Equal(t, int64(0), collector.GetAllStats()["global"].(map[string]interface{})["in_flight_requests"])
	})

	collector := newCollector(false)
	if _, ok := collector.GetStats(nil)["in_flight_requests"]; ok {
		t.Fatal("disabled concurrency tracking exposed in-flight stats")
	}
	if strings.Contains(collector.PrometheusExporter(), "wormhole_in_flight_requests") {
		t.Fatal("disabled concurrency tracking exported in-flight gauge")
	}
}

func TestEnhancedMetricsSnapshotsOwnConfigAndBuckets(t *testing.T) {
	t.Parallel()

	config := &EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10, 50},
		EnableTokenTracking:     true,
	}
	collector := NewEnhancedMetricsCollector(config)
	config.DefaultHistogramBuckets[0] = 999
	config.EnableTokenTracking = false

	collector.RecordRequest(nil, time.Millisecond, nil, 0, 3, 4)
	stats := collector.GetStats(nil)
	assert.Equal(t, int64(3), stats["input_tokens"])
	buckets := stats["histogram_buckets"].([]float64)
	assert.Equal(t, []float64{10, 50}, buckets)

	buckets[0] = 777
	assert.Equal(t, []float64{10, 50}, collector.GetStats(nil)["histogram_buckets"])
}

func TestEnhancedMetricsPerLabelStatsIncludeStructuredIdentity(t *testing.T) {
	t.Parallel()

	collector := NewEnhancedMetricsCollector(&EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10},
		EnableLabels:            true,
		LabelAggregation:        true,
	})
	labels := &RequestLabels{
		Provider:  "provider:with:colon",
		Model:     `model"quoted`,
		Method:    "text",
		ErrorType: "timeout",
	}
	collector.RecordRequest(labels, time.Millisecond, errors.New("timeout"), 0, 0, 0)

	perLabel := collector.GetAllStats()["per_label"].(map[string]interface{})
	require.Len(t, perLabel, 1)
	for _, raw := range perLabel {
		stats := raw.(map[string]interface{})
		assert.Equal(t, labels.Provider, stats["provider"])
		assert.Equal(t, labels.Model, stats["model"])
		assert.Equal(t, labels.Method, stats["method"])
		assert.Equal(t, labels.ErrorType, stats["error_type"])
	}
}

func assertActiveGauge(t *testing.T, collector *EnhancedMetricsCollector) {
	t.Helper()
	if got := collector.GetStats(nil)["in_flight_requests"]; got != int64(1) {
		t.Fatalf("GetStats in-flight gauge = %v, want 1", got)
	}
	global := collector.GetAllStats()["global"].(map[string]interface{})
	if got := global["in_flight_requests"]; got != int64(1) {
		t.Fatalf("GetAllStats global in-flight gauge = %v, want 1", got)
	}
	if !strings.Contains(collector.PrometheusExporter(), "wormhole_in_flight_requests 1\n") {
		t.Fatalf("PrometheusExporter missing active gauge:\n%s", collector.PrometheusExporter())
	}
}

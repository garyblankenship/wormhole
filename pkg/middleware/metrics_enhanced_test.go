package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnhancedMetricsCollector(t *testing.T) {
	t.Run("records basic request metrics", func(t *testing.T) {
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider: "openai",
			Model:    "gpt-4",
			Method:   "text",
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
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider: "anthropic",
			Model:    "claude-3",
			Method:   "text",
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
				result := detector.DetectErrorType(tt.err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("exports Prometheus format", func(t *testing.T) {
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider: "google",
			Model:    "gemini-pro",
			Method:   "text",
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
		collector := NewEnhancedMetricsCollector(nil)

		labels1 := &RequestLabels{
			Provider: "openai",
			Model:    "gpt-4",
			Method:   "text",
			ErrorType: "",
		}

		labels2 := &RequestLabels{
			Provider: "anthropic",
			Model:    "claude-3",
			Method:   "stream",
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
		collector := NewEnhancedMetricsCollector(nil)

		labels := &RequestLabels{
			Provider: "openai",
			Model:    "gpt-4",
			Method:   "text",
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
		config := &EnhancedMetricsConfig{
			DefaultHistogramBuckets: []float64{10, 50, 100, 500, 1000},
			EnableLabels:           true,
			LabelAggregation:       true,
		}

		collector := NewEnhancedMetricsCollector(config)

		labels1 := &RequestLabels{
			Provider: "openai",
			Model:    "gpt-4",
			Method:   "text",
			ErrorType: "",
		}

		labels2 := &RequestLabels{
			Provider: "anthropic",
			Model:    "claude-3",
			Method:   "text",
			ErrorType: "",
		}

		collector.RecordRequest(labels1, 100*time.Millisecond, nil, 0, 50, 100)
		collector.RecordRequest(labels2, 200*time.Millisecond, nil, 0, 75, 150)

		allStats := collector.GetAllStats()
		assert.Contains(t, allStats, "per_label")

		perLabelStats, ok := allStats["per_label"].(map[string]interface{})
		require.True(t, ok)

		// Check that we have separate metrics for each label
		assert.Contains(t, perLabelStats, "openai:gpt-4:text:")
		assert.Contains(t, perLabelStats, "anthropic:claude-3:text:")
	})

	t.Run("handles nil labels", func(t *testing.T) {
		collector := NewEnhancedMetricsCollector(nil)

		// Record without labels
		collector.RecordRequest(nil, 100*time.Millisecond, nil, 0, 0, 0)

		stats := collector.GetStats(nil)
		assert.Equal(t, int64(1), stats["requests"])
	})
}

func TestTypedEnhancedMetricsMiddleware(t *testing.T) {
	t.Run("implements ProviderMiddleware interface", func(t *testing.T) {
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
		collector := NewEnhancedMetricsCollector(nil)
		middleware := NewTypedEnhancedMetricsMiddleware(collector)

		ctx := context.WithValue(context.Background(), "wormhole_provider", "openai")

		labels := middleware.extractLabels(ctx, "text", "gpt-4")
		assert.Equal(t, "openai", labels.Provider)
		assert.Equal(t, "gpt-4", labels.Model)
		assert.Equal(t, "text", labels.Method)
	})

	t.Run("falls back to unknown provider", func(t *testing.T) {
		collector := NewEnhancedMetricsCollector(nil)
		middleware := NewTypedEnhancedMetricsMiddleware(collector)

		labels := middleware.extractLabels(context.Background(), "stream", "claude-3")
		assert.Equal(t, "unknown", labels.Provider)
		assert.Equal(t, "claude-3", labels.Model)
		assert.Equal(t, "stream", labels.Method)
	})
}

func TestEnhancedMetricsConfig(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		config := DefaultEnhancedMetricsConfig()

		assert.NotEmpty(t, config.DefaultHistogramBuckets)
		assert.True(t, config.EnableLabels)
		assert.True(t, config.EnableTokenTracking)
		assert.True(t, config.EnableConcurrencyTracking)
		assert.False(t, config.LabelAggregation)
	})

	t.Run("custom configuration", func(t *testing.T) {
		config := &EnhancedMetricsConfig{
			DefaultHistogramBuckets: []float64{5, 25, 100, 250},
			EnableLabels:           false,
			EnableTokenTracking:    false,
			EnableConcurrencyTracking: false,
			LabelAggregation:       true,
		}

		collector := NewEnhancedMetricsCollector(config)
		assert.NotNil(t, collector)
	})
}
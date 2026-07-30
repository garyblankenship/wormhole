package middleware

import (
	"sync"
	"testing"
	"time"
)

// BenchmarkEnhancedMetricsRecordRequestLabelHit measures the production
// steady-state label path. Its isolated sub-benchmarks make eager bucket
// construction attributable before Phase K considers a load-first experiment.
func BenchmarkEnhancedMetricsRecordRequestLabelHit(b *testing.B) {
	config := &EnhancedMetricsConfig{
		DefaultHistogramBuckets: []float64{10, 50, 100},
		EnableLabels:            true,
		EnableTokenTracking:     true,
		LabelAggregation:        true,
	}
	labels := &RequestLabels{Provider: "openai", Model: "gpt-5.6", Method: "text"}
	collector := NewEnhancedMetricsCollector(config)
	collector.RecordRequest(labels, time.Millisecond, nil, 0, 1, 1)

	b.Run("record-steady-hit", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			collector.RecordRequest(labels, time.Millisecond, nil, 0, 1, 1)
		}
	})

	b.Run("isolated-eager-construction", func(b *testing.B) {
		var buckets sync.Map
		key := labels.String()
		buckets.Store(key, newEnhancedMetricsBucket(config.DefaultHistogramBuckets))
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			candidate := newEnhancedMetricsBucket(config.DefaultHistogramBuckets)
			actual, loaded := buckets.LoadOrStore(key, candidate)
			if !loaded || actual == nil {
				b.Fatal("steady-state bucket lookup missed")
			}
		}
	})

	b.Run("isolated-load-first", func(b *testing.B) {
		var buckets sync.Map
		key := labels.String()
		buckets.Store(key, newEnhancedMetricsBucket(config.DefaultHistogramBuckets))
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if actual, ok := buckets.Load(key); !ok || actual == nil {
				b.Fatal("steady-state bucket lookup missed")
			}
		}
	})
}

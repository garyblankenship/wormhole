package middleware

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnhancedMetricsBoundsConcurrentLabelAdmission(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedMetricsConfig()
	config.LabelAggregation = true
	config.MaxLabelSets = 8
	collector := NewEnhancedMetricsCollector(config)

	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector.RecordRequest(&RequestLabels{
				Provider: "provider",
				Model:    fmt.Sprintf("model-%d", i),
				Method:   "text",
			}, time.Millisecond, nil, 0, 0, 0)
		}()
	}
	wg.Wait()

	all := collector.GetAllStats()
	perLabel := all["per_label"].(map[string]interface{})
	if len(perLabel) != 8 {
		t.Fatalf("admitted label sets = %d, want 8", len(perLabel))
	}
	global := all["global"].(map[string]interface{})
	if got := global["label_overflow_requests"]; got != int64(56) {
		t.Fatalf("label overflow = %v, want 56", got)
	}
	if got := global["requests"]; got != int64(56) {
		t.Fatalf("global overflow requests = %v, want 56", got)
	}
	if !strings.Contains(collector.PrometheusExporter(), "wormhole_label_overflow_requests_total 56\n") {
		t.Fatal("Prometheus output missing label overflow counter")
	}
}

func TestEnhancedMetricsResetClearsAdmissionAndOverflow(t *testing.T) {
	t.Parallel()

	config := DefaultEnhancedMetricsConfig()
	config.LabelAggregation = true
	config.MaxLabelSets = 1
	collector := NewEnhancedMetricsCollector(config)
	collector.RecordRequest(&RequestLabels{Provider: "a"}, time.Millisecond, nil, 0, 0, 0)
	collector.RecordRequest(&RequestLabels{Provider: "b"}, time.Millisecond, nil, 0, 0, 0)
	collector.Reset()
	collector.RecordRequest(&RequestLabels{Provider: "b"}, time.Millisecond, nil, 0, 0, 0)

	all := collector.JSONExporter()
	perLabel := all["per_label"].(map[string]interface{})
	if len(perLabel) != 1 {
		t.Fatalf("label sets after reset = %d, want 1", len(perLabel))
	}
	global := all["global"].(map[string]interface{})
	if got := global["label_overflow_requests"]; got != int64(0) {
		t.Fatalf("overflow after reset = %v, want 0", got)
	}
}

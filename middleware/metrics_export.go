package middleware

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// GetStats returns statistics for the given labels
func (c *EnhancedMetricsCollector) GetStats(labels *RequestLabels) map[string]interface{} {
	var bucket *enhancedMetricsBucket

	if c.config.LabelAggregation && labels != nil {
		key := labels.String()
		if val, ok := c.perLabel.Load(key); ok {
			bucket = val.(*enhancedMetricsBucket)
		} else {
			return make(map[string]interface{})
		}
	} else {
		bucket = c.global
	}

	return bucket.getStats(c.buckets)
}

// GetAllStats returns statistics for all labels
func (c *EnhancedMetricsCollector) GetAllStats() map[string]interface{} {
	result := make(map[string]interface{})

	// Add global stats
	result["global"] = c.global.getStats(c.buckets)

	// Add per-label stats if enabled
	if c.config.LabelAggregation {
		perLabelStats := make(map[string]interface{})
		c.perLabel.Range(func(key, value interface{}) bool {
			bucket := value.(*enhancedMetricsBucket)
			perLabelStats[key.(string)] = bucket.getStats(c.buckets)
			return true
		})
		result["per_label"] = perLabelStats
	}

	return result
}

// getStats returns statistics from a metrics bucket
func (b *enhancedMetricsBucket) getStats(buckets []float64) map[string]interface{} {
	requests := atomic.LoadInt64(&b.requests)
	errors := atomic.LoadInt64(&b.errors)
	retries := atomic.LoadInt64(&b.retries)
	totalDuration := atomic.LoadInt64(&b.totalDuration)
	inputTokens := atomic.LoadInt64(&b.inputTokens)
	outputTokens := atomic.LoadInt64(&b.outputTokens)

	avgDuration := time.Duration(0)
	if requests > 0 {
		avgDuration = time.Duration(totalDuration / requests)
	}

	// Get histogram counts
	histogramCounts := make([]int64, len(b.histogramCounts))
	for i := range b.histogramCounts {
		histogramCounts[i] = atomic.LoadInt64(&b.histogramCounts[i])
	}

	return map[string]interface{}{
		"requests":          requests,
		"errors":            errors,
		"retries":           retries,
		"total_duration_ns": totalDuration,
		"avg_duration":      avgDuration.String(),
		"input_tokens":      inputTokens,
		"output_tokens":     outputTokens,
		"histogram_buckets": buckets,
		"histogram_counts":  histogramCounts,
	}
}

// PrometheusExporter returns metrics in Prometheus format
func (c *EnhancedMetricsCollector) PrometheusExporter() string {
	var builder strings.Builder

	// Write global metrics
	builder.WriteString(c.global.prometheusFormat("", c.buckets))

	// Write per-label metrics if enabled
	if c.config.LabelAggregation {
		c.perLabel.Range(func(key, value interface{}) bool {
			bucket := value.(*enhancedMetricsBucket)
			builder.WriteString(bucket.prometheusFormat(key.(string), c.buckets))
			return true
		})
	}

	return builder.String()
}

// prometheusFormat returns Prometheus format metrics for a bucket
func (b *enhancedMetricsBucket) prometheusFormat(labels string, buckets []float64) string {
	var builder strings.Builder

	requests := atomic.LoadInt64(&b.requests)
	errors := atomic.LoadInt64(&b.errors)
	retries := atomic.LoadInt64(&b.retries)
	totalDuration := atomic.LoadInt64(&b.totalDuration)
	inputTokens := atomic.LoadInt64(&b.inputTokens)
	outputTokens := atomic.LoadInt64(&b.outputTokens)

	// Build label string
	labelStr := ""
	if labels != "" {
		labelStr = fmt.Sprintf("{%s}", labels)
	}

	// Write metrics
	fmt.Fprintf(&builder, "wormhole_requests_total%s %d\n", labelStr, requests)
	fmt.Fprintf(&builder, "wormhole_errors_total%s %d\n", labelStr, errors)
	fmt.Fprintf(&builder, "wormhole_retries_total%s %d\n", labelStr, retries)
	fmt.Fprintf(&builder, "wormhole_duration_total_ns%s %d\n", labelStr, totalDuration)
	fmt.Fprintf(&builder, "wormhole_input_tokens_total%s %d\n", labelStr, inputTokens)
	fmt.Fprintf(&builder, "wormhole_output_tokens_total%s %d\n", labelStr, outputTokens)

	// Write histogram (simplified)
	for i, count := range b.histogramCounts {
		if i < len(buckets) {
			fmt.Fprintf(&builder, "wormhole_duration_bucket{le=\"%f\"}%s %d\n", buckets[i], labelStr, count)
		} else {
			fmt.Fprintf(&builder, "wormhole_duration_bucket{le=\"+Inf\"}%s %d\n", labelStr, count)
		}
	}

	return builder.String()
}

// JSONExporter returns metrics in JSON format
func (c *EnhancedMetricsCollector) JSONExporter() map[string]interface{} {
	return c.GetAllStats()
}

// Reset clears all metrics
func (c *EnhancedMetricsCollector) Reset() {
	// Reset global bucket
	c.global = newEnhancedMetricsBucket(c.buckets)

	// Reset per-label buckets
	if c.config.LabelAggregation {
		c.perLabel = &sync.Map{}
	}
}

// Helper function to extract labels from request context
func ExtractLabelsFromRequest(ctx context.Context, req interface{}, method string) *RequestLabels {
	// This is a simplified implementation
	// In production, you'd extract provider and model from the request or context

	// Check if request has Provider() and Model() methods
	// This is a type-safe way to extract information
	return &RequestLabels{
		Provider:  "unknown",
		Model:     "unknown",
		Method:    method,
		ErrorType: "",
	}
}

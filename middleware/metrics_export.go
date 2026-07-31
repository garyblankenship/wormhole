package middleware

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// GetStats returns statistics for the given labels
func (c *EnhancedMetricsCollector) GetStats(labels *RequestLabels) map[string]interface{} {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	var bucket *enhancedMetricsBucket

	if c.config.LabelAggregation && labels != nil {
		key := requestLabelKeyFromLabels(labels, labels.ErrorType)
		if val, ok := c.perLabel.Load(key); ok {
			bucket = val.(*enhancedMetricsBucket)
		} else {
			return make(map[string]interface{})
		}
	} else {
		bucket = c.global
	}

	stats := bucket.getStats(c.buckets)
	if labels == nil {
		stats["label_overflow_requests"] = atomic.LoadInt64(&c.labelOverflowRequests)
		if c.config.EnableConcurrencyTracking {
			stats["in_flight_requests"] = atomic.LoadInt64(&c.inFlightRequests)
		}
	}
	return stats
}

// GetAllStats returns statistics for all labels
func (c *EnhancedMetricsCollector) GetAllStats() map[string]interface{} {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	result := make(map[string]interface{})

	// Add global stats
	global := c.global.getStats(c.buckets)
	global["label_overflow_requests"] = atomic.LoadInt64(&c.labelOverflowRequests)
	if c.config.EnableConcurrencyTracking {
		global["in_flight_requests"] = atomic.LoadInt64(&c.inFlightRequests)
	}
	result["global"] = global

	// Add per-label stats if enabled
	if c.config.LabelAggregation {
		perLabelStats := make(map[string]interface{})
		c.perLabel.Range(func(key, value any) bool {
			bucket := value.(*enhancedMetricsBucket)
			labelKey := key.(requestLabelKey)
			stats := bucket.getStats(c.buckets)
			stats["provider"] = labelKey.Provider
			stats["model"] = labelKey.Model
			stats["method"] = labelKey.Method
			stats["error_type"] = labelKey.ErrorType
			perLabelStats[canonicalLabelString(labelKey)] = stats
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
		"histogram_buckets": append([]float64(nil), buckets...),
		"histogram_counts":  histogramCounts,
	}
}

// PrometheusExporter returns metrics in Prometheus format
func (c *EnhancedMetricsCollector) PrometheusExporter() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	var builder strings.Builder

	// Write global metrics
	builder.WriteString(c.global.prometheusFormat(nil, c.buckets))
	fmt.Fprintf(&builder, "wormhole_label_overflow_requests_total %d\n", atomic.LoadInt64(&c.labelOverflowRequests))
	if c.config.EnableConcurrencyTracking {
		fmt.Fprintf(&builder, "wormhole_in_flight_requests %d\n", atomic.LoadInt64(&c.inFlightRequests))
	}

	// Write per-label metrics if enabled
	if c.config.LabelAggregation {
		type entry struct {
			key    requestLabelKey
			bucket *enhancedMetricsBucket
		}
		entries := make([]entry, 0)
		c.perLabel.Range(func(key, value any) bool {
			entries = append(entries, entry{key: key.(requestLabelKey), bucket: value.(*enhancedMetricsBucket)})
			return true
		})
		sort.Slice(entries, func(i, j int) bool {
			return canonicalLabelString(entries[i].key) < canonicalLabelString(entries[j].key)
		})
		for _, entry := range entries {
			builder.WriteString(entry.bucket.prometheusFormat(&entry.key, c.buckets))
		}
	}

	return builder.String()
}

// prometheusFormat returns Prometheus format metrics for a bucket
func (b *enhancedMetricsBucket) prometheusFormat(labels *requestLabelKey, buckets []float64) string {
	var builder strings.Builder

	requests := atomic.LoadInt64(&b.requests)
	errors := atomic.LoadInt64(&b.errors)
	retries := atomic.LoadInt64(&b.retries)
	totalDuration := atomic.LoadInt64(&b.totalDuration)
	inputTokens := atomic.LoadInt64(&b.inputTokens)
	outputTokens := atomic.LoadInt64(&b.outputTokens)

	// Build label string
	labelStr := prometheusLabelSet(labels, "")

	// Write metrics
	fmt.Fprintf(&builder, "wormhole_requests_total%s %d\n", labelStr, requests)
	fmt.Fprintf(&builder, "wormhole_errors_total%s %d\n", labelStr, errors)
	fmt.Fprintf(&builder, "wormhole_retries_total%s %d\n", labelStr, retries)
	fmt.Fprintf(&builder, "wormhole_duration_total_ns%s %d\n", labelStr, totalDuration)
	fmt.Fprintf(&builder, "wormhole_input_tokens_total%s %d\n", labelStr, inputTokens)
	fmt.Fprintf(&builder, "wormhole_output_tokens_total%s %d\n", labelStr, outputTokens)

	// Write histogram (simplified)
	for i := range b.histogramCounts {
		count := atomic.LoadInt64(&b.histogramCounts[i])
		le := "+Inf"
		if i < len(buckets) {
			le = fmt.Sprintf("%f", buckets[i])
		}
		fmt.Fprintf(&builder, "wormhole_duration_bucket%s %d\n", prometheusLabelSet(labels, le), count)
	}

	return builder.String()
}

func canonicalLabelString(key requestLabelKey) string {
	labelSet := prometheusLabelSet(&key, "")
	return labelSet[1 : len(labelSet)-1]
}

func prometheusLabelSet(key *requestLabelKey, le string) string {
	labels := make([]string, 0, 5)
	if key != nil {
		labels = append(labels,
			fmt.Sprintf("provider=\"%s\"", escapePrometheusLabelValue(key.Provider)),
			fmt.Sprintf("model=\"%s\"", escapePrometheusLabelValue(key.Model)),
			fmt.Sprintf("method=\"%s\"", escapePrometheusLabelValue(key.Method)),
			fmt.Sprintf("error_type=\"%s\"", escapePrometheusLabelValue(key.ErrorType)),
		)
	}
	if le != "" {
		labels = append(labels, fmt.Sprintf("le=\"%s\"", le))
	}
	if len(labels) == 0 {
		return ""
	}
	return "{" + strings.Join(labels, ",") + "}"
}

func escapePrometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

// JSONExporter returns metrics in JSON format
func (c *EnhancedMetricsCollector) JSONExporter() map[string]interface{} {
	return c.GetAllStats()
}

// Reset clears all metrics
func (c *EnhancedMetricsCollector) Reset() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.global.reset()
	c.perLabel.Clear()
	c.labelAdmissionMu.Lock()
	c.admittedLabelSets = 0
	c.labelAdmissionMu.Unlock()
	atomic.StoreInt64(&c.labelOverflowRequests, 0)
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

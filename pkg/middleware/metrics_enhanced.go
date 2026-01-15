package middleware

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EnhancedMetricsConfig holds configuration for enhanced metrics collection
type EnhancedMetricsConfig struct {
	// DefaultHistogramBuckets defines the default latency buckets in milliseconds
	DefaultHistogramBuckets []float64

	// EnableLabels controls whether label-based metrics are collected
	EnableLabels bool

	// EnableTokenTracking controls whether input/output token counts are tracked
	EnableTokenTracking bool

	// EnableConcurrencyTracking controls whether concurrent request gauge is maintained
	EnableConcurrencyTracking bool

	// LabelAggregation controls whether metrics are aggregated per-label or globally
	LabelAggregation bool
}

// DefaultEnhancedMetricsConfig returns the default configuration
func DefaultEnhancedMetricsConfig() *EnhancedMetricsConfig {
	return &EnhancedMetricsConfig{
		DefaultHistogramBuckets:  []float64{10, 50, 100, 500, 1000, 5000},
		EnableLabels:             true,
		EnableTokenTracking:      true,
		EnableConcurrencyTracking: true,
		LabelAggregation:         false,
	}
}

// RequestLabels represents the labels for a request
type RequestLabels struct {
	Provider string
	Model    string
	Method   string // text, stream, structured, embeddings, audio, image
	ErrorType string // auth, rate_limit, timeout, provider, network, unknown
}

// String returns a string representation of the labels for use as map key
func (l *RequestLabels) String() string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s:%s", l.Provider, l.Model, l.Method, l.ErrorType)
}

// EnhancedMetricsCollector collects enhanced metrics with labels and histograms
type EnhancedMetricsCollector struct {
	config *EnhancedMetricsConfig

	// Global metrics (if LabelAggregation is false or as fallback)
	global *enhancedMetricsBucket

	// Per-label metrics (if LabelAggregation is true)
	perLabel *sync.Map // map[string]*enhancedMetricsBucket

	// Histogram buckets (shared across all metrics)
	buckets []float64

	// Error type detection helper
	errorDetector *ErrorTypeDetector
}

// enhancedMetricsBucket holds metrics for a specific label combination
type enhancedMetricsBucket struct {
	// Basic counters
	requests       int64 // atomic
	errors         int64 // atomic
	retries        int64 // atomic
	totalDuration  int64 // atomic (nanoseconds)

	// Token counts (if enabled)
	inputTokens     int64 // atomic
	outputTokens    int64 // atomic

	// Histogram data - using fixed-size array with atomic operations
	histogramCounts []int64 // atomic slices for each bucket + overflow

	// Concurrency gauge (if enabled)
	concurrentRequests int64 // atomic
}

// ErrorTypeDetector categorizes errors by type
type ErrorTypeDetector struct{}

// DetectErrorType categorizes an error into known types
func (d *ErrorTypeDetector) DetectErrorType(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Check for common error patterns
	switch {
	case strings.Contains(errStr, "auth") || strings.Contains(errStr, "unauthorized") ||
	     strings.Contains(errStr, "token") || strings.Contains(errStr, "API key"):
		return "auth"
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "quota") ||
	     strings.Contains(errStr, "too many requests"):
		return "rate_limit"
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") ||
	     strings.Contains(errStr, "context deadline"):
		return "timeout"
	case strings.Contains(errStr, "provider") || strings.Contains(errStr, "model") ||
	     strings.Contains(errStr, "unsupported"):
		return "provider"
	case strings.Contains(errStr, "network") || strings.Contains(errStr, "connection") ||
	     strings.Contains(errStr, "dial") || strings.Contains(errStr, "EOF"):
		return "network"
	default:
		return "unknown"
	}
}

// NewEnhancedMetricsCollector creates a new enhanced metrics collector
func NewEnhancedMetricsCollector(config *EnhancedMetricsConfig) *EnhancedMetricsCollector {
	if config == nil {
		config = DefaultEnhancedMetricsConfig()
	}

	return &EnhancedMetricsCollector{
		config:        config,
		global:        newEnhancedMetricsBucket(config.DefaultHistogramBuckets),
		perLabel:      &sync.Map{},
		buckets:       config.DefaultHistogramBuckets,
		errorDetector: &ErrorTypeDetector{},
	}
}

// newEnhancedMetricsBucket creates a new metrics bucket with histogram
func newEnhancedMetricsBucket(buckets []float64) *enhancedMetricsBucket {
	return &enhancedMetricsBucket{
		histogramCounts: make([]int64, len(buckets)+1), // +1 for overflow bucket
	}
}

// RecordRequest records a request with enhanced metrics
func (c *EnhancedMetricsCollector) RecordRequest(labels *RequestLabels, duration time.Duration, err error, retries int, inputTokens, outputTokens int) {
	// Update error type if error exists
	errorType := ""
	if err != nil {
		errorType = c.errorDetector.DetectErrorType(err)
	}

	// Create or get labels if enabled
	var bucketLabels *RequestLabels
	if c.config.EnableLabels && labels != nil {
		bucketLabels = &RequestLabels{
			Provider:  labels.Provider,
			Model:     labels.Model,
			Method:    labels.Method,
			ErrorType: errorType,
		}
	}

	// Get the metrics bucket
	var bucket *enhancedMetricsBucket
	if c.config.LabelAggregation && bucketLabels != nil {
		key := bucketLabels.String()
		actual, _ := c.perLabel.LoadOrStore(key, newEnhancedMetricsBucket(c.buckets))
		bucket = actual.(*enhancedMetricsBucket)
	} else {
		bucket = c.global
	}

	// Record metrics
	bucket.record(c.buckets, duration, err != nil, retries, inputTokens, outputTokens)

	// Update concurrency gauge if enabled
	if c.config.EnableConcurrencyTracking {
		// Note: This is a simplified concurrency gauge. In production,
		// you'd want more sophisticated tracking with active request count
		// that increments at request start and decrements at request end
	}
}

// record updates a metrics bucket with a request
func (b *enhancedMetricsBucket) record(buckets []float64, duration time.Duration, isError bool, retries int, inputTokens, outputTokens int) {
	atomic.AddInt64(&b.requests, 1)
	atomic.AddInt64(&b.totalDuration, int64(duration))

	if isError {
		atomic.AddInt64(&b.errors, 1)
	}

	if retries > 0 {
		atomic.AddInt64(&b.retries, int64(retries))
	}

	if inputTokens > 0 {
		atomic.AddInt64(&b.inputTokens, int64(inputTokens))
	}

	if outputTokens > 0 {
		atomic.AddInt64(&b.outputTokens, int64(outputTokens))
	}

	// Update histogram
	durationMs := float64(duration.Milliseconds())
	bucketIndex := 0

	// Find the appropriate bucket
	for i, bucketValue := range buckets {
		if durationMs <= bucketValue {
			bucketIndex = i
			break
		}
		// If we reach the end, use overflow bucket
		if i == len(buckets)-1 {
			bucketIndex = len(buckets) // overflow bucket
		}
	}

	// Increment the appropriate bucket count
	if bucketIndex < len(b.histogramCounts) {
		atomic.AddInt64(&b.histogramCounts[bucketIndex], 1)
	}
}

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
		"requests":           requests,
		"errors":             errors,
		"retries":            retries,
		"total_duration_ns":  totalDuration,
		"avg_duration":       avgDuration.String(),
		"input_tokens":       inputTokens,
		"output_tokens":      outputTokens,
		"histogram_buckets":  buckets,
		"histogram_counts":   histogramCounts,
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
	builder.WriteString(fmt.Sprintf("wormhole_requests_total%s %d\n", labelStr, requests))
	builder.WriteString(fmt.Sprintf("wormhole_errors_total%s %d\n", labelStr, errors))
	builder.WriteString(fmt.Sprintf("wormhole_retries_total%s %d\n", labelStr, retries))
	builder.WriteString(fmt.Sprintf("wormhole_duration_total_ns%s %d\n", labelStr, totalDuration))
	builder.WriteString(fmt.Sprintf("wormhole_input_tokens_total%s %d\n", labelStr, inputTokens))
	builder.WriteString(fmt.Sprintf("wormhole_output_tokens_total%s %d\n", labelStr, outputTokens))

	// Write histogram (simplified)
	for i, count := range b.histogramCounts {
		if i < len(buckets) {
			builder.WriteString(fmt.Sprintf("wormhole_duration_bucket{le=\"%f\"}%s %d\n", buckets[i], labelStr, count))
		} else {
			builder.WriteString(fmt.Sprintf("wormhole_duration_bucket{le=\"+Inf\"}%s %d\n", labelStr, count))
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
		Provider: "unknown",
		Model:    "unknown",
		Method:   method,
		ErrorType: "",
	}
}
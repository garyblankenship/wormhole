package middleware

import (
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
		DefaultHistogramBuckets:   []float64{10, 50, 100, 500, 1000, 5000},
		EnableLabels:              true,
		EnableTokenTracking:       true,
		EnableConcurrencyTracking: true,
		LabelAggregation:          false,
	}
}

// RequestLabels represents the labels for a request
type RequestLabels struct {
	Provider  string
	Model     string
	Method    string // text, stream, structured, embeddings, audio, image
	ErrorType string // auth, rate_limit, timeout, provider, network, unknown
}

// requestLabelKey is the comparable identity used for per-label metrics.
// RequestLabels.String remains a compatibility display helper only.
type requestLabelKey struct {
	Provider  string
	Model     string
	Method    string
	ErrorType string
}

func requestLabelKeyFromLabels(labels *RequestLabels, errorType string) requestLabelKey {
	if labels == nil {
		return requestLabelKey{}
	}
	return requestLabelKey{
		Provider:  labels.Provider,
		Model:     labels.Model,
		Method:    labels.Method,
		ErrorType: errorType,
	}
}

// String returns the legacy compatibility display form. It is not a
// collision-free label identity; collector storage uses requestLabelKey.
func (l *RequestLabels) String() string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s:%s", l.Provider, l.Model, l.Method, l.ErrorType)
}

// EnhancedMetricsCollector collects enhanced metrics with labels and histograms
type EnhancedMetricsCollector struct {
	config  *EnhancedMetricsConfig
	stateMu sync.RWMutex

	// Global metrics (if LabelAggregation is false or as fallback)
	global *enhancedMetricsBucket

	// Per-label metrics (if LabelAggregation is true)
	perLabel sync.Map // map[requestLabelKey]*enhancedMetricsBucket

	// Histogram buckets (shared across all metrics)
	buckets []float64

	// Error type detection helper
	errorDetector *ErrorTypeDetector

	// inFlightRequests tracks active middleware handler invocations. It is live
	// state rather than a cumulative counter, so Reset deliberately preserves it.
	inFlightRequests int64
}

// enhancedMetricsBucket holds metrics for a specific label combination
type enhancedMetricsBucket struct {
	// Basic counters
	requests      int64 // atomic
	errors        int64 // atomic
	retries       int64 // atomic
	totalDuration int64 // atomic (nanoseconds)

	// Token counts (if enabled)
	inputTokens  int64 // atomic
	outputTokens int64 // atomic

	// Histogram data - using fixed-size array with atomic operations
	histogramCounts []int64 // atomic slices for each bucket + overflow
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
	configCopy := *config
	configCopy.DefaultHistogramBuckets = append([]float64(nil), config.DefaultHistogramBuckets...)

	return &EnhancedMetricsCollector{
		config:        &configCopy,
		global:        newEnhancedMetricsBucket(configCopy.DefaultHistogramBuckets),
		buckets:       configCopy.DefaultHistogramBuckets,
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
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	// Update error type if error exists
	errorType := ""
	if err != nil {
		errorType = c.errorDetector.DetectErrorType(err)
	}

	// Create or get labels if enabled
	var bucketKey requestLabelKey
	useLabels := false
	if c.config.EnableLabels && labels != nil {
		bucketKey = requestLabelKeyFromLabels(labels, errorType)
		useLabels = true
	}

	// Get the metrics bucket
	var bucket *enhancedMetricsBucket
	if c.config.LabelAggregation && useLabels {
		if actual, ok := c.perLabel.Load(bucketKey); ok {
			bucket = actual.(*enhancedMetricsBucket)
		} else {
			actual, _ := c.perLabel.LoadOrStore(bucketKey, newEnhancedMetricsBucket(c.buckets))
			bucket = actual.(*enhancedMetricsBucket)
		}
	} else {
		bucket = c.global
	}

	// Record metrics
	if !c.config.EnableTokenTracking {
		inputTokens = 0
		outputTokens = 0
	}
	bucket.record(c.buckets, duration, err != nil, retries, inputTokens, outputTokens)
}

func (c *EnhancedMetricsCollector) tokenTrackingEnabled() bool {
	return c.config.EnableTokenTracking
}

// beginRequest records an active middleware handler invocation when enabled.
// The returned function must be deferred by the middleware that owns it.
func (c *EnhancedMetricsCollector) beginRequest() func() {
	if !c.config.EnableConcurrencyTracking {
		return func() {}
	}
	atomic.AddInt64(&c.inFlightRequests, 1)
	return func() { atomic.AddInt64(&c.inFlightRequests, -1) }
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

func (b *enhancedMetricsBucket) reset() {
	atomic.StoreInt64(&b.requests, 0)
	atomic.StoreInt64(&b.errors, 0)
	atomic.StoreInt64(&b.retries, 0)
	atomic.StoreInt64(&b.totalDuration, 0)
	atomic.StoreInt64(&b.inputTokens, 0)
	atomic.StoreInt64(&b.outputTokens, 0)
	for i := range b.histogramCounts {
		atomic.StoreInt64(&b.histogramCounts[i], 0)
	}
}

package wormhole

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	testing_pkg "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// LoadTestConfig holds configuration for load testing
type LoadTestConfig struct {
	Duration         time.Duration
	Concurrency      int
	RequestsPerSec   int // Target requests per second (0 for unlimited)
	WarmupDuration   time.Duration
	CooldownDuration time.Duration
	EnableMetrics    bool
	EnableResource   bool
	ErrorRate        float64 // Percentage of requests that should error (0-100)
}

// LoadTestMetrics holds collected metrics during load testing
type LoadTestMetrics struct {
	TotalRequests     int64
	Successful        int64
	Failed            int64
	TotalDuration     time.Duration
	Throughput        float64 // requests per second
	LatencyP50        time.Duration
	LatencyP90        time.Duration
	LatencyP99        time.Duration
	LatencyMax        time.Duration
	LatencyMin        time.Duration
	ErrorRate         float64 // percentage
	MemoryAlloc       uint64
	TotalAlloc        uint64
	GoroutineCount    int
	GCPauses          []time.Duration
	StartMemStats     runtime.MemStats
	EndMemStats       runtime.MemStats
}

// ResourceMonitor tracks runtime resources during load tests
type ResourceMonitor struct {
	startMemStats runtime.MemStats
	endMemStats   runtime.MemStats
	gcPauses      []time.Duration
	maxGoroutines int
	samples       []goroutineSample
	stopChan      chan struct{}
	mu            sync.Mutex
}

type goroutineSample struct {
	time      time.Time
	count     int
	alloc     uint64
	totalAlloc uint64
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		gcPauses: make([]time.Duration, 0),
		samples:  make([]goroutineSample, 0),
		stopChan: make(chan struct{}),
	}
}

// Start begins monitoring resources
func (rm *ResourceMonitor) Start() {
	runtime.ReadMemStats(&rm.startMemStats)

	// Start goroutine sampling in background
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rm.sample()
			case <-rm.stopChan:
				return
			}
		}
	}()
}

// Stop ends monitoring and returns final stats
func (rm *ResourceMonitor) Stop() LoadTestMetrics {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if already stopped
	select {
	case <-rm.stopChan:
		// Already closed, return empty metrics
		return LoadTestMetrics{}
	default:
		close(rm.stopChan)
	}

	// Wait a bit for last sample
	time.Sleep(50 * time.Millisecond)

	runtime.ReadMemStats(&rm.endMemStats)

	var maxGoroutines int
	for _, sample := range rm.samples {
		if sample.count > maxGoroutines {
			maxGoroutines = sample.count
		}
	}

	return LoadTestMetrics{
		MemoryAlloc:    rm.endMemStats.Alloc,
		TotalAlloc:     rm.endMemStats.TotalAlloc,
		GoroutineCount: runtime.NumGoroutine(),
		GCPauses:       rm.gcPauses,
		StartMemStats:  rm.startMemStats,
		EndMemStats:    rm.endMemStats,
	}
}

func (rm *ResourceMonitor) sample() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	rm.samples = append(rm.samples, goroutineSample{
		time:      time.Now(),
		count:     runtime.NumGoroutine(),
		alloc:     memStats.Alloc,
		totalAlloc: memStats.TotalAlloc,
	})

	// Track GC pauses (simplified - track difference in pause times)
	currentPauseTotal := time.Duration(memStats.PauseTotalNs)
	if len(rm.gcPauses) == 0 {
		rm.gcPauses = append(rm.gcPauses, currentPauseTotal)
	} else {
		lastPause := rm.gcPauses[len(rm.gcPauses)-1]
		if currentPauseTotal > lastPause {
			rm.gcPauses = append(rm.gcPauses, currentPauseTotal-lastPause)
		}
	}
}

// calculatePercentiles calculates latency percentiles from sorted durations
func calculatePercentiles(latencies []time.Duration) (p50, p90, p99, minLatency, maxLatency time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0, 0
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	minLatency = latencies[0]
	maxLatency = latencies[len(latencies)-1]

	if len(latencies) >= 2 {
		p50 = latencies[len(latencies)*50/100]
	}
	if len(latencies) >= 10 {
		p90 = latencies[len(latencies)*90/100]
	}
	if len(latencies) >= 100 {
		p99 = latencies[len(latencies)*99/100]
	}

	return p50, p90, p99, minLatency, maxLatency
}

// TestLoadConcurrentRequests tests sustained concurrent load
func TestLoadConcurrentRequests(t *testing.T) {
	config := LoadTestConfig{
		Duration:         5 * time.Second, // Short for CI, can be longer for local
		Concurrency:      100,
		RequestsPerSec:   0, // Unlimited
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 100 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   true,
	}

	runLoadTest(t, config, "sustained_load")
}

// TestLoadHighConcurrency tests very high concurrency
func TestLoadHighConcurrency(t *testing.T) {
	config := LoadTestConfig{
		Duration:         3 * time.Second,
		Concurrency:      500,
		RequestsPerSec:   0,
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 100 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   true,
	}

	runLoadTest(t, config, "high_concurrency")
}

// TestLoadWithRateLimit tests load with rate limiting
func TestLoadWithRateLimit(t *testing.T) {
	config := LoadTestConfig{
		Duration:         3 * time.Second,
		Concurrency:      50,
		RequestsPerSec:   100, // 100 requests per second limit
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 100 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   false,
	}

	runLoadTest(t, config, "rate_limited")
}

// TestLoadWithErrorInjection tests error handling under load
func TestLoadWithErrorInjection(t *testing.T) {
	// Create mock provider that errors 20% of the time
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Hello, World!",
		Usage: &types.Usage{TotalTokens: 10},
	})

	// Create client with error-injecting provider
	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: &errorInjectingProvider{
					MockProvider: mockProvider,
					errorRate:    0.2, // 20% error rate
				},
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	config := LoadTestConfig{
		Duration:         3 * time.Second,
		Concurrency:      50,
		RequestsPerSec:   0,
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 100 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   false,
		ErrorRate:        20.0,
	}

	runLoadTestWithClient(t, config, client, "error_injection")
}

// errorInjectingProvider wraps MockProvider to inject errors
type errorInjectingProvider struct {
	*testing_pkg.MockProvider
	errorRate float64
	counter   atomic.Int64
}

func (p *errorInjectingProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	count := p.counter.Add(1)
	if float64(count%100) < p.errorRate {
		return nil, fmt.Errorf("injected error at request %d", count)
	}
	return p.MockProvider.Text(ctx, request)
}

// TestLoadWithMiddleware tests load with middleware chain
func TestLoadWithMiddleware(t *testing.T) {
	// Create middleware stack
	middlewares := []middleware.Middleware{
		func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
				// Rate limiting middleware
				time.Sleep(1 * time.Millisecond) // Simulate rate limit check
				return next(ctx, req)
			}
		},
		func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
				// Metrics middleware
				start := time.Now()
				resp, err := next(ctx, req)
				_ = time.Since(start) // Could record metrics here
				return resp, err
			}
		},
		func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
				// Circuit breaker middleware
				return next(ctx, req)
			}
		},
	}

	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Hello with middleware!",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return mockProvider, nil
		}),
		WithMiddleware(middlewares...),
	)

	config := LoadTestConfig{
		Duration:         3 * time.Second,
		Concurrency:      100,
		RequestsPerSec:   0,
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 100 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   false,
	}

	runLoadTestWithClient(t, config, client, "middleware_load")
}

// TestProviderPoolStress tests provider pool exhaustion and recovery
func TestProviderPoolStress(t *testing.T) {
	// Create multiple mock providers to simulate provider pool
	mockProvider1 := testing_pkg.NewMockProvider("mock1")
	mockProvider1.WithTextResponse(types.TextResponse{
		Text:  "Response from provider 1",
		Usage: &types.Usage{TotalTokens: 10},
	})

	mockProvider2 := testing_pkg.NewMockProvider("mock2")
	mockProvider2.WithTextResponse(types.TextResponse{
		Text:  "Response from provider 2",
		Usage: &types.Usage{TotalTokens: 10},
	})

	mockProvider3 := testing_pkg.NewMockProvider("mock3")
	mockProvider3.WithTextResponse(types.TextResponse{
		Text:  "Response from provider 3",
		Usage: &types.Usage{TotalTokens: 10},
	})

	// Create client with multiple providers
	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock1": {
				provider: mockProvider1,
				lastUsed: time.Now(),
				refCount: 1,
			},
			"mock2": {
				provider: mockProvider2,
				lastUsed: time.Now(),
				refCount: 1,
			},
			"mock3": {
				provider: mockProvider3,
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock1"},
		toolRegistry: NewToolRegistry(),
	}

	// Test with more concurrent requests than providers
	config := LoadTestConfig{
		Duration:         2 * time.Second,
		Concurrency:      10, // More than number of providers
		RequestsPerSec:   0,
		WarmupDuration:   50 * time.Millisecond,
		CooldownDuration: 50 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   true,
	}

	runLoadTestWithClient(t, config, client, "provider_pool_stress")
}

// TestMiddlewareChainDepth tests deep middleware chains under load
func TestMiddlewareChainDepth(t *testing.T) {
	// Create a deep middleware chain (10 layers)
	var middlewares []middleware.Middleware
	for i := 0; i < 10; i++ {
		middlewareLayer := func(layer int) middleware.Middleware {
			return func(next middleware.Handler) middleware.Handler {
				return func(ctx context.Context, req any) (any, error) {
					// Each layer adds 0.1ms delay
					time.Sleep(100 * time.Microsecond)
					return next(ctx, req)
				}
			}
		}(i)
		middlewares = append(middlewares, middlewareLayer)
	}

	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Response through deep middleware chain",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return mockProvider, nil
		}),
		WithMiddleware(middlewares...),
	)

	config := LoadTestConfig{
		Duration:         2 * time.Second,
		Concurrency:      50,
		RequestsPerSec:   0,
		WarmupDuration:   50 * time.Millisecond,
		CooldownDuration: 50 * time.Millisecond,
		EnableMetrics:    true,
		EnableResource:   false,
	}

	runLoadTestWithClient(t, config, client, "deep_middleware_chain")
}

// TestMemoryLeakDetection tests for memory leaks under sustained load
func TestMemoryLeakDetection(t *testing.T) {
	// This test runs multiple phases to detect memory leaks
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Memory test response",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	// Phase 1: Baseline memory
	var baselineMem runtime.MemStats
	runtime.ReadMemStats(&baselineMem)
	t.Logf("Baseline memory: Alloc=%d, TotalAlloc=%d", baselineMem.Alloc, baselineMem.TotalAlloc)

	// Phase 2: Sustained load
	config := LoadTestConfig{
		Duration:         5 * time.Second,
		Concurrency:      100,
		RequestsPerSec:   0,
		WarmupDuration:   100 * time.Millisecond,
		CooldownDuration: 500 * time.Millisecond, // Longer cooldown for GC
		EnableMetrics:    true,
		EnableResource:   true,
	}

	runLoadTestWithClient(t, config, client, "memory_leak_detection_phase1")

	// Phase 3: Force GC and check memory
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var postLoadMem runtime.MemStats
	runtime.ReadMemStats(&postLoadMem)

	// Calculate memory difference
	allocDiff := int64(postLoadMem.Alloc) - int64(baselineMem.Alloc)
	totalAllocDiff := int64(postLoadMem.TotalAlloc) - int64(baselineMem.TotalAlloc)

	t.Logf("Memory after load test:")
	t.Logf("  Alloc difference: %d bytes", allocDiff)
	t.Logf("  TotalAlloc difference: %d bytes", totalAllocDiff)

	// Reasonable threshold: less than 1MB retained after GC
	const maxAllowedLeak = 1024 * 1024 // 1MB
	if allocDiff > maxAllowedLeak {
		t.Errorf("Possible memory leak detected: retained %d bytes after GC (threshold: %d bytes)",
			allocDiff, maxAllowedLeak)
	}
}

// TestLoadWithMixedOperations tests mixed operations (text, embeddings, structured) under load
func TestLoadWithMixedOperations(t *testing.T) {
	// Create mock provider that supports all operations
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Text response",
		Usage: &types.Usage{TotalTokens: 10},
	})
	mockProvider.WithEmbeddings([]types.Embedding{
		{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
		{Index: 1, Embedding: []float64{0.4, 0.5, 0.6}},
	})
	mockProvider.WithStructuredData(map[string]any{
		"name": "Test",
		"age":  25,
	})

	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	// Run mixed operations test
	runMixedOperationsTest(t, client, "mixed_operations_load")
}

// runMixedOperationsTest runs load test with mixed operations
func runMixedOperationsTest(t *testing.T, client *Wormhole, testName string) {
	t.Run(testName, func(t *testing.T) {
		config := LoadTestConfig{
			Duration:         3 * time.Second,
			Concurrency:      50,
			RequestsPerSec:   0,
			WarmupDuration:   50 * time.Millisecond,
			CooldownDuration: 50 * time.Millisecond,
			EnableMetrics:    true,
			EnableResource:   true,
		}

		// Setup metrics
		var totalRequests atomic.Int64
		var textRequests atomic.Int64
		var embeddingsRequests atomic.Int64
		var structuredRequests atomic.Int64
		var successful atomic.Int64
		var failed atomic.Int64
		latencies := make(chan time.Duration, config.Concurrency*100)

		// Resource monitoring
		resourceMonitor := NewResourceMonitor()
		resourceMonitor.Start()
		defer func() {
			_ = resourceMonitor.Stop()
		}()

		// Warmup
		if config.WarmupDuration > 0 {
			time.Sleep(config.WarmupDuration)
		}

		startTime := time.Now()
		var wg sync.WaitGroup
		ctx := context.Background()

		// Start workers
		for i := 0; i < config.Concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				operationCounter := 0
				for time.Since(startTime) < config.Duration {
					// Cycle through different operations
					operationType := operationCounter % 3
					operationCounter++

					requestStart := time.Now()
					var err error

					switch operationType {
					case 0: // Text generation
						textRequests.Add(1)
						_, err = client.Text().
							Model("test-model").
							Prompt("test prompt").
							Generate(ctx)
					case 1: // Embeddings
						embeddingsRequests.Add(1)
						_, err = client.Embeddings().
							Model("embedding-model").
							Input("test input").
							Generate(ctx)
					case 2: // Structured generation
						structuredRequests.Add(1)
						schema := map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string"},
								"age":  map[string]any{"type": "integer"},
							},
						}
						_, err = client.Structured().
							Model("structured-model").
							Prompt("Generate test data").
							Schema(schema).
							Generate(ctx)
					}

					latency := time.Since(requestStart)
					totalRequests.Add(1)

					if err == nil {
						successful.Add(1)
					} else {
						failed.Add(1)
					}

					select {
					case latencies <- latency:
					default:
						// Channel full
					}
				}
			}(i)
		}

		wg.Wait()
		endTime := time.Now()

		// Cooldown
		if config.CooldownDuration > 0 {
			time.Sleep(config.CooldownDuration)
		}

		// Close latencies channel
		close(latencies)
		latencySlice := make([]time.Duration, 0, len(latencies))
		for latency := range latencies {
			latencySlice = append(latencySlice, latency)
		}

		// Calculate metrics
		totalDuration := endTime.Sub(startTime)
		throughput := float64(totalRequests.Load()) / totalDuration.Seconds()
		p50, p90, p99, minLatency, maxLatency := calculatePercentiles(latencySlice)

		total := totalRequests.Load()
		success := successful.Load()
		fail := failed.Load()
		errorRate := 0.0
		if total > 0 {
			errorRate = float64(fail) / float64(total) * 100
		}

		// Print detailed metrics
		t.Logf("Mixed Operations Load Test: %s", testName)
		t.Logf("  Total Requests: %d", total)
		t.Logf("  Text Requests: %d", textRequests.Load())
		t.Logf("  Embeddings Requests: %d", embeddingsRequests.Load())
		t.Logf("  Structured Requests: %d", structuredRequests.Load())
		t.Logf("  Successful: %d", success)
		t.Logf("  Failed: %d", fail)
		t.Logf("  Error Rate: %.2f%%", errorRate)
		t.Logf("  Throughput: %.2f req/sec", throughput)
		t.Logf("  Latency p50: %v", p50)
		t.Logf("  Latency p90: %v", p90)
		t.Logf("  Latency p99: %v", p99)
		t.Logf("  Latency min: %v", minLatency)
		t.Logf("  Latency max: %v", maxLatency)
		t.Logf("  Operation Mix: Text=%.1f%%, Embeddings=%.1f%%, Structured=%.1f%%",
			float64(textRequests.Load())/float64(total)*100,
			float64(embeddingsRequests.Load())/float64(total)*100,
			float64(structuredRequests.Load())/float64(total)*100)

		// Assertions
		if errorRate > 1.0 {
			t.Errorf("Error rate too high for mixed operations: %.2f%%", errorRate)
		}
		if total == 0 {
			t.Error("No requests processed during mixed operations test")
		}
	})
}

// runLoadTest runs a generic load test
func runLoadTest(t *testing.T, config LoadTestConfig, testName string) {
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Hello, World!",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	runLoadTestWithClient(t, config, client, testName)
}

// runLoadTestWithClient runs load test with specific client
func runLoadTestWithClient(t *testing.T, config LoadTestConfig, client *Wormhole, testName string) {
	t.Run(testName, func(t *testing.T) {
		// Validate config
		if config.Concurrency <= 0 {
			t.Fatal("Concurrency must be > 0")
		}
		if config.Duration <= 0 {
			t.Fatal("Duration must be > 0")
		}

		// Setup metrics collection
		var totalRequests atomic.Int64
		var successful atomic.Int64
		var failed atomic.Int64
		latencies := make(chan time.Duration, config.Concurrency*100)

		// Setup resource monitoring if enabled
		var resourceMonitor *ResourceMonitor
		if config.EnableResource {
			resourceMonitor = NewResourceMonitor()
			resourceMonitor.Start()
			defer func() {
				_ = resourceMonitor.Stop() // Results printed below
			}()
		}

		// Warmup phase
		if config.WarmupDuration > 0 {
			time.Sleep(config.WarmupDuration)
		}

		// Rate limiter if RequestsPerSec > 0
		var rateLimiter <-chan time.Time
		if config.RequestsPerSec > 0 {
			interval := time.Second / time.Duration(config.RequestsPerSec)
			rateLimiter = time.Tick(interval)
		}

		// Start time for throughput calculation
		startTime := time.Now()

		// Worker pool
		var wg sync.WaitGroup
		ctx := context.Background()

		// Start workers
		for i := 0; i < config.Concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				// Worker loop
				for time.Since(startTime) < config.Duration {
					// Apply rate limiting if configured
					if rateLimiter != nil {
						<-rateLimiter
					}

					// Make request
					requestStart := time.Now()
					_, err := client.Text().
						Model("test-model").
						Prompt("test prompt").
						Generate(ctx)
					latency := time.Since(requestStart)

					// Record metrics
					totalRequests.Add(1)
					if err == nil {
						successful.Add(1)
					} else {
						failed.Add(1)
					}

					// Record latency
					select {
					case latencies <- latency:
					default:
						// Channel full, skip this latency
					}
				}
			}(i)
		}

		// Wait for workers to complete
		wg.Wait()
		endTime := time.Now()

		// Cooldown phase
		if config.CooldownDuration > 0 {
			time.Sleep(config.CooldownDuration)
		}

		// Close latencies channel and collect data
		close(latencies)
		latencySlice := make([]time.Duration, 0, len(latencies))
		for latency := range latencies {
			latencySlice = append(latencySlice, latency)
		}

		// Calculate metrics
		totalDuration := endTime.Sub(startTime)
		throughput := float64(totalRequests.Load()) / totalDuration.Seconds()

		p50, p90, p99, minLatency, maxLatency := calculatePercentiles(latencySlice)

		total := totalRequests.Load()
		success := successful.Load()
		fail := failed.Load()
		errorRate := 0.0
		if total > 0 {
			errorRate = float64(fail) / float64(total) * 100
		}

		// Print metrics
		t.Logf("Load Test: %s", testName)
		t.Logf("  Duration: %v", totalDuration)
		t.Logf("  Concurrency: %d", config.Concurrency)
		t.Logf("  Total Requests: %d", total)
		t.Logf("  Successful: %d", success)
		t.Logf("  Failed: %d", fail)
		t.Logf("  Error Rate: %.2f%%", errorRate)
		t.Logf("  Throughput: %.2f req/sec", throughput)
		t.Logf("  Latency p50: %v", p50)
		t.Logf("  Latency p90: %v", p90)
		t.Logf("  Latency p99: %v", p99)
		t.Logf("  Latency min: %v", minLatency)
		t.Logf("  Latency max: %v", maxLatency)

		if config.EnableResource && resourceMonitor != nil {
			resourceMetrics := resourceMonitor.Stop()
			t.Logf("  Memory Allocation: %d bytes", resourceMetrics.MemoryAlloc)
			t.Logf("  Total Allocation: %d bytes", resourceMetrics.TotalAlloc)
			t.Logf("  Goroutine Count: %d", resourceMetrics.GoroutineCount)
			t.Logf("  GC Pauses: %d occurrences", len(resourceMetrics.GCPauses))
		}

		// Assertions
		if config.ErrorRate > 0 {
			// For error injection tests, we expect some failures
			if fail == 0 {
				t.Errorf("Expected some failures with error rate %.1f%%, but got 0", config.ErrorRate)
			}
		} else {
			// For normal tests, error rate should be very low
			if errorRate > 1.0 {
				t.Errorf("Error rate too high: %.2f%% (expected < 1%%)", errorRate)
			}
		}

		// Ensure we processed requests
		if total == 0 {
			t.Error("No requests processed during load test")
		}

		// Log if test was rate limited
		if config.RequestsPerSec > 0 {
			achievedRPS := throughput
			t.Logf("  Target RPS: %d, Achieved RPS: %.2f", config.RequestsPerSec, achievedRPS)
			if achievedRPS > float64(config.RequestsPerSec)*1.1 {
				t.Errorf("Rate limiting not effective: target %d RPS, achieved %.2f RPS",
					config.RequestsPerSec, achievedRPS)
			}
		}
	})
}

// BenchmarkLoadSustained benchmarks sustained load performance
func BenchmarkLoadSustained(b *testing.B) {
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Benchmark response",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now(),
				refCount: 1,
			},
		},
		config:        Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	// Use RunParallel for concurrent benchmarking
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Text().
				Model("benchmark-model").
				Prompt("benchmark prompt").
				Generate(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkLoadWithMiddleware benchmarks load with middleware
func BenchmarkLoadWithMiddleware(b *testing.B) {
	// Create simple middleware for benchmarking
	middlewares := []middleware.Middleware{
		func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
				return next(ctx, req)
			}
		},
	}

	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Benchmark with middleware",
		Usage: &types.Usage{TotalTokens: 10},
	})

	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return mockProvider, nil
		}),
		WithMiddleware(middlewares...),
	)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Text().
				Model("benchmark-model").
				Prompt("benchmark prompt").
				Generate(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
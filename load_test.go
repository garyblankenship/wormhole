package wormhole

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
	testingpkg "github.com/garyblankenship/wormhole/v3/wormholetest"
)

type LoadTestConfig struct {
	Duration, WarmupDuration, CooldownDuration time.Duration
	Concurrency, RequestsPerSec                int
	EnableMetrics, EnableResource              bool
	ErrorRate                                  float64
}

type LoadTestMetrics struct {
	TotalRequests, Successful, Failed int64
	TotalDuration                     time.Duration
	Throughput                        float64
	LatencyP50, LatencyP90            time.Duration
	LatencyP99, LatencyMax            time.Duration
	LatencyMin                        time.Duration
	ErrorRate                         float64
	MemoryAlloc, TotalAlloc           uint64
	GoroutineCount                    int
	GCPauses                          []time.Duration
	StartMemStats, EndMemStats        runtime.MemStats
}

type ResourceMonitor struct {
	startMemStats, endMemStats runtime.MemStats
	gcPauses                   []time.Duration
	samples                    []goroutineSample
	stopChan                   chan struct{}
	mu                         sync.Mutex
}

type goroutineSample struct {
	count int
	alloc uint64
}

func skipLoadTestInShortMode(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
}

func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{gcPauses: make([]time.Duration, 0), samples: make([]goroutineSample, 0), stopChan: make(chan struct{})}
}

func (rm *ResourceMonitor) Start() {
	runtime.ReadMemStats(&rm.startMemStats)
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

func (rm *ResourceMonitor) Stop() LoadTestMetrics {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	select {
	case <-rm.stopChan:
		return LoadTestMetrics{}
	default:
		close(rm.stopChan)
	}
	time.Sleep(50 * time.Millisecond)
	runtime.ReadMemStats(&rm.endMemStats)
	return LoadTestMetrics{MemoryAlloc: rm.endMemStats.Alloc, TotalAlloc: rm.endMemStats.TotalAlloc, GoroutineCount: runtime.NumGoroutine(), GCPauses: rm.gcPauses, StartMemStats: rm.startMemStats, EndMemStats: rm.endMemStats}
}

func (rm *ResourceMonitor) sample() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	rm.samples = append(rm.samples, goroutineSample{count: runtime.NumGoroutine(), alloc: stats.Alloc})
	if stats.PauseTotalNs > 0 && stats.PauseTotalNs <= 1<<63-1 {
		pause := time.Duration(int64(stats.PauseTotalNs)) //nolint:gosec // bounds checked above
		if len(rm.gcPauses) == 0 || pause > rm.gcPauses[len(rm.gcPauses)-1] {
			rm.gcPauses = append(rm.gcPauses, pause)
		}
	}
}

func TestLoadConcurrentRequests(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	runLoadTest(t, LoadTestConfig{Duration: 5 * time.Second, Concurrency: 100, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 100 * time.Millisecond, EnableMetrics: true, EnableResource: true}, "sustained_load")
}
func TestLoadHighConcurrency(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	runLoadTest(t, LoadTestConfig{Duration: 3 * time.Second, Concurrency: 500, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 100 * time.Millisecond, EnableMetrics: true, EnableResource: true}, "high_concurrency")
}
func TestLoadWithRateLimit(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	runLoadTest(t, LoadTestConfig{Duration: 3 * time.Second, Concurrency: 50, RequestsPerSec: 100, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 100 * time.Millisecond, EnableMetrics: true}, "rate_limited")
}

type errorInjectingProvider struct {
	*testingpkg.MockProvider
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

func TestLoadWithErrorInjection(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	mock := testingpkg.NewMockProvider("mock")
	mock.WithTextResponse(types.TextResponse{Text: "Hello, World!", Usage: &types.Usage{TotalTokens: 10}})
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
		return &errorInjectingProvider{MockProvider: mock, errorRate: .2}, nil
	}))
	t.Cleanup(func() { _ = client.Close() })
	runLoadTestWithClient(t, LoadTestConfig{Duration: 3 * time.Second, Concurrency: 50, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 100 * time.Millisecond, EnableMetrics: true, ErrorRate: 20}, client, "error_injection")
}

func TestLoadWithMiddleware(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	mock := testingpkg.NewMockProvider("mock")
	mock.WithTextResponse(types.TextResponse{Text: "Hello with middleware!", Usage: &types.Usage{TotalTokens: 10}})
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return mock, nil }), WithProviderMiddleware(middleware.NewTypedRateLimitMiddleware(1000), middleware.NewTypedMetricsMiddleware(middleware.NewTypedMetrics()), middleware.NewTypedCircuitBreakerMiddleware(5, time.Second)))
	t.Cleanup(func() { _ = client.Close() })
	runLoadTestWithClient(t, LoadTestConfig{Duration: 3 * time.Second, Concurrency: 100, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 100 * time.Millisecond, EnableMetrics: true}, client, "middleware_load")
}

func TestProviderPoolStress(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	client := loadTestMockClient("mock1", "Response from provider 1", 10)
	t.Cleanup(func() { _ = client.Close() })
	runLoadTestWithClient(t, LoadTestConfig{Duration: 2 * time.Second, Concurrency: 10, WarmupDuration: 50 * time.Millisecond, CooldownDuration: 50 * time.Millisecond, EnableMetrics: true, EnableResource: true}, client, "provider_pool_stress")
}

func TestMiddlewareChainDepth(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	stack := make([]types.ProviderMiddleware, 10)
	for i := range stack {
		stack[i] = middleware.NewTypedTimeoutMiddleware(time.Second)
	}
	mock := testingpkg.NewMockProvider("mock")
	mock.WithTextResponse(types.TextResponse{Text: "Response through deep middleware chain", Usage: &types.Usage{TotalTokens: 10}})
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return mock, nil }), WithProviderMiddleware(stack...))
	t.Cleanup(func() { _ = client.Close() })
	runLoadTestWithClient(t, LoadTestConfig{Duration: 2 * time.Second, Concurrency: 50, WarmupDuration: 50 * time.Millisecond, CooldownDuration: 50 * time.Millisecond, EnableMetrics: true}, client, "deep_middleware_chain")
}

func TestMemoryLeakDetection(t *testing.T) {
	skipLoadTestInShortMode(t)
	client := loadTestMockClient("mock", "Memory test response", 10)
	t.Cleanup(func() { _ = client.Close() })
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	runLoadTestWithClient(t, LoadTestConfig{Duration: 5 * time.Second, Concurrency: 100, WarmupDuration: 100 * time.Millisecond, CooldownDuration: 500 * time.Millisecond, EnableMetrics: true, EnableResource: true}, client, "memory_leak_detection_phase1")
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.Alloc > baseline.Alloc && after.Alloc-baseline.Alloc > 1024*1024 {
		t.Errorf("possible memory leak: retained %d bytes", after.Alloc-baseline.Alloc)
	}
}

func TestLoadWithMixedOperations(t *testing.T) {
	t.Parallel()
	skipLoadTestInShortMode(t)
	mock := testingpkg.NewMockProvider("mock")
	mock.WithTextResponse(types.TextResponse{Text: "Text response", Usage: &types.Usage{TotalTokens: 10}})
	mock.WithEmbeddings([]types.Embedding{{Index: 0, Embedding: []float64{.1, .2, .3}}})
	mock.WithStructuredData(map[string]any{"name": "Test", "age": 25})
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return mock, nil }))
	t.Cleanup(func() { _ = client.Close() })
	runMixedOperationsTest(t, client, "mixed_operations_load")
}

func runMixedOperationsTest(t *testing.T, client *Wormhole, name string) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		config := LoadTestConfig{Duration: 3 * time.Second, Concurrency: 50, WarmupDuration: 50 * time.Millisecond, CooldownDuration: 50 * time.Millisecond, EnableResource: true}
		var total, text, embeddings, structured, successful, failed atomic.Int64
		monitor := NewResourceMonitor()
		monitor.Start()
		t.Cleanup(func() { _ = monitor.Stop() })
		time.Sleep(config.WarmupDuration)
		start := time.Now()
		var wg sync.WaitGroup
		for range config.Concurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for operation := 0; time.Since(start) < config.Duration; operation++ {
					var err error
					switch operation % 3 {
					case 0:
						text.Add(1)
						_, err = client.Text().Model("test-model").Prompt("test prompt").Generate(context.Background())
					case 1:
						embeddings.Add(1)
						_, err = client.Embeddings().Model("embedding-model").Input("test input").Generate(context.Background())
					case 2:
						structured.Add(1)
						_, err = client.Structured().Model("structured-model").Prompt("Generate test data").Schema(map[string]any{"type": "object"}).Generate(context.Background())
					}
					total.Add(1)
					if err == nil {
						successful.Add(1)
					} else {
						failed.Add(1)
					}
				}
			}()
		}
		wg.Wait()
		time.Sleep(config.CooldownDuration)
		if total.Load() == 0 || successful.Load()+failed.Load() != total.Load() || text.Load()+embeddings.Load()+structured.Load() != total.Load() {
			t.Fatalf("mixed operation accounting invalid: total=%d text=%d embeddings=%d structured=%d", total.Load(), text.Load(), embeddings.Load(), structured.Load())
		}
		if rate := float64(failed.Load()) / float64(total.Load()) * 100; rate > 1 {
			t.Errorf("mixed operation error rate %.2f%% exceeds 1%%", rate)
		}
	})
}

func runLoadTest(t *testing.T, config LoadTestConfig, name string) {
	client := loadTestMockClient("mock", "Hello, World!", 10)
	t.Cleanup(func() { _ = client.Close() })
	runLoadTestWithClient(t, config, client, name)
}

func runLoadTestWithClient(t *testing.T, config LoadTestConfig, client *Wormhole, name string) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		if config.Concurrency <= 0 || config.Duration <= 0 {
			t.Fatal("load test concurrency and duration must be positive")
		}
		var total, successful, failed atomic.Int64
		var monitor *ResourceMonitor
		if config.EnableResource {
			monitor = NewResourceMonitor()
			monitor.Start()
			t.Cleanup(func() { _ = monitor.Stop() })
		}
		if config.WarmupDuration > 0 {
			time.Sleep(config.WarmupDuration)
		}
		var ticker *time.Ticker
		if config.RequestsPerSec > 0 {
			ticker = time.NewTicker(time.Second / time.Duration(config.RequestsPerSec))
			t.Cleanup(ticker.Stop)
		}
		start := time.Now()
		var wg sync.WaitGroup
		for range config.Concurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for time.Since(start) < config.Duration {
					if ticker != nil {
						<-ticker.C
					}
					_, err := client.Text().Model("test-model").Prompt("test prompt").Generate(context.Background())
					total.Add(1)
					if err == nil {
						successful.Add(1)
					} else {
						failed.Add(1)
					}
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)
		if config.CooldownDuration > 0 {
			time.Sleep(config.CooldownDuration)
		}
		if total.Load() == 0 {
			t.Fatal("no requests processed during load test")
		}
		rate := float64(failed.Load()) / float64(total.Load()) * 100
		if config.ErrorRate > 0 {
			if failed.Load() == 0 {
				t.Errorf("expected failures with configured error rate %.1f%%", config.ErrorRate)
			}
		} else if rate > 1 {
			t.Errorf("error rate %.2f%% exceeds 1%%", rate)
		}
		if config.RequestsPerSec > 0 && float64(total.Load())/elapsed.Seconds() > float64(config.RequestsPerSec)*1.1 {
			t.Errorf("rate limiting not effective: target %d RPS", config.RequestsPerSec)
		}
	})
}

func loadTestMockClient(name, text string, tokens int) *Wormhole {
	mock := testingpkg.NewMockProvider(name)
	mock.WithTextResponse(types.TextResponse{Text: text, Usage: &types.Usage{TotalTokens: tokens}})
	return New(WithDefaultProvider(name), WithCustomProvider(name, func(types.ProviderConfig) (types.Provider, error) { return mock, nil }))
}

func BenchmarkLoadSustained(b *testing.B) {
	client := loadTestMockClient("mock", "Benchmark response", 10)
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := client.Text().Model("benchmark-model").Prompt("benchmark prompt").Generate(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkLoadWithMiddleware(b *testing.B) {
	mock := testingpkg.NewMockProvider("mock")
	mock.WithTextResponse(types.TextResponse{Text: "Benchmark with middleware", Usage: &types.Usage{TotalTokens: 10}})
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return mock, nil }), WithProviderMiddleware(middleware.NewTypedTimeoutMiddleware(time.Second)))
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := client.Text().Model("benchmark-model").Prompt("benchmark prompt").Generate(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}

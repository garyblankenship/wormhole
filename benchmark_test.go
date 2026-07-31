package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
	testing_pkg "github.com/garyblankenship/wormhole/v3/wormholetest"
)

func createMockClient(name, response string, _ int) *Wormhole {
	provider := testing_pkg.NewMockProvider(name)
	provider.WithTextResponse(types.TextResponse{Text: response})
	return New(WithDefaultProvider(name), WithCustomProvider(name, testing_pkg.MockProviderFactory(provider)), WithModelValidation(false), WithDiscovery(false))
}

// BenchmarkTextGeneration measures the performance of text generation requests
func BenchmarkTextGeneration(b *testing.B) {
	// Create mock provider for consistent benchmarking
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Hello, World!",
		Usage: &types.Usage{TotalTokens: 10},
	})

	// Create Wormhole client with mock provider
	client := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now().UnixNano(),
				refCount: 1,
			},
		},
		config:       Config{DefaultProvider: "mock"},
		toolRegistry: NewToolRegistry(),
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Text().
			Model("gpt-3.5-turbo").
			Prompt("Hello").
			Generate(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEmbeddings measures the performance of embeddings generation
func BenchmarkEmbeddings(b *testing.B) {
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithEmbeddings([]types.Embedding{{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}}})

	client := &Wormhole{
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now().UnixNano(),
				refCount: 1,
			},
		},
		config: Config{DefaultProvider: "mock"},
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Embeddings().
			Model("text-embedding-ada-002").
			Input("test text").
			Generate(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStructuredGeneration measures structured output performance
func BenchmarkStructuredGeneration(b *testing.B) {
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithStructuredData(map[string]any{"name": "John", "age": 30})

	client := &Wormhole{
		providers: map[string]*cachedProvider{
			"mock": {
				provider: mockProvider,
				lastUsed: time.Now().UnixNano(),
				refCount: 1,
			},
		},
		config: Config{DefaultProvider: "mock"},
	}

	ctx := context.Background()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Structured().
			Model("gpt-3.5-turbo").
			Prompt("Generate a person").
			Schema(schema).
			Generate(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWithProviderMiddleware measures typed middleware overhead.
func BenchmarkWithProviderMiddleware(b *testing.B) {
	mockProvider := testing_pkg.NewMockProvider("mock")
	mockProvider.WithTextResponse(types.TextResponse{
		Text:  "Hello with middleware!",
		Usage: &types.Usage{TotalTokens: 12},
	})

	// Create client with custom provider and middleware stack using functional options
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return mockProvider, nil
		}),
		WithProviderMiddleware(
			middleware.NewTypedRateLimitMiddleware(1000),
			middleware.NewTypedMetricsMiddleware(middleware.NewTypedMetrics()),
			middleware.NewTypedCircuitBreakerMiddleware(10, time.Second),
		),
	)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Text().
			Model("gpt-3.5-turbo").
			Prompt("Hello").
			Generate(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConcurrent measures performance under concurrent load
func BenchmarkConcurrent(b *testing.B) {
	client := createMockClient("mock", "Concurrent response", 8)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Text().
				Model("gpt-3.5-turbo").
				Prompt("Hello concurrent").
				Generate(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkProviderInitialization measures provider creation overhead
func BenchmarkProviderInitialization(b *testing.B) {
	mockProvider := testing_pkg.NewMockProvider("mock")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		client := New(
			WithDefaultProvider("mock"),
			WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
				return mockProvider, nil
			}),
		)
		_ = client
	}
}

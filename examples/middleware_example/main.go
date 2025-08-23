package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Create client with comprehensive middleware stack using functional options
	client := wormhole.New(
		wormhole.WithOpenAI(""), // Will use env var OPENAI_API_KEY
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithMiddleware(
			middleware.RateLimitMiddleware(10),                          // 10 requests per second
			middleware.RetryMiddleware(middleware.DefaultRetryConfig()), // Auto-retry on failures
			middleware.CircuitBreakerMiddleware(5, 30*time.Second),      // Circuit breaker protection
			middleware.TimeoutMiddleware(30*time.Second),                // Request timeout
			middleware.CacheMiddleware(middleware.CacheConfig{
				Cache: middleware.NewMemoryCache(100),
				TTL:   5 * time.Minute,
			}),
		),
	)

	// Example: Rate limiting in action
	fmt.Println("Testing rate limiting...")
	for i := 0; i < 3; i++ {
		resp, err := client.Text().
			Model("gpt-3.5-turbo").
			Prompt(fmt.Sprintf("Say 'Response %d'", i+1)).
			MaxTokens(10).
			Generate(context.Background())

		if err != nil {
			log.Printf("Request %d failed: %v", i+1, err)
		} else {
			fmt.Printf("Request %d: %s\n", i+1, resp.Text)
		}
	}

	// Example: Load balancing across providers
	fmt.Println("\nLoad balancing example...")

	// Create multiple provider handlers
	providers := map[string]middleware.Handler{
		"primary": func(ctx context.Context, req any) (any, error) {
			// Primary provider logic
			return "Response from primary", nil
		},
		"secondary": func(ctx context.Context, req any) (any, error) {
			// Secondary provider logic
			return "Response from secondary", nil
		},
	}

	// Create load balancer
	lb := middleware.LoadBalancerMiddleware(middleware.RoundRobin, providers)

	// Apply to chain
	chain := middleware.NewChain(lb)
	handler := chain.Apply(func(ctx context.Context, req any) (any, error) {
		return "Default response", nil
	})

	// Test load balancing
	for i := 0; i < 4; i++ {
		resp, err := handler(context.Background(), nil)
		if err != nil {
			log.Printf("Load balanced request %d failed: %v", i+1, err)
		} else {
			fmt.Printf("Load balanced request %d: %v\n", i+1, resp)
		}
	}

	// Example: Adaptive rate limiting
	fmt.Println("\nAdaptive rate limiting...")
	_ = wormhole.New(
		wormhole.WithOpenAI(""),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithMiddleware(
			middleware.AdaptiveRateLimitMiddleware(
				5,                    // Initial rate
				2,                    // Min rate
				20,                   // Max rate
				100*time.Millisecond, // Target latency
			),
		),
	)

	fmt.Println("Client configured with adaptive rate limiting based on latency")

	// Example: Health checking
	fmt.Println("\nHealth checking setup...")
	checker := middleware.NewHealthChecker(30 * time.Second)
	checker.SetCheckFunction(func(ctx context.Context, provider string) error {
		// Custom health check logic
		fmt.Printf("Health check for provider: %s\n", provider)
		return nil
	})

	checker.Start([]string{"openai", "anthropic"})
	defer checker.Stop()

	// Example: Metrics collection
	fmt.Println("\nMetrics collection...")
	metrics := middleware.NewMetrics()
	metricsClient := wormhole.New(
		wormhole.WithOpenAI(""),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithMiddleware(middleware.MetricsMiddleware(metrics)),
	)

	// Make a request to collect metrics
	ctx := context.Background()
	_, _ = metricsClient.Text().
		Model("gpt-3.5-turbo").
		Prompt("Hello").
		MaxTokens(5).
		Generate(ctx)

	requests, errors, avgDuration := metrics.GetStats()
	fmt.Printf("Metrics - Requests: %d, Errors: %d, Avg Duration: %v\n",
		requests, errors, avgDuration)

	// NEW: Debug logging middleware demo
	fmt.Println("\nDebug logging middleware...")
	debugClient := wormhole.New(
		wormhole.WithOpenAI(""),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithMiddleware(middleware.DebugLoggingMiddleware(nil)), // Uses default logger
	)

	_, err := debugClient.Text().
		Model("gpt-3.5-turbo").
		Prompt("Test debug logging").
		MaxTokens(5).
		Generate(ctx)

	if err != nil {
		fmt.Printf("Debug request completed (may have failed): %v\n", err)
	}

	// NEW: Structured error handling with middleware
	fmt.Println("\nStructured error handling...")
	errorClient := wormhole.New(
		wormhole.WithOpenAI(""),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithMiddleware(middleware.RetryMiddleware(middleware.DefaultRetryConfig())),
	)

	_, err = errorClient.Text().
		Model("invalid-model-name").
		Prompt("This will fail").
		Generate(ctx)

	if err != nil {
		if wormholeErr, ok := types.AsWormholeError(err); ok {
			fmt.Printf("Structured error - Code: %s, Retryable: %v, Message: %s\n",
				wormholeErr.Code, wormholeErr.IsRetryable(), wormholeErr.Message)
		} else {
			fmt.Printf("Non-wormhole error: %v\n", err)
		}
	}

	// NEW: Custom provider with middleware demo
	fmt.Println("\nCustom provider with middleware...")
	customClient := wormhole.New(
		wormhole.WithProviderConfig("mock", types.ProviderConfig{
			APIKey:  "test",
			BaseURL: "http://localhost",
		}),
		wormhole.WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return &MockProvider{}, nil
		}),
		wormhole.WithMiddleware(
			middleware.MetricsMiddleware(middleware.NewMetrics()),
			middleware.TimeoutMiddleware(5*time.Second),
		),
	)

	fmt.Printf("Custom provider configured with middleware stack (provider: %v)\n", customClient != nil)

	// NEW: Multi-provider fallback demo
	fmt.Println("\nMulti-provider fallback with middleware...")

	// This would require OpenRouter configuration, so just show the pattern
	fmt.Println("Example pattern for provider fallback:")
	fmt.Println("1. Try primary provider (OpenAI)")
	fmt.Println("2. Circuit breaker triggers on failures")
	fmt.Println("3. Fallback to secondary provider (OpenRouter)")
	fmt.Println("4. Metrics track provider performance")
	fmt.Println("5. Health checker monitors all providers")

	fmt.Println("\n🎯 ALL MIDDLEWARE FEATURES DEMONSTRATED!")
	fmt.Println("✅ Rate limiting, retries, circuit breaker, caching")
	fmt.Println("✅ Load balancing, health checking, metrics")
	fmt.Println("✅ Debug logging, structured errors")
	fmt.Println("✅ Custom provider integration")
	fmt.Println("✅ Multi-provider fallback patterns")
}

// MockProvider for demonstration
type MockProvider struct{}

func (p *MockProvider) Name() string { return "mock" }
func (p *MockProvider) Text(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
	return &types.TextResponse{Text: "Mock response", Model: req.Model}, nil
}
func (p *MockProvider) Stream(ctx context.Context, req types.TextRequest) (<-chan types.TextChunk, error) {
	ch := make(chan types.TextChunk)
	close(ch)
	return ch, nil
}
func (p *MockProvider) Structured(ctx context.Context, req types.StructuredRequest) (*types.StructuredResponse, error) {
	return &types.StructuredResponse{Data: map[string]any{"mock": true}}, nil
}
func (p *MockProvider) Embeddings(ctx context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return &types.EmbeddingsResponse{}, nil
}
func (p *MockProvider) Audio(ctx context.Context, req types.AudioRequest) (*types.AudioResponse, error) {
	return &types.AudioResponse{}, nil
}
func (p *MockProvider) Images(ctx context.Context, req types.ImagesRequest) (*types.ImagesResponse, error) {
	return &types.ImagesResponse{}, nil
}

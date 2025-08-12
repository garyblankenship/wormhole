package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Create client with simple factory
	client := wormhole.QuickOpenAI()

	// Add comprehensive middleware stack
	client = client.
		Use(middleware.RateLimitMiddleware(10)).                          // 10 requests per second
		Use(middleware.RetryMiddleware(middleware.DefaultRetryConfig())). // Auto-retry on failures
		Use(middleware.CircuitBreakerMiddleware(5, 30*time.Second)).      // Circuit breaker protection
		Use(middleware.TimeoutMiddleware(30 * time.Second)).              // Request timeout
		Use(middleware.CacheMiddleware(middleware.CacheConfig{
			Cache: middleware.NewMemoryCache(100),
			TTL:   5 * time.Minute,
		}))

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
		"primary": func(ctx context.Context, req interface{}) (interface{}, error) {
			// Primary provider logic
			return "Response from primary", nil
		},
		"secondary": func(ctx context.Context, req interface{}) (interface{}, error) {
			// Secondary provider logic
			return "Response from secondary", nil
		},
	}

	// Create load balancer
	lb := middleware.LoadBalancerMiddleware(middleware.RoundRobin, providers)

	// Apply to chain
	chain := middleware.NewChain(lb)
	handler := chain.Apply(func(ctx context.Context, req interface{}) (interface{}, error) {
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
	_ = wormhole.QuickOpenAI().
		Use(middleware.AdaptiveRateLimitMiddleware(
			5,                    // Initial rate
			2,                    // Min rate
			20,                   // Max rate
			100*time.Millisecond, // Target latency
		))

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
	metricsClient := wormhole.QuickOpenAI().
		Use(middleware.MetricsMiddleware(metrics))

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

	fmt.Println("\nMiddleware examples completed!")
}

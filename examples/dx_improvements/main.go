package main

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Example addressing the specific DX issues from meesix feedback
func main() {
	fmt.Println("=== Wormhole DX Improvements Demo ===")

	// 1. MIDDLEWARE DISCOVERY - No more source diving!
	fmt.Println("\n1. Available Middleware:")
	for _, mw := range middleware.AvailableMiddleware() {
		fmt.Printf("  %-25s %s\n", mw.Name+":", mw.Purpose)
		fmt.Printf("  %-25s %s\n", "", mw.Example)
		fmt.Printf("  %-25s Config: %s\n", "", mw.ConfigType)
		fmt.Println()
	}

	// 2. CLEAR FUNCTION SIGNATURES - No more guessing!
	fmt.Println("2. Clear Configuration Examples:")

	// Cache middleware - the RIGHT way (from feedback)
	cache := middleware.NewMemoryCache(100)
	cacheConfig := middleware.CacheConfig{
		Cache: cache,
		TTL:   5 * time.Minute,
	}
	fmt.Printf("  CacheMiddleware: middleware.CacheConfig{Cache: cache, TTL: 5*time.Minute}\n")

	// Per-provider retry configuration - NEW improved pattern!
	maxRetries := 5
	retryDelay := 2 * time.Second
	maxRetryDelay := 30 * time.Second
	fmt.Printf("  Per-Provider Retry: types.ProviderConfig{MaxRetries: &%d, RetryDelay: &%v}\n",
		maxRetries, retryDelay)
	fmt.Printf("  Benefits: Fine-grained control per provider, no middleware stack complexity\n")

	// 3. BETTER ERROR MESSAGES - Typed error demonstration
	fmt.Println("\n3. Better Error Messages:")

	client := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithOpenAI("invalid-key-demo", types.ProviderConfig{
			MaxRetries:    &maxRetries,
			RetryDelay:    &retryDelay,
			RetryMaxDelay: &maxRetryDelay,
		}), // This will fail but demonstrate per-provider retry
		wormhole.WithMiddleware(
			middleware.CacheMiddleware(cacheConfig),
		),
	)

	ctx := context.Background()

	// This will demonstrate better error handling
	_, err := client.Text().
		Model("gpt-4o-mini").
		Prompt("Test error handling").
		Generate(ctx)

	if err != nil {
		// Better error messages as suggested in feedback
		if wormholeErr, ok := types.AsWormholeError(err); ok {
			fmt.Printf("  ✓ Structured Error: %s\n", wormholeErr.Code)
			fmt.Printf("  ✓ Clear Message: %s\n", wormholeErr.Message)
			fmt.Printf("  ✓ Retryable: %t\n", wormholeErr.Retryable)
			if wormholeErr.Model != "" {
				fmt.Printf("  ✓ Model Context: %s\n", wormholeErr.Model)
			}
		} else {
			fmt.Printf("  Generic error: %v\n", err)
		}
	}

	// 4. MIDDLEWARE COMPOSITION - Production patterns
	fmt.Println("\n4. Production Middleware Stack:")

	productionRetries := 3
	productionRetryDelay := 500 * time.Millisecond
	
	productionClient := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithOpenAI("your-key-here", types.ProviderConfig{
			MaxRetries: &productionRetries,
			RetryDelay: &productionRetryDelay,
		}),
		// Professional middleware stack from feedback  
		wormhole.WithMiddleware(
			middleware.CircuitBreakerMiddleware(5, 30*time.Second),
			middleware.RateLimitMiddleware(100),
			middleware.CacheMiddleware(cacheConfig),
			middleware.TimeoutMiddleware(60*time.Second),
		),
	)

	fmt.Printf("  ✓ Per-provider retry with exponential backoff\n")
	fmt.Printf("  ✓ Circuit breaker (5 failures, 30s timeout)\n")
	fmt.Printf("  ✓ Rate limiting (100 req/sec)\n")
	fmt.Printf("  ✓ Response caching (5min TTL)\n")
	fmt.Printf("  ✓ Request timeout (60s)\n")

	// 5. WHAT MEESIX NEEDS - Template integration concept
	fmt.Println("\n5. Future: Template Integration (Concept)")
	fmt.Printf("  // What meesix needs but wormhole doesn't provide YET:\n")
	fmt.Printf("  // response, err := client.Text().\n")
	fmt.Printf("  //     Model(\"gpt-5\").\n")
	fmt.Printf("  //     TemplatePrompt(\"prompts/role.tpl\", templateData).\n")
	fmt.Printf("  //     Generate(ctx)\n")
	fmt.Printf("  //\n")
	fmt.Printf("  // This would require TemplateMiddleware - architecturally sound!\n")

	// 6. COST ESTIMATION - Show the capability
	fmt.Println("\n6. Cost Management:")
	if cost, err := types.EstimateModelCost("gpt-4o-mini", 1000, 500); err == nil {
		fmt.Printf("  ✓ Estimated cost for 1K input + 500 output: $%.4f\n", cost)
	}

	// Show model constraints handling
	if constraints, err := types.GetModelConstraints("gpt-5"); err == nil {
		fmt.Printf("  ✓ GPT-5 constraints: %+v\n", constraints)
	}

	fmt.Println("\n=== DX Improvements Demo Complete ===")
	fmt.Println("🎯 Key improvements:")
	fmt.Println("  ✅ Middleware discovery API")
	fmt.Println("  ✅ Clear function signatures in docs")
	fmt.Println("  ✅ Better error messages with context")
	fmt.Println("  ✅ Production-ready middleware examples")
	fmt.Println("  🔮 Future: Template engine integration")

	_ = productionClient // Suppress unused variable
}

// DemonstratePainPoints shows the BEFORE vs AFTER
func DemonstratePainPoints() {
	fmt.Println("=== Before vs After Comparison ===")

	fmt.Println("\n❌ BEFORE (Confusing):")
	fmt.Printf("  middleware.CacheMiddleware(cache, ttl) // Wrong - doesn't exist\n")
	fmt.Printf("  middleware.RetryConfig{} // How do I configure this?\n")
	fmt.Printf("  // Had to dive into source code to find DefaultRetryConfig()\n")

	fmt.Println("\n✅ AFTER (Clear):")
	fmt.Printf("  middleware.AvailableMiddleware() // Discover all options\n")
	fmt.Printf("  middleware.CacheMiddleware(middleware.CacheConfig{...}) // Clear signature\n")
	fmt.Printf("  middleware.DefaultRetryConfig() // Well-documented default\n")

	fmt.Println("\n📈 Impact:")
	fmt.Printf("  - No more source diving\n")
	fmt.Printf("  - Clear examples in GoDoc\n")
	fmt.Printf("  - Discovery API for all middleware\n")
	fmt.Printf("  - Better error messages with context\n")
}

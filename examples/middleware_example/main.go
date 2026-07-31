package main

import (
	"context"
	"log/slog"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/middleware"
)

func main() {
	cache := middleware.NewMemoryCache(100)
	defer func() { _ = cache.Close() }()
	client := wormhole.New(
		wormhole.WithOpenAI("your-api-key"),
		wormhole.WithProviderMiddleware(
			middleware.NewTypedRateLimitMiddleware(10),
			middleware.NewTypedCircuitBreakerMiddleware(5, 30*time.Second),
			middleware.NewTypedTimeoutMiddleware(30*time.Second),
			middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: time.Minute}),
			middleware.NewTypedLoggingMiddleware(middleware.DefaultLoggingConfig(slog.Default())),
		),
	)
	defer func() { _ = client.Close() }()
	_, _ = client.Text().Model("gpt-5.6").Prompt("Explain typed middleware.").Generate(context.Background())
}

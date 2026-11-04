package wormhole

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
	testing_pkg "github.com/garyblankenship/wormhole/v3/wormholetest"
)

// These benchmarks isolate the per-call cost of the two optional layers that
// each derive a key by JSON-marshaling the request: the typed response cache
// (middleware) and the idempotency cache (core). With both enabled, one call
// marshals the request twice for keys; hits additionally clone the stored
// response. A mock provider removes network and provider decode cost so the
// layer deltas dominate.

func newLayerBenchClient(cache, idem bool) *Wormhole {
	provider := testing_pkg.NewMockProvider("mock")
	provider.WithTextResponse(types.TextResponse{
		Text:  "ok",
		Usage: &types.Usage{TotalTokens: 4},
	})
	opts := []Option{
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(config types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		WithModelValidation(false),
		WithDiscovery(false),
	}
	if cache {
		opts = append(opts, WithProviderMiddleware(
			middleware.NewTypedCacheMiddleware(middleware.CacheConfig{
				Cache: middleware.NewMemoryCache(1024),
				TTL:   time.Minute,
			}),
		))
	}
	if idem {
		opts = append(opts, WithIdempotencyKey("bench", time.Minute))
	}
	return New(opts...)
}

func layerBenchMessages(tag string) []types.Message {
	const msgs = 40
	const textLen = 400
	text := strings.Repeat("layer composition benchmark message body ", textLen/40+1)
	messages := make([]types.Message, msgs)
	for i := range messages {
		messages[i] = types.NewUserMessage(fmt.Sprintf("%s-%d: %s", tag, i, text[:textLen]))
	}
	return messages
}

func layerBenchTools() []types.Tool {
	tools := make([]types.Tool, 6)
	for i := range tools {
		tools[i] = types.Tool{
			Type:        "function",
			Name:        fmt.Sprintf("bench_tool_%d", i),
			Description: "Layer composition benchmark tool definition.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		}
	}
	return tools
}

func runLayerBench(b *testing.B, cache, idem bool, distinct int) {
	client := newLayerBenchClient(cache, idem)
	tools := layerBenchTools()
	options := map[string]any{"temperature": 0.2, "user": "bench"}

	pool := make([][]types.Message, distinct)
	for i := range pool {
		pool[i] = layerBenchMessages(fmt.Sprintf("miss-%d", i))
	}
	fixed := layerBenchMessages("hit")

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgs := fixed
		if distinct > 1 {
			msgs = pool[i%distinct]
		}
		if _, err := client.Text().
			Model("gpt-test").
			Messages(msgs...).
			Tools(tools...).
			ProviderOptions(options).
			Generate(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLayerBaseline(b *testing.B) {
	runLayerBench(b, false, false, 1)
}

func BenchmarkLayerCacheHit(b *testing.B) {
	runLayerBench(b, true, false, 1)
}

func BenchmarkLayerCacheMiss(b *testing.B) {
	runLayerBench(b, true, false, 256)
}

func BenchmarkLayerIdempotencyHit(b *testing.B) {
	runLayerBench(b, false, true, 1)
}

func BenchmarkLayerBothHit(b *testing.B) {
	runLayerBench(b, true, true, 1)
}

func BenchmarkLayerBothMiss(b *testing.B) {
	runLayerBench(b, true, true, 256)
}

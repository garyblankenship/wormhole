package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// WithIdempotencyKey adds an idempotency key to prevent duplicate operations during retries.
// When provided, the SDK will simulate server-side deduplication by caching responses.
//
// Parameters:
//   - key: Unique identifier for the operation (e.g., UUID, request hash)
//   - ttl: Time-to-live for cached responses (default: 24 hours)
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithIdempotencyKey("req-123", 1*time.Hour),
//	)
func WithIdempotencyKey(key string, ttl ...time.Duration) Option {
	return func(c *Config) {
		if c.Idempotency == nil {
			c.Idempotency = &IdempotencyConfig{}
		}
		c.Idempotency.Key = key
		if len(ttl) > 0 {
			c.Idempotency.TTL = ttl[0]
		}
	}
}

// WithModels populates the opt-in model registry with the given models.
//
// The global model registry (types.DefaultModelRegistry) starts empty. When
// model validation is enabled (the default), validation helpers have nothing
// to check against until the registry is populated. WithModels loads the
// provided models into the registry at New() time, making the opt-in explicit.
//
// Example:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithModels([]*types.ModelInfo{
//	        {ID: "my-model", Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityChat}},
//	    }),
//	)
func WithModels(models ...*types.ModelInfo) Option {
	return func(c *Config) {
		c.Models = append(c.Models, models...)
	}
}

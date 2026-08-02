# Migrating to v3

Wormhole v3 is a lean provider bridge. Update imports to
`github.com/garyblankenship/wormhole/v3` and use typed provider middleware.

## Import paths

### Migrating from v2

| v2 import | v3 import |
| --- | --- |
| `github.com/garyblankenship/wormhole/v2` | `github.com/garyblankenship/wormhole/v3` |
| `github.com/garyblankenship/wormhole/v2/config` | `github.com/garyblankenship/wormhole/v3/config` |
| `github.com/garyblankenship/wormhole/v2/discovery` | `github.com/garyblankenship/wormhole/v3/discovery` |
| `github.com/garyblankenship/wormhole/v2/discovery/fetchers` | `github.com/garyblankenship/wormhole/v3/discovery/fetchers` |
| `github.com/garyblankenship/wormhole/v2/middleware` | `github.com/garyblankenship/wormhole/v3/middleware` |
| `github.com/garyblankenship/wormhole/v2/providers` | `github.com/garyblankenship/wormhole/v3/providers` |
| `github.com/garyblankenship/wormhole/v2/providers/anthropic` | `github.com/garyblankenship/wormhole/v3/providers/anthropic` |
| `github.com/garyblankenship/wormhole/v2/providers/gemini` | `github.com/garyblankenship/wormhole/v3/providers/gemini` |
| `github.com/garyblankenship/wormhole/v2/providers/ollama` | `github.com/garyblankenship/wormhole/v3/providers/ollama` |
| `github.com/garyblankenship/wormhole/v2/providers/openai` | `github.com/garyblankenship/wormhole/v3/providers/openai` |
| `github.com/garyblankenship/wormhole/v2/types` | `github.com/garyblankenship/wormhole/v3/types` |
| `github.com/garyblankenship/wormhole/v2/wormholetest` | `github.com/garyblankenship/wormhole/v3/wormholetest` |

### Migrating from v1.24.0 (`pkg/` layout)

Legacy v1 installations nest packages under `pkg/`. In v3, `pkg/` is removed in favor of root-level subpackages:

```diff
// go.mod
- require github.com/garyblankenship/wormhole v1.24.0
+ require github.com/garyblankenship/wormhole/v3 v3.0.0
```

| v1.24.0 import | v3 import |
| --- | --- |
| `github.com/garyblankenship/wormhole/pkg/wormhole` | `github.com/garyblankenship/wormhole/v3` |
| `github.com/garyblankenship/wormhole/pkg/types` | `github.com/garyblankenship/wormhole/v3/types` |
| `github.com/garyblankenship/wormhole/pkg/config` | `github.com/garyblankenship/wormhole/v3/config` |
| `github.com/garyblankenship/wormhole/pkg/providers` | `github.com/garyblankenship/wormhole/v3/providers` |
| `github.com/garyblankenship/wormhole/pkg/providers/openai` | `github.com/garyblankenship/wormhole/v3/providers/openai` |
| `github.com/garyblankenship/wormhole/pkg/providers/gemini` | `github.com/garyblankenship/wormhole/v3/providers/gemini` |
| `github.com/garyblankenship/wormhole/pkg/providers/anthropic` | `github.com/garyblankenship/wormhole/v3/providers/anthropic` |

Example import migration:

```go
// Before (v1.24.0)
import (
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// After (v3)
import (
	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)
```

## Removed APIs and replacements

| v2 surface | v3 replacement |
| --- | --- |
| `Config.Middleware`, `WithMiddleware`, `middleware.Middleware`, `Handler`, `Chain`, `NewChain`, `Chain.Apply`, `Chain.Add`, `LegacyAdapter`, `NewLegacyAdapter`, and its `Apply*` methods | `Config.ProviderMiddlewares`, `WithProviderMiddleware`, and `types.ProviderMiddlewareChain` |
| `RateLimitMiddleware`, `CircuitBreakerMiddleware`, `CacheMiddleware` | `NewTypedRateLimitMiddleware`, `NewTypedCircuitBreakerMiddleware`, `NewTypedCacheMiddleware` |
| `LoggingMiddleware`, `DetailedLoggingMiddleware`, `DebugLoggingMiddleware` | `NewTypedLoggingMiddleware` or `NewDebugTypedLoggingMiddleware` |
| `TimeoutMiddleware` | `NewTypedTimeoutMiddleware` |
| `Metrics`, `NewMetrics`, `Metrics.RecordRequest`, `Metrics.GetStats`, `MetricsMiddleware`, and `EnhancedMetricsMiddleware` | `TypedMetrics`, `NewTypedMetricsMiddleware`, or `NewTypedEnhancedMetricsMiddleware` |
| `RetryConfig`, `DefaultRetryConfig`, `DefaultRetryableFunc`, `RetryMiddleware` | `types.ProviderConfig` retry fields or `WithRetries`; provider transports own retry classification |
| `LoadBalanceStrategy`; `RoundRobin`, `Random`, `LeastConnections`, `WeightedRoundRobin`, `ResponseTime`, and `Adaptive`; `ProviderHandler`; `ProviderStats`; `LoadBalancer`, `NewLoadBalancer`, `AddProvider`, `SelectProvider`, `Execute`, `StartHealthChecks`, `StopHealthChecks`, and `GetProviderStats`; `LoadBalancerMiddleware`; and `ErrNoHealthyProviders` | `TextRequestBuilder.WithFallback` and `WithProviderFallback` |
| `config.FallbackLoadBalancerHealthInterval` and `config.DefaultLoadBalancerHealthInterval` | Removed with the legacy load balancer |
| `AdaptiveRateLimiter`, `HealthMetrics`, `NewAdaptiveRateLimiter`, `NewHealthAwareAdaptiveRateLimiter`, `RecordLatency`, `RecordHealthMetrics`, `Close`, `AdaptiveRateLimitMiddleware`, and `HealthAwareAdaptiveRateLimitMiddleware` | Client adaptive concurrency configured through `EnableAdaptiveConcurrency` |
| `ProviderAwareLimiter`, `ProviderAwareConcurrencyLimitConfig`, and both `ProviderAwareConcurrencyLimitMiddleware` constructors | `EnhancedAdaptiveLimiter.AcquireToken` or `AcquireTokenWithProvider`; normal client requests use `EnableAdaptiveConcurrency` |
| `HealthCheckMiddleware` | Use `HealthChecker` directly for monitoring; route requests with builder fallbacks |
| `Wormhole.Provider` | `ProviderWithHandle`, then close the returned handle |
| `PIDConfig`, `DefaultPIDConfig`, `PIDController`, `NewPIDController`, `Compute`, `Reset`, and PID fields on adaptive/provider config | `TargetLatency`, `MinCapacity`, `MaxCapacity`, and `InitialCapacity` |
| `AdaptiveLimiter.Acquire`, `AdaptiveLimiter.Release`, `EnhancedAdaptiveLimiter.Acquire`, `AcquireWithProvider`, `Release`, `ReleaseWithProvider`, and `ProviderAdaptiveState.Limiter` | `AcquireToken` or `AcquireTokenWithProvider`; call the returned release closure |
| `ToolSafetyConfig.MaxMemoryMB`, `MaxCPUTime`, `EnableResourceIsolation`, `HasMemoryLimit`, and `HasCPULimit` | Removed; process isolation belongs to the host application |
| `BuildToolResultMessage` | `BuildToolResultMessages` |
| `discovery.JournalEntry` | Removed; discovery no longer writes append journals |
| `types.LegacyProvider` and the `LegacyTextProvider`, `LegacyStreamProvider`, `LegacyStructuredProvider`, `LegacyEmbeddingsProvider`, `LegacyAudioProvider`, and `LegacyImageProvider` interfaces | `types.Provider` |

Typed cache values are detached on both store and hit. Streams always bypass
cache storage. Circuit state is scoped to the provider identity and operation;
one typed rate limiter shares a cancellation-aware budget across operations.

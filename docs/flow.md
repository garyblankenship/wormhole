# Wormhole SDK - Architecture Flow

**Project**: Wormhole - Unified LLM SDK for Go
**Type**: Library (SDK) with CLI examples
**Stack**: Go 1.23, HTTP client, provider abstractions
**Entry points**: examples/wormhole-cli/main.go, pkg/wormhole/*.go
**Key files**: pkg/wormhole/*.go (core SDK), pkg/types/*.go (types), pkg/middleware/*.go (middleware), pkg/providers/*.go (provider implementations)
**Build**: `go build ./...`, `go test ./pkg/wormhole/... -short`
**External services**: Multiple AI providers (OpenAI, Anthropic, Gemini, OpenRouter, Ollama, LM Studio, Mistral, etc.)
**Concurrency model**: Goroutines, async processing, middleware chain

**Created**: 2026-01-14
**Audit mode**: flow (lean)

---

## 1. Mental Model

### What Wormhole Is
A **unified provider abstraction layer** that normalizes 10+ different AI provider APIs into a single Go interface. Think of it as a **database driver** for LLMs - you write once, it runs anywhere.

### Core Metaphor: Plug Adaptor
```go
// Traditional: Vendor-locked
openaiClient.ChatCompletion()  // OpenAI specific
anthropicClient.Messages()     // Anthropic specific

// Wormhole: Unified interface
wormhole.Text()               // Works with ANY provider
```

### Design Philosophy
1. **Provider Agnostic**: Switch providers without code changes
2. **Middleware First**: Observability via composable middleware
3. **Performance Aware**: Caching, batching, adaptive concurrency
4. **Developer Experience**: Strong types, clear errors, CLI examples

### Key Abstractions
- `types.Provider` - Core interface all providers implement
- `BaseProvider` - Default "not implemented" implementations
- `Wormhole` - Main client with caching, middleware, discovery
- `ProviderMiddlewareChain` - Type-safe middleware system

---

## 2. Architecture

### High-Level View
```
┌─────────────────────────────────────────────────────────┐
│                     Application                          │
│  (Your code using Wormhole)                              │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                    Wormhole Client                       │
│  • Provider selection & routing                          │
│  • Adaptive concurrency control                          │
│  • Caching (provider instances, responses)               │
│  • Middleware chain application                          │
└─────────────────────────────────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
                    ▼                   ▼
        ┌─────────────────┐     ┌─────────────────┐
        │   Provider      │     │   Provider      │
        │   OpenAI        │     │   Anthropic     │
        └─────────────────┘     └─────────────────┘
                    │                   │
                    ▼                   ▼
            ┌──────────────┐     ┌──────────────┐
            │  HTTP API    │     │  HTTP API    │
            │  OpenAI.com  │     │ Anthropic.com│
            └──────────────┘     └──────────────┘
```

### Core Components

**1. Types Layer (`pkg/types/`)**
- `Provider` interface - Unified 10+ method interface
- `BaseProvider` - Default implementations (return "not implemented")
- Request/response types - Strongly typed API contracts
- Error types - Provider-specific error wrapping

**2. Provider Implementations (`pkg/providers/`)**
- Each provider embeds `BaseProvider`
- Override only supported methods
- Handle provider-specific API formats
- Examples: OpenAI, Anthropic, Gemini, Ollama, OpenRouter

**3. Middleware System (`pkg/middleware/`)**
- Two systems: legacy generic `Chain`, type-safe `ProviderMiddlewareChain`
- Built-in middleware: metrics, logging, retry, circuit breaker, cache
- Context-aware labeling (provider, model, method)

**4. Core Client (`pkg/wormhole/`)**
- `Wormhole` struct - Main entry point
- Provider factory registry - Lazy instantiation
- Adaptive concurrency control - PID controller for rate limiting
- Idempotency cache - Avoid duplicate requests
- Tool registry - Function calling support

**5. Discovery Service (`pkg/discovery/`)**
- Dynamic model fetching from provider APIs
- Version management (GPT-5.2 vs GPT-5.1)
- Caching to avoid API rate limits

### Concurrency Patterns
- **Goroutine per request** with context propagation
- **Sync.Map** for idempotency cache (thread-safe)
- **Atomic counters** for metrics (lock-free)
- **WaitGroup** for graceful shutdown tracking
- **Adaptive limiter** - PID controller adjusts concurrency based on success rate

---

## 3. Data Flow

### Text Generation Request
```
1. Application calls wormhole.Text(request)
   │
2. Wormhole resolves provider → cachedProvider or factory.create()
   │   • Provider caching with ref counting & LRU eviction
   │
3. Apply middleware chain (metrics → logging → retry → circuit breaker)
   │   • Each middleware wraps and calls next()
   │
4. Provider implementation translates to provider-specific API
   │   • OpenAI: POST /v1/chat/completions
   │   • Anthropic: POST /v1/messages
   │   • Gemini: POST /v1/models/gemini-2.0:generateContent
   │
5. HTTP client executes request with timeout context
   │
6. Response parsed into unified TextResponse
   │
7. Return to application
```

### Streaming Flow
```
1. wormhole.Stream(request) returns <-chan TextChunk
   │
2. Provider creates goroutine that:
   │   • Opens SSE connection to provider
   │   • Parses streaming JSON chunks
   │   • Sends chunks to channel
   │
3. Application consumes from channel
   │
4. Goroutine cleans up on completion/error
```

### Middleware Chain Execution
```
MetricsMiddleware (start timer)
   │
LoggingMiddleware (log request)
   │
RetryMiddleware (attempt N times)
   │
CircuitBreakerMiddleware (check health)
   │
CacheMiddleware (check cache)
   │
Actual Provider Method
   │
MetricsMiddleware (record duration, error)
   │
Return to caller
```

### Adaptive Concurrency Flow
```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Success    │────▶│   PID       │────▶│  Adjust     │
│  Rate       │     │ Controller  │     │  Concurrency│
└─────────────┘     └─────────────┘     └─────────────┘
       ▲                    │                    │
       │                    ▼                    ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Monitor    │◀────│  Requests   │◀────│  Execute    │
│  Outcomes   │     │  In Flight  │     │  Requests   │
└─────────────┘     └─────────────┘     └─────────────┘
```

---

## 4. Failure Modes & Recovery

### Provider-Specific Failures
| Failure | Detection | Recovery |
|---------|-----------|----------|
| **Invalid API key** | `validateAPIKey()` format check | Return error immediately |
| **Provider API down** | HTTP 5xx, timeout | Circuit breaker → fail fast |
| **Rate limiting** | HTTP 429, provider headers | Retry with exponential backoff |
| **Model not found** | HTTP 404 | Discovery service updates cache |
| **Network partition** | Context timeout | Fail request, log metric |

### SDK Internal Failures
| Failure | Detection | Recovery |
|---------|-----------|----------|
| **Provider cache leak** | Ref counting mismatch | LRU eviction after timeout |
| **Middleware chain error** | Error wrapping | Propagate with context |
| **Adaptive limiter drift** | Success rate monitoring | PID controller reset |
| **Tool execution deadlock** | Timeout context | Cancel goroutine, clean up |
| **Memory leak in streaming** | Goroutine tracking | WaitGroup ensures cleanup |

### Error Propagation Pattern
```go
// All errors flow through wrapIfNotWormholeError()
err = provider.Text(ctx, req)
if err != nil {
    // Wrap with context if not already a WormholeError
    return nil, wrapIfNotWormholeError("provider", "text", err)
}
```

### Graceful Degradation
1. **Circuit breaker**: After N failures, stop trying provider
2. **Retry with backoff**: For transient network errors
3. **Fallback providers**: Configurable provider fallback chain
4. **Cache responses**: Idempotent requests return cached results
5. **Timeout propagation**: Context deadlines respected at all levels

### Monitoring Points
- **Metrics middleware**: Latency, error rates per provider/model
- **Enhanced metrics**: Labels (provider, model, method, error type)
- **Cache metrics**: Hits/misses/evictions
- **Adaptive limiter**: Current concurrency, success rate
- **Discovery service**: Model freshness, fetch failures

---

## Key Insights

### Strengths
1. **Clean abstraction**: Single interface for 10+ providers
2. **Observability built-in**: Middleware provides metrics out of box
3. **Performance aware**: Caching, batching, adaptive concurrency
4. **Strong typing**: Compile-time safety for API contracts
5. **Gradual adoption**: `BaseProvider` makes implementing new providers easy

### Areas for Attention
1. **Dual middleware systems**: Legacy `Chain` vs type-safe `ProviderMiddlewareChain`
2. **Provider sprawl**: 10+ implementations to maintain
3. **Model discovery complexity**: Keeping up with rapid provider model releases
4. **Streaming resource management**: Goroutine lifecycle needs careful tracking

### Evolution Trajectory
The SDK shows **mature library patterns**:
- Started with basic provider interface
- Added middleware for observability
- Introduced performance optimizations (caching, concurrency)
- Now evolving toward **type-safe middleware** and **dynamic discovery**

The architecture supports **both simple use** (text generation) and **advanced scenarios** (tool calling, streaming, batching, metrics).

---

## Flow Summary

**Wormhole normalizes chaos**: It takes 10+ different AI provider APIs with varying endpoints, authentication, error formats, and capabilities, and presents a **single, consistent Go interface**.

The flow is: **Request → Provider resolution → Middleware chain → Provider-specific translation → HTTP call → Unified response**.

Failures are **detected, categorized, and recovered** at appropriate levels: validation (immediate), network (retry), provider (circuit breaker), system (graceful degradation).

The result is **LLM infrastructure as code** - reliable, observable, and provider-agnostic.

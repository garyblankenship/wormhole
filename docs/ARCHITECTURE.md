# Wormhole SDK Architecture Audit

**Date**: 2026-01-14
**Version**: 1.x (main branch)
**Coverage**: 52.6% (wormhole core), 47.8% overall
**Files**: 166 Go sources, 15,047 lines of tests

---

## Executive Summary

Wormhole is a high-performance, production-grade Go SDK providing unified access to multiple LLM providers (OpenAI, Anthropic, Gemini, Ollama, OpenRouter). The architecture achieves **67ns request overhead** through functional options, object pooling, and zero-allocation patterns, with comprehensive middleware support for production resilience.

**Key Architectural Patterns**:
- Unified `Provider` interface with capability-based delegation
- Functional options + builder pattern for configuration
- Type-safe middleware chain with backward compatibility
- Object pooling for zero-allocation hot paths
- Graceful shutdown with request draining

---

## System Architecture

### High-Level Layer Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     CLIENT API LAYER                         │
│  Wormhole → Builder Pattern → Request Execution             │
│  (wormhole.go, *_builder.go)                                │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                  MIDDLEWARE LAYER                            │
│  Type-Safe Chain → Retry → Circuit Breaker → Rate Limit     │
│  (pkg/middleware/)                                           │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                  PROVIDER LAYER                              │
│  Unified Interface → Transform → HTTP Client                 │
│  (openai/, anthropic/, gemini/, ollama/)                     │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                  INFRASTRUCTURE LAYER                        │
│  Object Pools → SSE Parser → Error Handling                 │
│  (internal/pool/, internal/utils/)                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Components

### 1. Wormhole Client (`pkg/wormhole/wormhole.go`)

**Responsibilities**:
- Provider factory registration and lazy initialization
- Provider instance caching with LRU eviction
- Model discovery service coordination
- Tool registry management
- Graceful shutdown with zero-downtime draining

**State Management**:
```go
type Wormhole struct {
    providerFactories  map[string]types.ProviderFactory  // Factory per provider
    providers          map[string]*cachedProvider        // Cached instances (thread-safe)
    providersMutex     sync.RWMutex                      // Concurrent access control
    toolRegistry       *ToolRegistry                     // Global tool definitions
    discoveryService   *discovery.DiscoveryService       // Dynamic model catalog
    adaptiveLimiter    *EnhancedAdaptiveLimiter         // Concurrency control
    shutdownChan       chan struct{}                     // Graceful shutdown signal
    activeRequests     sync.WaitGroup                    // In-flight request tracking
    providerMiddleware *types.ProviderMiddlewareChain   // Type-safe middleware
}
```

**Caching Strategy**:
- **Double-checked locking** for provider creation (RLock → check → Lock → check)
- **Atomic reference counting** (`atomic.Int32`) for provider lifecycle tracking
- **Atomic last-used timestamps** (`atomic.Int64`) for LRU eviction
- **Configurable eviction** (max age + max count limits)

**Performance**:
- Cache hit: **RLock only** (~5ns)
- Cache miss: **RLock → Lock → create → cache** (~67ns total)
- Zero allocations in hot path

---

### 2. Provider Interface (`pkg/types/provider.go`)

**Unified Interface**:
```go
type Provider interface {
    io.Closer
    Name() string
    SupportedCapabilities() []ModelCapability

    // Core methods (all providers must implement)
    Text(ctx context.Context, request TextRequest) (*TextResponse, error)
    Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
    Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
    Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
    Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error)
    Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error)
    SpeechToText, TextToSpeech, GenerateImage...
}
```

**BaseProvider Pattern**:
- Providers embed `*providers.BaseProvider` for default implementations
- Override only supported methods (e.g., OpenAI overrides all, Ollama overrides Text/Stream/Embeddings)
- Unsupported methods return `ErrorCodeProvider` with clear message

**Provider Implementations**:

| Provider | Location | Capabilities | Auth Pattern |
|----------|----------|--------------|--------------|
| **OpenAI** | `pkg/providers/openai/` | Text, Stream, Structured, Embeddings, Audio, Images | Bearer token |
| **Anthropic** | `pkg/providers/anthropic/` | Text, Stream, Structured, Vision | `x-api-key` header |
| **Gemini** | `pkg/providers/gemini/` | Text, Stream, Structured, Embeddings | API key in URL |
| **Ollama** | `pkg/providers/ollama/` | Text, Stream, Embeddings | None (local) |

**Base Provider Architecture** (`pkg/providers/base.go`):
- HTTP client pooling per provider
- Retry logic with exponential backoff
- Error normalization to `WormholeError`
- Request/response sanitization (API key masking)

---

### 3. Builder Pattern (`pkg/wormhole/*_builder.go`)

**Builder Hierarchy**:
```
CommonBuilder (shared config, provider override, validation)
    ├── TextRequestBuilder      → Text generation, streaming, tool calling
    ├── StructuredRequestBuilder → JSON schemas, typed output
    ├── EmbeddingsRequestBuilder → Vector generation, semantic search
    ├── ImageRequestBuilder      → Image generation
    ├── AudioRequestBuilder      → TTS, STT
    └── BatchBuilder             → Concurrent multi-request execution
```

**Shared Configuration (CommonBuilder)**:
- Provider override (`.Using("anthropic")`)
- Base URL override (`.BaseURL("http://localhost:11434/v1")`)
- Validation helpers (`.Validate()`)
- Clone support (`.Clone()` for builder reuse)

**Advanced Features**:
- **Fallback Models**: `.WithFallback("gpt-4o-mini")` auto-retries on primary failure
- **Tool Auto-Execution**: `.WithAutoExecution(10)` runs multi-turn tool calling loop
- **Stream & Accumulate**: `.StreamAndAccumulate(ctx)` returns channel + final text getter
- **Conversation Builder**: `.Conversation(conv)` for multi-turn chats

**Memory Management**:
- Request/response objects pooled via `get*Request()` / `put*Request()` helpers
- Zero-allocation for request construction (value semantics)

---

### 4. Middleware System

**Type-Safe Architecture** (v1.1+):
```go
type ProviderMiddleware interface {
    Name() string
    Wrap(next ProviderHandler) ProviderHandler
}

type ProviderHandler func(ctx context.Context, req ProviderRequest) (ProviderResponse, error)
```

**Built-in Middleware** (`pkg/middleware/`):

| Middleware | Purpose | Key Features |
|------------|---------|--------------|
| **Retry** | Exponential backoff for transient failures | Respects `Retry-After`, max delay cap, jitter |
| **Circuit Breaker** | Fail-fast on provider outages | Half-open testing, configurable threshold |
| **Rate Limiter** | Token bucket rate limiting | Per-second limits, burst capacity |
| **Load Balancer** | Distribute across providers | Round-robin, weighted, health-aware |
| **Logging** | Structured request/response logging | Typed logging, debug mode |
| **Metrics** | Prometheus-compatible metrics | Latency histograms, token usage, error rates |
| **Health Check** | Provider availability probing | Periodic health pings, failure tracking |
| **Timeout** | Per-request timeouts | Context-based cancellation |

**Execution Order** (applied in reverse during setup):
```
Request → Logging → Metrics → Timeout → RateLimit → CircuitBreaker → Retry → Provider
```

**Backward Compatibility**:
- Legacy `middleware.Middleware` (reflection-based) auto-converted to type-safe via adapter
- Deprecated in v1.1+, will be removed in v2.0

---

### 5. Streaming Architecture (`internal/utils/streaming.go`)

**SSE Parsing Pipeline**:
```
HTTP Response Body → SSEParser → Provider-Specific Transform → TextChunk channel
```

**Components**:
- **SSEParser**: Stateful parser for `event:`, `data:`, `id:` fields
- **StreamProcessor**: Transforms SSE events into `types.TextChunk`
- **ProcessStream**: Goroutine wrapper with automatic `io.ReadCloser` cleanup

**Provider-Specific Transformers**:
- **OpenAI**: `transform.NewOpenAIStreamingTransformer()` (delta-based chunks)
- **Anthropic**: `transform.NewAnthropicStreamingTransformer()` (event types: `message_start`, `content_block_delta`, `message_stop`)
- **Gemini**: Custom JSON array streaming (not SSE-based)

**Memory Safety**:
- **Pooled line buffers** (`lineBufferPool`) for zero-allocation parsing
- **Buffered channels** (default: 100 chunks) prevent goroutine blocking
- **Automatic body close** on stream completion/error
- **Defer-based cleanup** ensures no leaks

**Error Handling**:
- Stream errors sent as `TextChunk{Error: err}`
- Client drains channel to completion
- Context cancellation propagates immediately

---

### 6. Request/Response Transformation (`pkg/providers/transform/`)

**Transform Layers**:

1. **Request Transform** (SDK → Provider):
   - **OpenAI**: Direct mapping (native format)
   - **Anthropic**: Messages API format, separate `system` field
   - **Gemini**: `content.parts[]` array, role mapping (`user`/`model`)

2. **Response Transform** (Provider → SDK):
   - Unified `TextResponse` structure
   - Normalized `ToolCall` format (function name, arguments)
   - Consistent `Usage` token counting

3. **Streaming Transform**:
   - SSE event → `TextChunk` delta
   - Incremental tool call assembly (delta aggregation)
   - Finish reason detection (`stop`, `length`, `tool_calls`)

**Common Transform Utilities** (`pkg/providers/transform/common.go`):
- `NormalizeToolCalls()`: Standardize tool call format across providers
- `ExtractTextContent()`: Pull text from various response structures
- `MergeUsage()`: Aggregate token counts across chunks

---

### 7. Error Handling (`pkg/types/errors.go`)

**Error Hierarchy**:
```go
*WormholeError (base)
    ├── ErrorCodeAuth       → Invalid/missing API key
    ├── ErrorCodeModel      → Model not found/supported
    ├── ErrorCodeRateLimit  → Quota/rate limit exceeded
    ├── ErrorCodeRequest    → Invalid params, payload too large
    ├── ErrorCodeTimeout    → Request timeout
    ├── ErrorCodeProvider   → Provider config error
    ├── ErrorCodeNetwork    → Connection failed, service unavailable
    ├── ErrorCodeValidation → Field validation failure
    └── ErrorCodeMiddleware → Circuit open, no healthy providers
```

**Context Enrichment**:
```go
return types.ErrModelNotFound.
    WithProvider("openai").
    WithModel("gpt-5").
    WithStatusCode(404).
    WithDetails("model registry lookup failed").
    WithOperation("TextRequestBuilder.Generate")
```

**Retry Decision Logic**:
```go
if types.IsRetryableError(err) {
    delay := types.GetRetryAfter(err)  // Smart backoff: 30s rate limit, 5s network
    time.Sleep(delay)
    // retry...
}
```

**HTTP Status Mapping** (`HTTPStatusToError`):
- `401` → `ErrInvalidAPIKey`
- `429` → `ErrRateLimited` (retryable)
- `503` → `ErrServiceUnavailable` (retryable)
- `400`, `422` → `ErrInvalidRequest` (non-retryable)

**Specialized Error Types**:
- `ModelConstraintError`: Model-specific parameter violations (e.g., GPT-5 temperature=1.0 only)
- `ValidationError`: Field-level validation with constraint details
- `ValidationErrors`: Multi-field batch validation

---

### 8. Model Discovery (`pkg/discovery/`)

**Architecture**:
```
DiscoveryService
    ├── Fetchers (per provider)
    │   ├── OpenAI: GET /v1/models
    │   ├── Anthropic: Hardcoded list (no API)
    │   ├── Ollama: GET /api/tags
    │   └── OpenRouter: GET /api/v1/models
    ├── Cache (in-memory, TTL-based)
    └── Background Refresh (goroutine)
```

**Configuration**:
```go
type DiscoveryConfig struct {
    RefreshInterval  time.Duration  // 0 = disabled
    CacheTTL         time.Duration  // Cache expiration
    OfflineMode      bool           // Skip fetches, use local only
    CustomFetchers   []ModelFetcher // User-provided fetchers
}
```

**API**:
```go
// Fetch models for a provider
models, _ := client.ListAvailableModels("openai")

// Force refresh all providers
client.RefreshModels()

// Clear cache
client.ClearModelCache()
```

**Caching Strategy**:
- First call: Fetch from API
- Subsequent calls: Serve from cache (TTL-based)
- Background refresh: Optional periodic updates (default: disabled)

---

### 9. Tool Calling System

**Components**:

1. **ToolRegistry** (`tool_registry.go`):
   - Global tool definition storage
   - Thread-safe with `sync.RWMutex`
   - Tool lookup by name

2. **Type-Safe Registration** (`tool_typed.go`):
   - Reflection-based schema generation from Go structs
   - Zero boilerplate for tool handlers
   - Compile-time type safety

3. **Tool Executor** (`tool_executor.go`):
   - Multi-turn conversation loop
   - Automatic tool invocation
   - Safety limits (max iterations, timeout)

**Example**:
```go
// Type-safe tool registration
type WeatherArgs struct {
    City string `json:"city" tool:"required"`
    Unit string `json:"unit" tool:"enum=celsius,fahrenheit"`
}

wormhole.RegisterTypedTool(client, "get_weather", "Get weather",
    func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
        return getWeather(args.City, args.Unit), nil
    },
)

// Auto-execute tools in conversation
resp, _ := client.Text().
    Model("gpt-4o").
    Prompt("What's the weather in NYC?").
    WithAutoExecution(10).  // Max 10 tool call rounds
    Generate(ctx)
```

**Safety Features**:
- Max iteration limit (prevent infinite loops)
- Timeout per tool call
- Error propagation to model for recovery

---

### 10. Object Pooling (`internal/pool/`)

**Pooled Resources**:
- **JSON Buffers** (`json.go`): Marshal/Unmarshal with pooled `[]byte`, 4KB initial allocation
- **Line Buffers** (streaming): 1KB buffers for SSE line parsing

**API**:
```go
// Pooled JSON marshaling
jsonBytes, err := pool.Marshal(data)
defer pool.Return(jsonBytes)  // Return to pool

// Pooled unmarshaling
err := pool.Unmarshal(jsonBytes, &target)
```

**Benchmark Impact**:
- Core request: **0 allocs/op** (down from 2 allocs/op)
- Streaming: **2 allocs/chunk** (down from 15 allocs/chunk)

---

## Data Flow

### Text Generation Request

```
1. Client Call
   client.Text().Model("gpt-4o").Prompt("Hello").Generate(ctx)

2. Builder Validation
   TextRequestBuilder.Validate()
   → Check model, messages, required fields

3. Provider Resolution
   Wormhole.getProvider("openai")
   → Cache hit? Return cached instance (atomic refCount++)
   → Cache miss? Create via factory, cache, return

4. Middleware Chain
   Request → Logging → Metrics → RateLimit → CircuitBreaker → Retry

5. Provider Request Transform
   SDK TextRequest → OpenAI chat/completions payload

6. HTTP Execution
   BaseProvider.DoRequest()
   → Build HTTP request with headers, auth
   → Execute via pooled HTTP client
   → Parse status → WormholeError if non-2xx

7. Response Transform
   OpenAI response JSON → SDK TextResponse

8. Middleware Response (reverse order)
   Provider → Retry → CircuitBreaker → RateLimit → Metrics → Logging

9. Return to Client
   *types.TextResponse
```

### Streaming Request

```
1. Client Call
   stream, _ := client.Text().Model("gpt-4o").Prompt("Hello").Stream(ctx)

2. Provider Streaming Request
   POST /chat/completions with stream=true

3. SSE Parsing (goroutine)
   HTTP body → SSEParser → transformToTextChunk(event) → channel

4. Client Consumption
   for chunk := range stream {
       if chunk.HasError() { return chunk.Error }
       fmt.Print(chunk.Content())
   }

5. Cleanup
   defer body.Close() (automatic in ProcessStream goroutine)
```

---

## Concurrency & Thread Safety

### Thread-Safe Components

| Component | Mechanism | Access Pattern |
|-----------|-----------|----------------|
| **Provider cache** | `sync.RWMutex` | Read-heavy (cache hits), write-rare (creation) |
| **Tool registry** | `sync.RWMutex` | Read-heavy (lookup), write-rare (registration) |
| **Cached provider state** | `atomic.Int64`, `atomic.Int32` | Atomic ref counting, last-used timestamps |
| **Idempotency cache** | `sync.Map` | Concurrent read/write for duplicate detection |
| **Shutdown state** | `atomic.Bool`, `sync.WaitGroup` | Graceful shutdown coordination |

### Graceful Shutdown

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := client.Shutdown(ctx); err != nil { log.Fatal(err) }
```

**Shutdown Sequence**:
1. Set `shuttingDown` atomic flag (reject new requests)
2. Close `shutdownChan` (signal background goroutines)
3. Wait for `activeRequests` WaitGroup (drain in-flight, respects ctx timeout)
4. Close all cached providers (cleanup HTTP clients)
5. Stop discovery service (stop refresh goroutine)
6. Return errors if cleanup failed or timeout

---

## Performance Characteristics

### Benchmark Results

```
BenchmarkTextGeneration-16     12566146    67.0 ns/op    0 B/op    0 allocs/op
BenchmarkWithMiddleware-16      5837629   171.5 ns/op    0 B/op    0 allocs/op
BenchmarkConcurrent-16          6826171   146.4 ns/op    0 B/op    0 allocs/op
```

### Optimization Techniques

1. **Object Pooling**: JSON buffers, line buffers → 0 allocs/op
2. **Provider Caching**: Lazy init, LRU eviction, atomic ref counting
3. **String Builder**: Streaming accumulation, error messages → 40% fewer allocs
4. **Buffered Channels**: 100-chunk buffer prevents goroutine blocking

---

## Security

**API Key Validation**:
- Format validation before use (OpenAI: `sk-`, Anthropic: `sk-ant-`)
- No logging in debug mode
- Sanitization in error messages (`sk-1****cdef`)

**TLS Configuration**:
- `ProviderConfig.WithInsecureTLS(bool)` for legacy compatibility
- Default: TLS 1.2+ with strict certificate validation

---

## Dependencies

**External** (from `go.mod`):
- `github.com/stretchr/testify`: Test assertions only

**Standard Library**:
- `net/http`, `context`, `encoding/json`, `sync`, `time`, `io`

**No runtime dependencies** beyond Go stdlib.

---

## Extension Points

**Custom Provider**:
```go
func myProviderFactory(config types.ProviderConfig) (types.Provider, error) {
    return &MyProvider{BaseProvider: providers.NewBaseProvider("my-provider", config)}, nil
}

client := wormhole.New(
    wormhole.WithCustomProvider("my-provider", myProviderFactory),
)
```

**Custom Middleware**:
```go
type MyMiddleware struct{}
func (m *MyMiddleware) Name() string { return "my-middleware" }
func (m *MyMiddleware) Wrap(next types.ProviderHandler) types.ProviderHandler {
    return func(ctx context.Context, req types.ProviderRequest) (types.ProviderResponse, error) {
        // Pre-processing
        resp, err := next(ctx, req)
        // Post-processing
        return resp, err
    }
}
```

---

**Last Updated**: 2026-01-14
**Audit Scope**: Full codebase (main branch)

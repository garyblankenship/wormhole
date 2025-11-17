# Architecture

**Project**: Wormhole - Unified Go SDK for LLM APIs

---

## Overview

Wormhole is a production-grade Go SDK providing a unified interface to multiple LLM providers (OpenAI, Anthropic, Gemini, OpenRouter, Groq, Mistral, Ollama). The architecture prioritizes performance (67ns core overhead), reliability (enterprise middleware), and developer experience (functional options pattern).

**Core Philosophy**: Provider-agnostic API with automatic constraint handling, zero-allocation hot paths, and thread-safe concurrent operations.

---

## Core Components

### 1. Provider Interface (`pkg/types/provider.go`)

**Responsibility**: Unified contract for all LLM providers

```go
type Provider interface {
    Name() string
    Text(ctx context.Context, request TextRequest) (*TextResponse, error)
    Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error)
    Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error)
    Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error)
    Images(ctx context.Context, request ImageRequest) (*ImageResponse, error)
    TTS(ctx context.Context, request TTSRequest) (*TTSResponse, error)
    STT(ctx context.Context, request STTRequest) (*STTResponse, error)
    Moderate(ctx context.Context, request ModerationRequest) (*ModerationResponse, error)
}
```

**Key Patterns**:
- **BaseProvider**: Default "not implemented" implementations for all methods
- **Method-Level Capability Discovery**: Providers return `NotImplementedError` for unsupported features
- **Composability**: Providers embed `BaseProvider` and override only supported methods

### 2. Client Architecture (`pkg/wormhole/`)

**Main Components**:

```
pkg/wormhole/
├── wormhole.go              # Client struct + provider management
├── options.go               # Functional options (WithOpenAI, WithAnthropic, etc.)
├── text_builder.go          # Fluent API for text generation
├── stream_builder.go        # Streaming request builder
├── structured_builder.go    # Structured output builder
├── embeddings_builder.go    # Vector embeddings builder
└── types.go                 # Request/response types
```

**Design Pattern**: Functional Options + Builder Pattern

```go
// Initialization (functional options)
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("sk-..."),
    wormhole.WithAnthropic("sk-ant-..."),
    wormhole.WithTimeout(30*time.Second),
)

// Request building (builder pattern)
response, err := client.Text().
    Model("gpt-5").
    Prompt("Generate text").
    Temperature(0.7).
    MaxTokens(100).
    Generate(ctx)
```

### 3. Provider Implementations (`pkg/providers/`)

**Structure**:
```
pkg/providers/
├── openai/          # OpenAI API (GPT models)
│   ├── openai.go    # Provider implementation
│   └── transform.go # Request/response transformations
├── anthropic/       # Anthropic API (Claude models)
├── gemini/          # Google Gemini API
└── ollama/          # Local Ollama server
```

**Key Mechanisms**:
- **BaseURL Approach**: One provider (OpenAI) handles all OpenAI-compatible APIs (Groq, Mistral, LM Studio, Ollama via `/v1/chat/completions`)
- **Transform Layer**: Converts Wormhole's unified types to provider-specific payloads
- **Error Normalization**: Maps provider-specific errors to `types.WormholeError`

### 4. Middleware System (`pkg/middleware/`)

**Composable Behavior Stack**:

```go
type Middleware func(types.Provider) types.Provider

// Built-in middleware
- CircuitBreakerMiddleware(failureThreshold, timeout)
- RateLimitMiddleware(requestsPerSecond)
- LoggingMiddleware(logger)
- CacheMiddleware(store, ttl)
- HealthMiddleware(healthChecker)
- LoadBalancerMiddleware(providers)
```

**Execution Flow**:
```
Request → Middleware Stack → Provider → Response
          ↓
     [Logging] → [Rate Limit] → [Circuit Breaker] → OpenAI Provider
```

**Thread Safety**: Each middleware uses sync primitives (RWMutex, atomic) for concurrent access.

### 5. Configuration System (`pkg/config/`)

**Centralized Defaults with Environment Overrides**:

```go
// defaults.go
func GetDefaultHTTPTimeout() time.Duration {
    if env := os.Getenv("WORMHOLE_DEFAULT_TIMEOUT"); env != "" {
        return parseDuration(env)
    }
    return 300 * time.Second  // Fallback
}

// Environment variables supported:
// - WORMHOLE_DEFAULT_TIMEOUT
// - WORMHOLE_MAX_RETRIES
// - WORMHOLE_INITIAL_RETRY_DELAY
// - WORMHOLE_MAX_RETRY_DELAY
```

**Per-Provider Configuration**:
```go
type ProviderConfig struct {
    APIKey        string
    BaseURL       string
    MaxRetries    *int           // Per-provider retry count
    RetryDelay    *time.Duration // Initial retry delay
    RetryMaxDelay *time.Duration // Max retry backoff
}
```

---

## Data Flow

### Text Generation Request Lifecycle

```
1. User Code
   └─> client.Text().Model("gpt-5").Prompt("...").Generate(ctx)

2. Builder Pattern (text_builder.go)
   └─> Constructs TextRequest with defaults
   └─> Applies model-specific constraints (e.g., GPT-5 temperature=1.0)

3. Provider Selection (wormhole.go)
   └─> getProviderWithBaseURL() resolves provider
   └─> Creates OpenAI provider with BaseURL if specified

4. Middleware Stack (middleware/)
   └─> Executes middleware chain (logging, rate limit, circuit breaker)

5. Provider Execution (providers/openai/)
   └─> Transform TextRequest → OpenAI payload
   └─> HTTP POST to /v1/chat/completions
   └─> Parse response → TextResponse

6. Response Handling
   └─> Cost calculation (usage.InputTokens * model.InputCost)
   └─> Return TextResponse to user
```

### Streaming Request Lifecycle

```
1. User Code
   └─> chunks, _ := client.Text().Stream(ctx)

2. Stream Builder (stream_builder.go)
   └─> Constructs streaming request

3. Provider Execution (providers/openai/)
   └─> HTTP POST with "stream": true
   └─> Server-Sent Events (SSE) parsing
   └─> Yield StreamChunk for each delta

4. Channel Communication
   └─> Provider sends StreamChunk to channel
   └─> User consumes: for chunk := range chunks { ... }
```

### Retry & Error Handling

**Per-Provider Retry Logic** (integrated at provider level):

```go
// Automatic retry for retryable errors:
// - 429 Too Many Requests (respects Retry-After header)
// - 500 Internal Server Error
// - 502 Bad Gateway
// - 503 Service Unavailable
// - 504 Gateway Timeout

// Exponential backoff:
// Retry 1: RetryDelay (default 500ms)
// Retry 2: RetryDelay * 2 (1s)
// Retry 3: RetryDelay * 4 (2s)
// Capped at RetryMaxDelay (default 30s)
```

**Non-Retryable Errors** (fail immediately):
- `400 Bad Request` - Malformed request
- `401 Unauthorized` - Invalid API key
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Model doesn't exist
- `422 Unprocessable Entity` - Validation errors

---

## Design Patterns

### 1. Functional Options Pattern (Initialization)

**Purpose**: Configure client without complex constructors

```go
type Option func(*Config)

func WithOpenAI(apiKey string, config ...types.ProviderConfig) Option {
    return func(c *Config) {
        // Configure OpenAI provider
    }
}

// Usage:
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("key"),
    wormhole.WithTimeout(30*time.Second),
)
```

**Benefits**:
- Optional parameters with defaults
- Clear, self-documenting API
- Backward compatible (new options don't break existing code)

### 2. Builder Pattern (Request Construction)

**Purpose**: Fluent API for complex requests

```go
response, _ := client.Text().
    Model("gpt-5").
    Prompt("Generate").
    Temperature(0.7).
    MaxTokens(100).
    Messages(
        types.NewSystemMessage("System context"),
        types.NewUserMessage("User query"),
    ).
    Generate(ctx)
```

**Benefits**:
- Type-safe request building
- Progressive disclosure (only specify what you need)
- Method chaining for readability

### 3. Factory Pattern (Provider Registration)

**Purpose**: Dynamic provider creation without hard-coding

```go
type ProviderFactory func(types.ProviderConfig) (types.Provider, error)

// Built-in factories
providerFactories["openai"] = func(c types.ProviderConfig) (types.Provider, error) {
    return openai.New(c), nil
}

// Dynamic registration
client.RegisterProvider("custom", NewCustomProvider)
```

**Benefits**:
- Extensibility (add custom providers)
- Thread-safe registration (sync.RWMutex)
- No core code changes for new providers

### 4. Middleware Chain Pattern (Cross-Cutting Concerns)

**Purpose**: Composable request/response processing

```go
type Middleware func(types.Provider) types.Provider

func LoggingMiddleware(logger Logger) Middleware {
    return func(next types.Provider) types.Provider {
        return &loggingProvider{next: next, logger: logger}
    }
}

// Composition
client := wormhole.New(
    wormhole.WithMiddleware(
        LoggingMiddleware(logger),
        RateLimitMiddleware(100),
        CircuitBreakerMiddleware(5, 30*time.Second),
    ),
)
```

**Benefits**:
- Separation of concerns (logging, caching, retries)
- Reusable across providers
- Testable in isolation

---

## Key Design Decisions

### Decision 1: BaseURL Approach for OpenAI-Compatible Providers

**Context**: Many providers (Groq, Mistral, Ollama, LM Studio) implement OpenAI's API format

**Decision**: Use single OpenAI provider with configurable BaseURL instead of separate provider packages

**Rationale**:
- Reduces code duplication (no separate transform layers)
- Instant support for new OpenAI-compatible providers
- Consistent API surface (one provider implementation to maintain)

**Consequences**:
- ✅ **Benefit**: Adding Groq/Mistral/etc. requires ZERO new code
- ✅ **Benefit**: One security audit surface (OpenAI transform logic)
- ❌ **Trade-off**: Provider-specific features require custom handling

**Example**:
```go
// All use the same OpenAI provider internally
client.Text().BaseURL("https://api.groq.com/openai/v1").Model("mixtral-8x7b")
client.Text().BaseURL("https://api.mistral.ai/v1").Model("mistral-large")
client.Text().BaseURL("http://localhost:11434/v1").Model("llama3.2")
```

### Decision 2: Per-Provider Retry Configuration vs Global Middleware

**Context**: Different providers have different reliability profiles

**Decision**: Implement retry logic at provider level with configurable per-provider settings

**Rationale**:
- OpenAI is usually stable (2-3 retries sufficient)
- Anthropic can be finicky (5+ retries may be needed)
- Local providers (Ollama) don't need network retries

**Consequences**:
- ✅ **Benefit**: Fine-grained control per provider
- ✅ **Benefit**: Respects Retry-After headers automatically
- ✅ **Benefit**: Transport-level logic (HTTP retries, not application retries)
- ❌ **Trade-off**: More complex provider implementations

**Example**:
```go
client := wormhole.New(
    wormhole.WithOpenAI("key", types.ProviderConfig{
        MaxRetries: &[]int{2}[0],  // Conservative
    }),
    wormhole.WithAnthropic("key", types.ProviderConfig{
        MaxRetries: &[]int{5}[0],  // Aggressive
    }),
)
```

### Decision 3: Automatic Model Constraint Handling

**Context**: GPT-5 models require `temperature=1.0` (per OpenAI constraints)

**Decision**: SDK automatically applies model-specific constraints

**Rationale**:
- Users shouldn't memorize model quirks
- Fail early with clear errors if user overrides are invalid
- Reduces support burden

**Consequences**:
- ✅ **Benefit**: GPT-5 works without manual temperature setting
- ✅ **Benefit**: Clear errors if user tries invalid settings
- ❌ **Trade-off**: Requires maintaining model constraint registry

**Implementation**:
```go
// types/model_registry.go
ModelConstraints: map[string]map[string]interface{}{
    "gpt-5":      {"temperature": 1.0},
    "gpt-5-mini": {"temperature": 1.0},
}
```

### Decision 4: Thread-Safe Concurrent Operations

**Context**: Early versions had race conditions in concurrent map access

**Decision**: Use sync.RWMutex for all provider registration and access

**Rationale**:
- Production deployments use goroutines for parallel requests
- Race detector caught concurrent map writes
- Double-checked locking pattern for performance

**Consequences**:
- ✅ **Benefit**: Safe concurrent provider registration and access
- ✅ **Benefit**: 146ns overhead for concurrent operations (tested)
- ❌ **Trade-off**: Slight lock contention under extreme concurrency

**Implementation**:
```go
type Wormhole struct {
    providers        map[string]types.Provider
    providerFactories map[string]ProviderFactory
    mu               sync.RWMutex  // Protects maps
}
```

---

## Database Schema

**Not applicable** - Wormhole is a stateless SDK with no database persistence.

---

## Integration Points

### External Services

| Service | Purpose | Protocol | Authentication |
|---------|---------|----------|----------------|
| **OpenAI** | GPT models | HTTPS REST | Bearer token (API key) |
| **Anthropic** | Claude models | HTTPS REST | `x-api-key` header |
| **Google Gemini** | Gemini models | HTTPS REST | API key query param |
| **OpenRouter** | 200+ models | HTTPS REST (OpenAI format) | Bearer token |
| **Groq** | Fast inference | HTTPS REST (OpenAI format) | Bearer token |
| **Mistral** | European AI | HTTPS REST (OpenAI format) | Bearer token |
| **Ollama** | Local models | HTTP REST (OpenAI format) | None (local) |

### Internal Communication

```
User Code (main.go)
    ↓
wormhole.Client (pkg/wormhole/)
    ↓
Middleware Stack (pkg/middleware/)
    ↓
Provider (pkg/providers/openai|anthropic|gemini)
    ↓
HTTP Client (net/http)
    ↓
External API (OpenAI, Anthropic, etc.)
```

---

## Performance Optimizations

### 1. Zero-Allocation Hot Path

**Goal**: 67ns per request (BenchmarkTextGeneration)

**Techniques**:
- Reuse HTTP clients (connection pooling)
- Minimize interface allocations
- Pre-allocate slices with known capacity
- Avoid string concatenation in hot paths

**Evidence** (from benchmarks):
```
BenchmarkTextGeneration-16    12566146    67 ns/op    0 B/op    0 allocs/op
```

### 2. Concurrent Request Handling

**Goal**: Linear scaling across goroutines

**Techniques**:
- sync.RWMutex for provider maps (read-heavy workload)
- Context-based cancellation (timeout handling)
- Thread-safe middleware implementations

**Evidence** (from benchmarks):
```
BenchmarkConcurrent-16        6826171   146.4 ns/op    0 B/op    0 allocs/op
```

### 3. Middleware Overhead Minimization

**Goal**: <200ns overhead for full middleware stack

**Techniques**:
- Lazy initialization of middleware components
- Bypass middleware for hot paths when possible
- Atomic operations for rate limiting

**Evidence** (from benchmarks):
```
BenchmarkWithMiddleware-16     5837629   171.5 ns/op    0 B/op    0 allocs/op
```

---

## Security Architecture

### API Key Management

- ✅ Environment variable support (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`)
- ✅ Automatic key masking in error messages (`sk-1****cdef`)
- ✅ No hardcoded secrets in code or tests

### Error Message Sanitization

**Before**:
```
Error: HTTP 401 at https://api.openai.com?key=sk-1234567890abcdef
```

**After**:
```
Error: HTTP 401 at https://api.openai.com?key=sk-1****cdef
```

### HTTPS-Only Policy

- All production providers use HTTPS
- Ollama (local) supports HTTP (localhost exception)

---

## Testing Strategy

### Unit Tests

- Mock provider (`pkg/testing/mock_provider.go`)
- Table-driven tests for transformations
- Edge case coverage (empty prompts, nil contexts)

### Integration Tests

- Real API calls (optional, via environment variables)
- Provider-specific behavior validation
- Streaming tests (SSE parsing)

### Benchmarks

```bash
make bench

# Output:
# BenchmarkTextGeneration-16     12566146    67 ns/op    0 B/op    0 allocs/op
# BenchmarkWithMiddleware-16      5837629   171.5 ns/op    0 B/op    0 allocs/op
# BenchmarkConcurrent-16          6826171   146.4 ns/op    0 B/op    0 allocs/op
```

---

## Deployment Considerations

### Production Setup

```go
// Recommended production configuration
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY"), types.ProviderConfig{
        MaxRetries:    &[]int{3}[0],
        RetryDelay:    &[]time.Duration{500 * time.Millisecond}[0],
        RetryMaxDelay: &[]time.Duration{30 * time.Second}[0],
    }),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY"), types.ProviderConfig{
        MaxRetries: &[]int{5}[0],
    }),
    wormhole.WithTimeout(2*time.Minute),
    wormhole.WithMiddleware(
        middleware.CircuitBreakerMiddleware(5, 30*time.Second),
        middleware.RateLimitMiddleware(100),
        middleware.LoggingMiddleware(logger),
    ),
)
```

### Environment Variables

```bash
# Required (at least one provider)
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENROUTER_API_KEY="sk-or-..."

# Optional (overrides defaults)
export WORMHOLE_DEFAULT_TIMEOUT="5m"
export WORMHOLE_MAX_RETRIES="3"
export WORMHOLE_INITIAL_RETRY_DELAY="500ms"
export WORMHOLE_MAX_RETRY_DELAY="30s"
```

---

## Future Architecture Considerations

### Potential Improvements (from roadmap.md)

1. **Multi-Modal Extensions** (images, audio)
2. **RAG Helpers** (embeddings + retrieval utilities)
3. **OpenTelemetry Integration** (distributed tracing)

See `docs/KNOWLEDGE.md` for detailed roadmap.

---

**Last Updated**: 2025-11-16
**Version**: Based on v1.4.0 codebase analysis

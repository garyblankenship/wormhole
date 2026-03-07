# Client Architecture

## Overview

The Wormhole client is the main entry point for interacting with LLM providers. It provides a unified interface for accessing multiple providers (OpenAI, Anthropic, Gemini, Ollama, OpenRouter) through a single, idiomatic Go API.

## Unified Client Interface

The client follows a **builder pattern** for request construction and **functional options** for client configuration:

```go
// Client creation with functional options
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
    wormhole.WithDefaultProvider("openai"),
)

// Request creation with builder pattern
resp, err := client.Text().
    Model("gpt-4o").
    Prompt("Explain quantum computing").
    Generate(ctx)
```

### Request Builders

The client provides typed builders for each request type:

| Builder | Purpose | Methods |
|---------|---------|---------|
| **`Text()`** | Text generation & streaming | `Model()`, `Prompt()`, `Messages()`, `Stream()`, `Generate()` |
| **`Structured()`** | Structured JSON output | `Model()`, `Prompt()`, `Schema()`, `ParseTo()` |
| **`Embeddings()`** | Vector embeddings | `Model()`, `Text()`, `Embed()` |
| **`Image()`** | Image generation | `Model()`, `Prompt()`, `Generate()` |
| **`Audio()`** | Audio (TTS/STT) | `Model()`, `Text()`, `Generate()` |
| **`Batch()`** | Concurrent execution | `Add()`, `Concurrency()`, `Execute()` |

### Provider Delegation

Request builders automatically delegate to the appropriate provider:

```go
// Uses default provider
resp, _ := client.Text().Model("gpt-4o").Prompt("Hello").Generate(ctx)

// Override provider for this request
resp, _ := client.Text().
    Using("anthropic").
    Model("claude-sonnet-4-5").
    Prompt("Hello").
    Generate(ctx)

// Custom base URL (e.g., local Ollama)
resp, _ := client.Text().
    BaseURL("http://localhost:11434/v1").
    Model("llama3.2").
    Prompt("Hello").
    Generate(ctx)
```

## Client Lifecycle

### 1. Creation

```go
client := wormhole.New(
    wormhole.WithOpenAI("sk-..."),
    wormhole.WithAnthropic("sk-ant-..."),
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithDebugLogging(true),
)
```

**Configuration Options:**

| Option | Purpose |
|--------|---------|
| `WithOpenAI(key)` | Configure OpenAI provider |
| `WithAnthropic(key)` | Configure Anthropic provider |
| `WithGemini(key)` | Configure Google Gemini provider |
| `WithOllama(baseURL)` | Configure local Ollama provider |
| `WithDefaultProvider(name)` | Set default provider for requests |
| `WithDebugLogging(bool)` | Enable debug logging |
| `WithProviderMiddleware(...)` | Add middleware chain |
| `WithCustomProvider(name, factory)` | Register custom provider |

### 2. Usage

```go
// Simple request
resp, err := client.Text().
    Model("gpt-4o").
    Prompt("Explain Go's concurrency model").
    Generate(ctx)

// Streaming
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Tell me a story").
    Stream(ctx)

for chunk := range stream {
    if chunk.HasError() {
        return chunk.Error
    }
    fmt.Print(chunk.Content())
}
```

### 3. Cleanup

```go
// Immediate close (for graceful shutdown, use Shutdown())
defer client.Close()

// Graceful shutdown (waits for in-flight requests)
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := client.Shutdown(ctx); err != nil {
    log.Printf("Shutdown error: %v", err)
}
```

**Shutdown Sequence:**
1. Reject new requests
2. Wait for in-flight requests (respects context timeout)
3. Close all provider connections
4. Stop background services (discovery, limiter)
5. Return cleanup errors (if any)

## Thread Safety

The client is **designed for concurrent use** with thread-safe components:

### Thread-Safe Components

| Component | Mechanism | Access Pattern |
|-----------|-----------|----------------|
| **Provider cache** | `sync.RWMutex` | Read-heavy (cache hits), write-rare (creation) |
| **Tool registry** | `sync.RWMutex` | Read-heavy (lookup), write-rare (registration) |
| **Cached provider state** | `atomic.Int64`, `atomic.Int32` | Atomic ref counting, last-used timestamps |
| **Idempotency cache** | `sync.Map` | Concurrent read/write for duplicate detection |
| **Shutdown state** | `atomic.Bool`, `sync.WaitGroup` | Graceful shutdown coordination |

### Concurrent Request Pattern

```go
// Safe: multiple goroutines using the same client
var wg sync.WaitGroup
for _, prompt := range prompts {
    wg.Add(1)
    go func(p string) {
        defer wg.Done()
        resp, _ := client.Text().Model("gpt-4o").Prompt(p).Generate(ctx)
        // Handle response...
    }(prompt)
}
wg.Wait()
```

### Provider Caching

Providers are cached with **double-checked locking** and **atomic reference counting**:

```go
// Pseudo-code of thread-safe provider access
func (p *Wormhole) Provider(name string) (types.Provider, error) {
    // 1. Fast path: read lock for cache hit
    p.providersMutex.RLock()
    if cached, exists := p.providers[name]; exists {
        atomic.AddInt32(&cached.refCount, 1)
        atomic.StoreInt64(&cached.lastUsed, time.Now().UnixNano())
        p.providersMutex.RUnlock()
        return cached.provider, nil  // ~5ns overhead
    }
    p.providersMutex.RUnlock()

    // 2. Slow path: write lock for creation
    p.providersMutex.Lock()
    defer p.providersMutex.Unlock()

    // 3. Double-check: another goroutine might have created it
    if cached, exists := p.providers[name]; exists {
        atomic.AddInt32(&cached.refCount, 1)
        return cached.provider, nil
    }

    // 4. Create and cache provider
    provider := p.providerFactories[name](config)
    p.providers[name] = &cachedProvider{
        provider: provider,
        refCount: 1,
        lastUsed: time.Now().UnixNano(),
    }
    return provider, nil  // ~67ns total overhead
}
```

## Provider Handles (Reference Counting)

For long-running operations, use **provider handles** for explicit lifecycle management:

```go
// Get provider with automatic reference counting
handle, err := client.ProviderWithHandle("openai")
if err != nil {
    return err
}
defer handle.Close()  // IMPORTANT: Always close the handle

// Use provider directly (bypasses middleware)
resp, err := handle.Text(ctx, request)
```

**Benefits:**
- Prevents premature provider eviction during cleanup
- Explicit resource management for long-lived operations
- Thread-safe reference counting via `atomic.Int32`

## Graceful Shutdown

The client supports **zero-downtime graceful shutdown** with request draining:

```go
// Shutdown with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := client.Shutdown(ctx); err != nil {
    log.Printf("Graceful shutdown failed: %v", err)
}
```

**Shutdown Behavior:**

| State | Action |
|-------|--------|
| **Pre-shutdown** | All requests accepted normally |
| **Shutdown initiated** | New requests rejected with error |
| **Draining phase** | In-flight requests complete (respects timeout) |
| **Cleanup phase** | Providers closed, services stopped |
| **Post-shutdown** | All resources released |

**Idempotency:** Multiple `Shutdown()` calls are safe (only first call has effect).

## Advanced Features

### Model Discovery

```go
// List available models for a provider
models, err := client.ListAvailableModels("openai")
for _, model := range models {
    fmt.Printf("%s: %v\n", model.Name, model.Capabilities)
}

// Force refresh model catalogs
client.RefreshModels()

// Clear cached model data
client.ClearModelCache()
```

### Tool Registration

```go
// Register tools for function calling
client.RegisterTool(
    "get_weather",
    "Get current weather for a city",
    types.ObjectSchema{
        Type: "object",
        Properties: map[string]types.Schema{
            "city": types.StringSchema{Type: "string"},
        },
        Required: []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        return map[string]any{"temp": 72, "condition": "sunny"}, nil
    },
)

// Use tools in requests
resp, _ := client.Text().
    Model("gpt-4o").
    Prompt("What's the weather in Tokyo?").
    WithAutoExecution(10).  // Auto-execute tool calls
    Generate(ctx)
```

### Cache Metrics

```go
// Inspect provider cache performance
metrics := client.GetCacheMetrics()
fmt.Printf("Hits: %d, Misses: %d, Evictions: %d, Size: %d\n",
    metrics.Hits, metrics.Misses, metrics.Evictions, metrics.Size,
)

// Cleanup stale providers
client.CleanupStaleProviders(1*time.Hour, 10)  // maxAge, maxCount
```

### Adaptive Concurrency

```go
// Enable adaptive concurrency control
client.EnableAdaptiveConcurrency(nil)  // Uses defaults

// Get runtime statistics
stats := client.GetAdaptiveConcurrencyStats()
fmt.Printf("Active: %d, Limited: %d, Rejected: %d\n",
    stats["active_concurrency"],
    stats["limited_count"],
    stats["rejected_count"],
)
```

## Error Handling

The client uses **structured error types** with context enrichment:

```go
resp, err := client.Text().Model("gpt-4o").Prompt("Hello").Generate(ctx)
if err != nil {
    var wErr *types.WormholeError
    if errors.As(err, &wErr) {
        fmt.Printf("Provider: %s, Model: %s, Code: %s\n",
            wErr.Provider, wErr.Model, wErr.Code)
    }
    return err
}
```

**Common Error Codes:**

| Code | Cause | Retryable |
|------|-------|-----------|
| `ErrorCodeAuth` | Invalid API key | No |
| `ErrorCodeModel` | Model not found | No |
| `ErrorCodeRateLimit` | Rate limit exceeded | Yes |
| `ErrorCodeTimeout` | Request timeout | Yes |
| `ErrorCodeNetwork` | Connection failed | Yes |

## Best Practices

1. **Always close the client** (or use graceful shutdown)
2. **Use context with timeout** for all requests
3. **Reuse client instances** across requests (provider caching)
4. **Enable debug logging** during development
5. **Check error types** for proper retry logic
6. **Use provider handles** for long-running operations
7. **Clean up stale providers** in long-lived applications

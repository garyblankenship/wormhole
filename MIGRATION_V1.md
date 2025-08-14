# Wormhole v1.0 Migration Guide

**Breaking Changes**: This is a major version with breaking changes. The old mutable API has been replaced with an immutable functional options pattern.

## Quick Migration Examples

### Basic Client Creation

**Before (v0.x)**:
```go
// Old mutable pattern
config := wormhole.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{
        "openai": {APIKey: "your-api-key"},
    },
}
client := wormhole.New(config)
```

**After (v1.0)**:
```go
// New functional options pattern
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("your-api-key"),
)
```

### Multiple Providers

**Before**:
```go
config := wormhole.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{
        "openai":    {APIKey: "openai-key"},
        "anthropic": {APIKey: "anthropic-key"},
        "gemini":    {APIKey: "gemini-key"},
    },
}
client := wormhole.New(config)
```

**After**:
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("openai-key"),
    wormhole.WithAnthropic("anthropic-key"),
    wormhole.WithGemini("gemini-key"),
)
```

### Post-Creation Configuration (REMOVED)

**Before (NO LONGER WORKS)**:
```go
client := wormhole.New(config)
client.Use(middleware.RetryMiddleware(3))          // ❌ REMOVED
client.WithGemini("key")                           // ❌ REMOVED  
client.RegisterProvider("custom", factory)        // ❌ REMOVED
```

**After (Declare Everything at Creation)**:
```go
client := wormhole.New(
    wormhole.WithOpenAI("openai-key"),
    wormhole.WithMiddleware(middleware.RetryMiddleware(3)),
    wormhole.WithCustomProvider("custom", factory),
)
// client is now immutable - no more mutations possible
```

## Complete Migration Reference

### 1. Provider Configuration

| Old Pattern | New Pattern |
|-------------|-------------|
| `Config{Providers: map[string]types.ProviderConfig{"openai": {APIKey: "key"}}}` | `WithOpenAI("key")` |
| `Config{Providers: map[string]types.ProviderConfig{"anthropic": {APIKey: "key"}}}` | `WithAnthropic("key")` |
| `Config{Providers: map[string]types.ProviderConfig{"gemini": {APIKey: "key"}}}` | `WithGemini("key")` |
| `Config{Providers: map[string]types.ProviderConfig{"groq": {APIKey: "key"}}}` | `WithGroq("key")` |
| `Config{Providers: map[string]types.ProviderConfig{"mistral": {APIKey: "key"}}}` | `WithMistral(types.ProviderConfig{APIKey: "key"})` |
| `Config{Providers: map[string]types.ProviderConfig{"ollama": {BaseURL: "url"}}}` | `WithOllama(types.ProviderConfig{BaseURL: "url"})` |

### 2. Custom Providers

**Before**:
```go
client := wormhole.New(config)
client.RegisterProvider("custom", func(cfg types.ProviderConfig) (types.Provider, error) {
    return myProvider, nil
})
```

**After**:
```go
client := wormhole.New(
    wormhole.WithCustomProvider("custom", func(cfg types.ProviderConfig) (types.Provider, error) {
        return myProvider, nil
    }),
    wormhole.WithProviderConfig("custom", types.ProviderConfig{APIKey: "key"}),
)
```

### 3. OpenAI-Compatible Providers

**Before**:
```go
client := wormhole.New(config)
client.WithOpenAICompatible("lmstudio", "http://localhost:1234", types.ProviderConfig{})
```

**After**:
```go
client := wormhole.New(
    wormhole.WithOpenAICompatible("lmstudio", "http://localhost:1234", types.ProviderConfig{}),
)
```

### 4. Middleware

**Before**:
```go
client := wormhole.New(config)
client.Use(middleware.RetryMiddleware(retryConfig))
client.Use(middleware.RateLimitMiddleware(10))
client.Use(middleware.TimeoutMiddleware(30*time.Second))
```

**After**:
```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithMiddleware(
        middleware.RetryMiddleware(retryConfig),
        middleware.RateLimitMiddleware(10),
        middleware.TimeoutMiddleware(30*time.Second),
    ),
)
```

### 5. Factory Helper Methods

**Before**:
```go
factory := wormhole.NewSimpleFactory()
client := factory.OpenAI("key")
client = factory.WithRetry(client, 3)
client = factory.WithRateLimit(client, 10)
```

**After**:
```go
factory := wormhole.NewSimpleFactory()
client := factory.OpenAI("key")
// Factory helpers now return Options, use during creation:
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    factory.WithRetry(3),        // Returns Option
    factory.WithRateLimit(10),   // Returns Option
)
```

### 6. Debug Logging

**Before**:
```go
config := wormhole.Config{
    DebugLogging: true,
    Logger: myLogger,
}
client := wormhole.New(config)
```

**After**:
```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithDebugLogging(myLogger),
)
```

### 7. Configuration Settings

**Before**:
```go
config := wormhole.Config{
    DefaultProvider: "openai",
    Middleware: []middleware.Middleware{...},
    DebugLogging: true,
}
```

**After**:
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithMiddleware(...),
    wormhole.WithDebugLogging(),
    wormhole.WithTimeout(30*time.Second),
    wormhole.WithRetries(3, time.Second),
)
```

## Common Migration Patterns

### Pattern 1: Simple Single Provider

```go
// Before
config := wormhole.Config{
    DefaultProvider: "openai", 
    Providers: map[string]types.ProviderConfig{
        "openai": {APIKey: os.Getenv("OPENAI_API_KEY")},
    },
}
client := wormhole.New(config)

// After  
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
)
```

### Pattern 2: Multi-Provider with Middleware

```go
// Before
config := wormhole.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{
        "openai":    {APIKey: "openai-key"},
        "anthropic": {APIKey: "anthropic-key"},
    },
    Middleware: []middleware.Middleware{
        middleware.RetryMiddleware(retryConfig),
    },
    DebugLogging: true,
}
client := wormhole.New(config)
client.Use(middleware.RateLimitMiddleware(10))

// After
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("openai-key"),
    wormhole.WithAnthropic("anthropic-key"), 
    wormhole.WithMiddleware(
        middleware.RetryMiddleware(retryConfig),
        middleware.RateLimitMiddleware(10),
    ),
    wormhole.WithDebugLogging(),
)
```

### Pattern 3: Custom Provider Registration

```go
// Before
client := wormhole.New(wormhole.Config{})
client.RegisterProvider("custom", func(cfg types.ProviderConfig) (types.Provider, error) {
    return &MyCustomProvider{config: cfg}, nil
})

// After
client := wormhole.New(
    wormhole.WithCustomProvider("custom", func(cfg types.ProviderConfig) (types.Provider, error) {
        return &MyCustomProvider{config: cfg}, nil
    }),
    wormhole.WithProviderConfig("custom", types.ProviderConfig{
        APIKey: "custom-key",
        BaseURL: "https://my-api.com",
    }),
)
```

## Why This Change?

### Problems with Old Mutable API
- **Thread Safety Issues**: Post-creation mutations caused race conditions
- **Configuration Drift**: Hard to know final client state after mutations
- **Mixed Patterns**: Constructor config + fluent mutations was confusing
- **Testing Difficulty**: Mutable state made testing complex

### Benefits of New Functional Options
- **Immutability**: Configuration cannot change after creation
- **Thread Safety**: No mutations = no race conditions
- **Declarative**: All configuration visible at creation point
- **Type Safety**: Compile-time validation of all options
- **Composability**: Options can be combined and reused

## Migration Checklist

- [ ] Replace `wormhole.New(config)` with `wormhole.New(options...)`
- [ ] Convert provider configurations to `WithProvider()` options
- [ ] Move middleware from `.Use()` calls to `WithMiddleware()` option
- [ ] Replace `.RegisterProvider()` with `WithCustomProvider()` option  
- [ ] Remove all post-creation `.With*()` method calls
- [ ] Update factory helper usage to get Options, not mutate clients
- [ ] Test thoroughly - the API is now immutable after creation

## Quick Search & Replace

Use these regex patterns to help with migration:

```bash
# Find old constructor calls
grep -r "New(.*Config{" .

# Find old Use() calls  
grep -r "\.Use(" .

# Find old With*() method calls
grep -r "\.With[A-Z]" .

# Find old RegisterProvider calls
grep -r "\.RegisterProvider(" .
```

## Need Help?

- Check the updated examples in `/examples` directory
- See the comprehensive test files for usage patterns
- The functional options pattern is well-documented in Go community resources
- All old functionality is available, just configured differently

**Remember**: This is a breaking change for a good reason - the new API is safer, cleaner, and more maintainable!
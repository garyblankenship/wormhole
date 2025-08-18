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
import (
    "os"
    
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

// New functional options pattern
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
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
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
    wormhole.WithGemini(os.Getenv("GEMINI_API_KEY")),
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

## Step-by-Step Migration Process

### Step 1: Update Dependencies
```bash
# Update to latest version
go get github.com/garyblankenship/wormhole@latest

# Clean module cache if needed
go clean -modcache
go mod tidy
```

### Step 2: Update Imports (if needed)
```go
// Usually no import changes needed, but verify:
import "github.com/garyblankenship/wormhole/pkg/wormhole"
```

### Step 3: Convert Client Creation
Replace old config-based creation with functional options:

```diff
- config := wormhole.Config{
-     DefaultProvider: "openai",
-     Providers: map[string]types.ProviderConfig{
-         "openai": {APIKey: os.Getenv("OPENAI_API_KEY")},
-     },
- }
- client := wormhole.New(config)

+ client := wormhole.New(
+     wormhole.WithDefaultProvider("openai"),
+     wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
+ )
```

### Step 4: Migrate Middleware
```diff
- client := wormhole.New(config)
- client.Use(middleware.RetryMiddleware(retryConfig))
- client.Use(middleware.RateLimitMiddleware(10))

+ client := wormhole.New(
+     wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
+     wormhole.WithMiddleware(
+         middleware.RetryMiddleware(retryConfig),
+         middleware.RateLimitMiddleware(10),
+     ),
+ )
```

### Step 5: Remove Post-Creation Mutations
```diff
- client := wormhole.New(config)
- client.WithAnthropic("key")           // ❌ NO LONGER EXISTS
- client.RegisterProvider("custom", fn) // ❌ NO LONGER EXISTS

+ client := wormhole.New(
+     wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
+     wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
+     wormhole.WithCustomProvider("custom", fn),
+ )
```

### Step 6: Test Your Migration
Create a simple test to verify your migration:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Test basic functionality after migration
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    )
    
    ctx := context.Background()
    response, err := client.Text().
        Model("gpt-4o-mini").
        Prompt("Hello, migration test!").
        MaxTokens(50).
        Generate(ctx)
    
    if err != nil {
        log.Printf("Migration test failed: %v", err)
        return
    }
    
    fmt.Printf("✅ Migration successful! Response: %s\n", response.Content)
}
```

## Migration Troubleshooting

### Build Errors After Migration

**Error**: `cannot use client.Use (undefined)`
```bash
client.Use undefined (type *wormhole.Wormhole has no field or method Use)
```

**Solution**: Move middleware to client creation:
```go
// ❌ Old way (doesn't work)
client.Use(middleware.RetryMiddleware(config))

// ✅ New way
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithMiddleware(middleware.RetryMiddleware(config)),
)
```

**Error**: `client.RegisterProvider undefined`
```bash
client.RegisterProvider undefined
```

**Solution**: Use functional options:
```go
// ❌ Old way
client.RegisterProvider("custom", factory)

// ✅ New way
client := wormhole.New(
    wormhole.WithCustomProvider("custom", factory),
    wormhole.WithProviderConfig("custom", types.ProviderConfig{...}),
)
```

### Runtime Errors After Migration

**Error**: `provider "openai" not found`

**Solutions**:
1. Ensure you're setting the default provider:
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"), // ← This is required
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
)
```

2. Or specify provider explicitly in requests:
```go
response, err := client.Text().
    Using("openai").
    Model("gpt-4o").
    Generate(ctx)
```

**Error**: Missing environment variables

**Solution**: Verify your environment variables are set:
```bash
# Check if variables are set
echo $OPENAI_API_KEY
echo $ANTHROPIC_API_KEY

# Set them if missing
export OPENAI_API_KEY="sk-your-key-here"
```

### Performance Issues After Migration

**Problem**: Slower response times

**Investigation**: Check if middleware ordering changed:
```go
// Middleware executes in LIFO order - order matters!
wormhole.WithMiddleware(
    middleware.TimeoutMiddleware(30*time.Second),   // Executes FIRST
    middleware.RetryMiddleware(config),             // Executes SECOND
    middleware.RateLimitMiddleware(10),             // Executes LAST
)
```

## Migration Checklist

- [ ] **Update dependencies**: `go get github.com/garyblankenship/wormhole@latest`
- [ ] **Replace `wormhole.New(config)`** with `wormhole.New(options...)`
- [ ] **Convert provider configurations** to `WithProvider()` options
- [ ] **Move middleware** from `.Use()` calls to `WithMiddleware()` option
- [ ] **Replace `.RegisterProvider()`** with `WithCustomProvider()` option  
- [ ] **Remove all post-creation** `.With*()` method calls
- [ ] **Update factory helper usage** to get Options, not mutate clients
- [ ] **Add environment variable usage** instead of hardcoded keys
- [ ] **Test basic functionality** with a simple request
- [ ] **Verify middleware ordering** if using multiple middlewares
- [ ] **Update documentation/comments** reflecting new patterns

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
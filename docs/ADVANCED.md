# 🔧 Advanced Wormhole Features

Comprehensive guide to enterprise-grade features and advanced patterns.

## Table of Contents

- [Custom Provider Registration](#custom-provider-registration)
- [Middleware System](#middleware-system)
- [Structured Output](#structured-output)
- [Model Validation & Constraints](#model-validation--constraints)
- [Error Handling](#error-handling)
- [Performance Optimization](#performance-optimization)
- [Production Patterns](#production-patterns)

## Custom Provider Registration

Add new AI providers without modifying core code.

### Basic Custom Provider

```go
// 1. Implement Provider interface
type MyProvider struct {
    config types.ProviderConfig
}

func (p *MyProvider) Text(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
    // Your implementation
    return &types.TextResponse{Text: "Response"}, nil
}

// Implement all other Provider methods...

// 2. Create factory function
func NewMyProvider(config types.ProviderConfig) (types.Provider, error) {
    return &MyProvider{config: config}, nil
}

// 3. Register and use with functional options
client := wormhole.New(
    wormhole.WithCustomProvider("custom", NewMyProvider),
    wormhole.WithProviderConfig("custom", types.ProviderConfig{
        APIKey: "key", 
        BaseURL: "https://api.custom.com",
    }),
)

// Register custom models
types.DefaultModelRegistry.Register(&types.ModelInfo{
    ID:           "custom-model",
    Provider:     "custom",
    Capabilities: []types.ModelCapability{types.CapabilityText},
    MaxTokens:    4096,
})

response, err := client.Text().
    Using("custom").
    Model("custom-model").
    Prompt("Hello").
    Generate(ctx)
```

### OpenAI-Compatible Shortcut

For providers using OpenAI's API format:

```go
// Cloud services (preserve API key)
client := wormhole.New(
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: "your-openrouter-key",
    }),
)

// Local services (no API key needed)  
client := wormhole.New(
    wormhole.WithOpenAICompatible("ollama", "http://localhost:11434", types.ProviderConfig{}),
)
```

## Middleware System

Enterprise-grade reliability and observability.

### Production Middleware Stack

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("your-api-key"),
    wormhole.WithTimeout(30*time.Second),              // Global timeouts
    wormhole.WithRetries(3, 2*time.Second),            // Auto-retry with backoff
    wormhole.WithMiddleware(
        middleware.RateLimitMiddleware(100),                         // 100 req/sec
        middleware.RetryMiddleware(middleware.DefaultRetryConfig()), // Auto-retry
        middleware.CircuitBreakerMiddleware(5, 30*time.Second),      // Failover
        middleware.TimeoutMiddleware(30 * time.Second),              // Request timeouts
        middleware.CacheMiddleware(middleware.CacheConfig{
            Cache: middleware.NewMemoryCache(1000),
            TTL:   10 * time.Minute,
        }),
        middleware.MetricsMiddleware(middleware.NewMetrics()),       // Observability
        middleware.DebugLoggingMiddleware(nil),                      // Request tracing
    ),
)
```

### Custom Middleware

```go
func CustomMiddleware() middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            // Pre-processing
            start := time.Now()
            
            // Call next middleware
            resp, err := next(ctx, req)
            
            // Post-processing
            duration := time.Since(start)
            log.Printf("Request took %v", duration)
            
            return resp, err
        }
    }
}

// Use custom middleware with functional options
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("your-api-key"),
    wormhole.WithMiddleware(CustomMiddleware()),
)
```

### Load Balancing

```go
providers := map[string]middleware.Handler{
    "primary": func(ctx context.Context, req interface{}) (interface{}, error) {
        return primaryProvider.Handle(ctx, req)
    },
    "secondary": func(ctx context.Context, req interface{}) (interface{}, error) {
        return secondaryProvider.Handle(ctx, req)
    },
}

client.Use(middleware.LoadBalancerMiddleware(middleware.RoundRobin, providers))
```

## Structured Output

Type-safe JSON responses with schema validation.

```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
    City string `json:"city"`
}

var person Person
err := client.Structured().
    Model("gpt-4o").
    Prompt("Generate a person profile").
    Schema(map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "name": map[string]interface{}{"type": "string"},
            "age":  map[string]interface{}{"type": "integer"},
            "city": map[string]interface{}{"type": "string"},
        },
        "required": []string{"name", "age"},
    }).
    GenerateAs(ctx, &person)
```

## Model Validation & Constraints

Automatic model capability and constraint validation.

```go
// Check if model supports capability
err := types.ValidateModelForCapability("gpt-5", types.CapabilityStructured)

// Get model constraints (e.g., GPT-5 requires temperature=1.0)
constraints, err := types.GetModelConstraints("gpt-5")

// List available models for provider
models := types.ListAvailableModels("openai")

// Estimate costs
cost, err := types.EstimateModelCost("gpt-4o", 1000, 500) // input/output tokens
```

## Error Handling

Structured error types with retry guidance.

```go
response, err := client.Text().Generate(ctx)
if err != nil {
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        switch wormholeErr.Code {
        case types.ErrorCodeRateLimit:
            // Retry - middleware handles this automatically
            log.Printf("Rate limited: %s", wormholeErr.Details)
        case types.ErrorCodeAuth:
            // Fix API key - no point retrying
            log.Fatal("Invalid API key")
        case types.ErrorCodeModel:
            // Try different model
            return client.Text().Model("gpt-3.5-turbo").Generate(ctx)
        case types.ErrorCodeTimeout:
            // Increase timeout
            ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
            defer cancel()
            return client.Text().Generate(ctx)
        }
    }
}
```

## Performance Optimization

### Concurrency

```go
func parallelGeneration(prompts []string) {
    var wg sync.WaitGroup
    results := make(chan string, len(prompts))
    
    for _, prompt := range prompts {
        wg.Add(1)
        go func(p string) {
            defer wg.Done()
            resp, err := client.Text().
                Model("gpt-3.5-turbo").
                Prompt(p).
                Generate(context.Background())
            if err == nil {
                results <- resp.Text
            }
        }(prompt)
    }
    
    wg.Wait()
    close(results)
}
```

### Streaming for Long Responses

```go
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Write a long story").
    Stream(ctx)

for chunk := range stream {
    if chunk.Error != nil {
        log.Printf("Stream error: %v", chunk.Error)
        break
    }
    fmt.Print(chunk.Text) // Display immediately
}
```

### Connection Pooling

```go
// Configure HTTP client for connection reuse
config := wormhole.Config{
    Providers: map[string]types.ProviderConfig{
        "openai": {
            APIKey: "key",
            // Custom HTTP client with pooling
        },
    },
}
```

## Production Patterns

### Health Checking

```go
checker := middleware.NewHealthChecker(30 * time.Second)
checker.SetCheckFunction(func(ctx context.Context, provider string) error {
    // Custom health check
    _, err := client.Text().
        Using(provider).
        Model("gpt-3.5-turbo").
        Prompt("health").
        MaxTokens(1).
        Generate(ctx)
    return err
})

checker.Start([]string{"openai", "anthropic"})
defer checker.Stop()
```

### Multi-Provider Fallback

```go
providers := []string{"openai", "anthropic", "gemini"}

for _, provider := range providers {
    resp, err := client.Text().
        Using(provider).
        Model(getModelForProvider(provider)).
        Prompt("Generate response").
        Generate(ctx)
    
    if err == nil {
        return resp, nil
    }
    
    log.Printf("Provider %s failed: %v", provider, err)
}

return nil, errors.New("all providers failed")
```

### Configuration Management

```go
type Config struct {
    Providers    map[string]ProviderConfig `yaml:"providers"`
    DefaultModel string                    `yaml:"default_model"`
    Timeout      time.Duration            `yaml:"timeout"`
    RateLimit    int                      `yaml:"rate_limit"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var config Config
    err = yaml.Unmarshal(data, &config)
    return &config, err
}
```

### Monitoring & Metrics

```go
metrics := middleware.NewMetrics()
client.Use(middleware.MetricsMiddleware(metrics))

// Collect metrics
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        requests, errors, avgDuration := metrics.GetStats()
        log.Printf("Metrics - Requests: %d, Errors: %d, Avg: %v", 
            requests, errors, avgDuration)
    }
}()
```

## Best Practices

1. **Always use context with timeouts**
2. **Implement proper error handling with retries**
3. **Use middleware for cross-cutting concerns**
4. **Monitor provider performance and implement fallbacks**
5. **Cache responses when appropriate**
6. **Validate models before making requests**
7. **Use structured output for reliable data extraction**
8. **Implement health checks for production systems**

See the `examples/` directory for complete working implementations of these patterns.
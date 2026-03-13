# Options and Configuration

The functional options pattern for flexible client configuration.

## Overview

Wormhole uses the **functional options pattern** for client configuration. This pattern provides:

- **Zero default configuration** - clients work without any options
- **Composability** - combine options in any order
- **Extensibility** - add custom options without breaking existing code
- **Type safety** - compile-time validation of configuration
- **Future-proof** - add new options without changing the API

In practice, this makes client setup both composable and extensible as your application grows.

## The Options Pattern

### Basic Concept

An `Option` is a function that modifies a configuration struct:

```go
// Option is a function that configures a Wormhole client
type Option func(*Config)

// Example option implementation
func WithDefaultProvider(name string) Option {
    return func(c *Config) {
        c.DefaultProvider = name
    }
}
```

### Usage

Options are passed to `New()` and applied in order:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithAnthropic(apiKey),
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithDebugLogging(),
)
```

**Benefits over struct-based configuration:**

| Struct Config | Options Pattern |
|---------------|-----------------|
| `Config{Provider: "openai", Timeout: 30}` | `WithOpenAI(key), WithTimeout(30)` |
| Must know all fields | Discover options via autocomplete |
| Hard to add defaults | Sensible defaults built-in |
| Breaking changes add fields | New options are non-breaking |

## Available Options

### Provider Configuration

#### WithOpenAI

Configure the OpenAI provider:

```go
client := wormhole.New(
    wormhole.WithOpenAI("sk-..."),
)
```

With additional configuration:

```go
client := wormhole.New(
    wormhole.WithOpenAI("sk-...", types.ProviderConfig{
        BaseURL: "https://api.openai.com/v1",
        Timeout: 60 * time.Second,
    }),
)
```

#### WithAnthropic

Configure the Anthropic provider:

```go
client := wormhole.New(
    wormhole.WithAnthropic("sk-ant-..."),
)
```

#### WithGemini

Configure the Google Gemini provider:

```go
client := wormhole.New(
    wormhole.WithGemini("AIza..."),
)
```

#### WithOpenAICompatible

Configure a generic OpenAI-compatible provider:

```go
client := wormhole.New(
    wormhole.WithOpenAICompatible("custom", "https://api.example.com/v1", types.ProviderConfig{
        APIKey: "key",
    }),
)
```

**Common OpenAI-compatible providers:**

- **Groq**: `WithGroq(apiKey)`
- **Mistral**: `WithMistral(config)`
- **OpenRouter**: `WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", config)`

#### WithGroq

Configure Groq using its OpenAI-compatible API:

```go
client := wormhole.New(
    wormhole.WithGroq("gsk_..."),
)
```

#### WithMistral

Configure Mistral using the built-in OpenAI-compatible adapter:

```go
client := wormhole.New(
    wormhole.WithMistral(types.ProviderConfig{APIKey: "mistral-key"}),
)
```

#### WithOllama

Configure a local Ollama server:

```go
client := wormhole.New(
    wormhole.WithOllama(types.ProviderConfig{BaseURL: "http://localhost:11434"}),
)
```

#### WithLMStudio

Configure LM Studio as a local OpenAI-compatible provider:

```go
client := wormhole.New(
    wormhole.WithLMStudio(types.ProviderConfig{BaseURL: "http://localhost:1234/v1"}),
)
```

#### WithVLLM

Configure a vLLM deployment:

```go
client := wormhole.New(
    wormhole.WithVLLM(types.ProviderConfig{BaseURL: "http://localhost:8000/v1"}),
)
```

#### WithOllamaOpenAI

Configure Ollama through its OpenAI-compatible endpoint:

```go
client := wormhole.New(
    wormhole.WithOllamaOpenAI(types.ProviderConfig{BaseURL: "http://localhost:11434/v1"}),
)
```

#### WithCustomProvider

Register a custom provider with a factory function:

```go
client := wormhole.New(
    wormhole.WithCustomProvider("myprovider", func(cfg types.ProviderConfig) (types.Provider, error) {
        return &MyProvider{config: cfg}, nil
    }),
    wormhole.WithProviderConfig("myprovider", types.ProviderConfig{
        APIKey: "key",
    }),
)
```

### Client Behavior

#### WithDefaultProvider

Set the default provider for requests:

```go
client := wormhole.New(
    wormhole.WithOpenAI(openAIKey),
    wormhole.WithAnthropic(anthropicKey),
    wormhole.WithDefaultProvider("openai"),
)

// Uses "openai" by default
resp, _ := client.Text().Model("gpt-5.2").Prompt("Hello").Generate(ctx)

// Override for specific request
resp, _ := client.Text().Using("anthropic").Model("claude-sonnet-4-5").Prompt("Hello").Generate(ctx)
```

#### WithTimeout

Set the default timeout for all requests:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithTimeout(60 * time.Second),
)
```

#### WithUnlimitedTimeout

Disable timeouts for long-running operations:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithUnlimitedTimeout(),
)
```

**Use case**: Heavy text processing that may take 3+ minutes.

### Logging and Debugging

#### WithDebugLogging

Enable debug logging:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithDebugLogging(),
)
```

With custom logger:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithDebugLogging(types.LoggerFunc(func(fmt string, args ...any) {
        log.Printf("[Wormhole] "+fmt, args...)
    })),
)
```

#### WithLogger

Set a custom logger without enabling debug mode:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithLogger(myLogger),
)
```

### Middleware

#### WithProviderMiddleware

Add type-safe middleware to the execution chain:

```go
import "github.com/garyblankenship/wormhole/pkg/middleware"

client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithProviderMiddleware(
        middleware.NewCircuitBreaker(5, 30*time.Second),
        middleware.RateLimitMiddleware(10),
    ),
)
```

#### WithMiddleware (Deprecated)

Legacy middleware (automatically converted to type-safe):

```go
// Deprecated: Use WithProviderMiddleware instead
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithMiddleware(legacyMiddleware...),
)
```

### Model Discovery

#### WithDiscoveryConfig

Configure dynamic model discovery:

```go
import "github.com/garyblankenship/wormhole/pkg/discovery"

client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        CacheTTL:        12 * time.Hour,  // Cache for 12 hours
        RefreshInterval: 6 * time.Hour,   // Refresh every 6 hours
        OfflineMode:     false,           // Allow network fetching
    }),
)
```

#### WithDiscovery

Enable or disable discovery:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithDiscovery(false), // Disable discovery, use fallback models
)
```

#### WithOfflineMode

Enable offline mode (no network fetching):

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithOfflineMode(true),
)
```

#### WithModelValidation

Enable or disable model validation:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithModelValidation(true), // Enable validation
)
```

### Environment Variables

#### WithProviderFromEnv

Configure a provider from environment variables:

```go
// Set environment variables:
// export OPENAI_API_KEY=sk-...
// export ANTHROPIC_API_KEY=sk-ant-...

client := wormhole.New(
    wormhole.WithProviderFromEnv("openai"),
    wormhole.WithProviderFromEnv("anthropic"),
    wormhole.WithDefaultProvider("openai"),
)
```

**Supported providers:**

| Provider | API Key Env Var | Base URL Env Var |
|----------|----------------|------------------|
| `openai` | `OPENAI_API_KEY` | `OPENAI_BASE_URL` |
| `anthropic` | `ANTHROPIC_API_KEY` | `ANTHROPIC_BASE_URL` |
| `gemini` | `GEMINI_API_KEY` | `GEMINI_BASE_URL` |
| `groq` | `GROQ_API_KEY` | - |
| `openrouter` | `OPENROUTER_API_KEY` | - |

**Silent skip**: If the environment variable is not set, the option does nothing (no error).

#### WithAllProvidersFromEnv

Configure all available providers from environment:

```go
// All providers with API keys in env will be configured
client := wormhole.New(
    wormhole.WithAllProvidersFromEnv(),
    wormhole.WithDefaultProvider("openai"),
)
```

### Idempotency

#### WithIdempotencyKey

Add idempotency key for duplicate operation prevention:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithIdempotencyKey("req-123", 1*time.Hour),
)
```

**How it works**: The SDK caches responses keyed by the idempotency key. If the same key is used again, the cached response is returned instead of making a new request.

**Use cases**:

- Preventing duplicate charge operations during retries
- Ensuring exactly-once processing in distributed systems
- Avoiding duplicate API calls on client retry

## Option Composition

### Basic Composition

Options are applied in order, allowing overwriting:

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithDefaultProvider("anthropic"), // This overwrites the previous
)
// Result: default provider is "anthropic"
```

### Conditional Options

Use conditionals to build option lists:

```go
opts := []wormhole.Option{
    wormhole.WithOpenAI(openAIKey),
}

if enableDebug {
    opts = append(opts, wormhole.WithDebugLogging())
}

if anthropicKey != "" {
    opts = append(opts, wormhole.WithAnthropic(anthropicKey))
}

client := wormhole.New(opts...)
```

### Reusable Option Groups

Create reusable option groups:

```go
// Production configuration
var ProductionOpts = []wormhole.Option{
    wormhole.WithProviderFromEnv("openai"),
    wormhole.WithProviderFromEnv("anthropic"),
    wormhole.WithTimeout(60 * time.Second),
    wormhole.WithProviderMiddleware(
        middleware.NewCircuitBreaker(5, 30*time.Second),
    ),
}

// Test configuration
var TestOpts = []wormhole.Option{
    wormhole.WithOpenAI("test-key"),
    wormhole.WithDebugLogging(),
    wormhole.WithTimeout(5 * time.Second),
}

// Usage
client := wormhole.New(ProductionOpts...)
```

### Dynamic Provider Configuration

Load providers dynamically:

```go
func NewClientFromConfig(cfg Config) (*wormhole.Wormhole, error) {
    opts := []wormhole.Option{}

    if cfg.OpenAIKey != "" {
        opts = append(opts, wormhole.WithOpenAI(cfg.OpenAIKey))
    }

    if cfg.AnthropicKey != "" {
        opts = append(opts, wormhole.WithAnthropic(cfg.AnthropicKey))
    }

    if cfg.DefaultProvider != "" {
        opts = append(opts, wormhole.WithDefaultProvider(cfg.DefaultProvider))
    }

    if cfg.Debug {
        opts = append(opts, wormhole.WithDebugLogging())
    }

    return wormhole.New(opts...), nil
}
```

## Request Options vs Client Options

Wormhole uses **two different patterns** for configuration:

### Client Options (Functional Options)

Applied at client creation, affect all requests:

```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithTimeout(30 * time.Second),
)
```

### Request Options (Builder Pattern)

Applied per-request, override client defaults:

```go
resp, err := client.Text().
    Using("anthropic").      // Override provider
    Model("claude-sonnet-4-5").  // Set model
    Temperature(0.7).        // Set temperature
    Prompt("Hello").         // Set prompt
    Generate(ctx)
```

**Key differences:**

| Aspect | Client Options | Request Options |
|--------|----------------|-----------------|
| **Pattern** | Functional options | Builder pattern |
| **Scope** | All requests | Single request |
| **Override** | Later options win | Builder methods chain |
| **Examples** | `WithOpenAI()`, `WithTimeout()` | `.Model()`, `.Temperature()` |

## Custom Options

### Creating Custom Options

You can create custom options for your application:

```go
// Custom option for your application
func WithMyAppConfig(cfg MyAppConfig) wormhole.Option {
    return func(c *wormhole.Config) {
        // Modify the config based on your app settings
        if cfg.Debug {
            c.DebugLogging = true
        }
        if cfg.Timeout > 0 {
            c.DefaultTimeout = cfg.Timeout
        }
        if cfg.DefaultProvider != "" {
            c.DefaultProvider = cfg.DefaultProvider
        }
    }
}

// Usage
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    WithMyAppConfig(myConfig),
)
```

### Provider-Specific Options

Create options for provider-specific settings:

```go
// Option for OpenAI-specific settings
func WithOpenAIOrganization(org string) wormhole.Option {
    return func(c *wormhole.Config) {
        if c.Providers == nil {
            c.Providers = make(map[string]types.ProviderConfig)
        }
        cfg := c.Providers["openai"]
        cfg.Organization = org
        c.Providers["openai"] = cfg
    }
}

// Usage
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    WithOpenAIOrganization("org-123"),
)
```

## Best Practices

### DO

- **Use environment variables** for sensitive data (API keys)
- **Group related options** in reusable slices
- **Enable debug logging** during development
- **Set appropriate timeouts** based on your use case
- **Use WithProviderFromEnv** for flexible configuration

### DON'T

- **Hardcode API keys** in source code
- **Mix provider configuration** in a single option
- **Overwrite options** unintentionally (order matters)
- **Forget timeout** on production deployments
- **Enable debug logging** in production (performance impact)

### Example: Production Configuration

```go
func NewProductionClient() (*wormhole.Wormhole, error) {
    return wormhole.New(
        // Providers from environment
        wormhole.WithAllProvidersFromEnv(),
        wormhole.WithDefaultProvider("openai"),

        // Production timeout
        wormhole.WithTimeout(60 * time.Second),

        // Production middleware
        wormhole.WithProviderMiddleware(
            middleware.NewCircuitBreaker(5, 30*time.Second),
            middleware.RateLimitMiddleware(10),
        ),

        // Discovery settings
        wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
            CacheTTL:        12 * time.Hour,
            RefreshInterval: 6 * time.Hour,
            OfflineMode:     false,
        }),
    ), nil
}
```

### Example: Test Configuration

```go
func NewTestClient(apiKey string) *wormhole.Wormhole {
    return wormhole.New(
        wormhole.WithOpenAI(apiKey),
        wormhole.WithDebugLogging(),
        wormhole.WithTimeout(5 * time.Second),
        wormhole.WithDiscovery(false), // Use fallback models only
    )
}
```

## See Also

- [Client Architecture](./client.md) - Client lifecycle and usage
- [Errors](./errors.md) - Error handling and types

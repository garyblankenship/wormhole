# OpenRouter Provider

The OpenRouter provider provides access to 200+ models from multiple AI providers through a unified API gateway.

## Quick Start

### Creating a Client

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/pkg/types"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Create a new client with OpenRouter configuration
    client := wormhole.New(
        wormhole.WithDefaultProvider("openrouter"),
        wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
            APIKey:        os.Getenv("OPENROUTER_API_KEY"),
            DynamicModels: true, // Enable access to all 200+ models
        }),
    )

    ctx := context.Background()

    // Simple text generation with any OpenRouter model
    response, err := client.Text().
        Model("anthropic/claude-sonnet-4-5").
        Prompt("Hello from OpenRouter!").
        Generate(ctx)

    if err != nil {
        panic(err)
    }

    fmt.Println(response.Text)
}
```

### Quick Factory Method

For the simplest setup, use the factory method:

```go
client, err := wormhole.QuickOpenRouter()
if err != nil {
    panic(err)
}

// Access any model from 200+ available
response, err := client.Text().
    Model("openai/gpt-5-mini").
    Prompt("Explain quantum computing").
    Generate(context.Background())
```

## Model Routing

OpenRouter uses a **provider/model** naming convention that gives you access to models from multiple providers:

### Model ID Format

All OpenRouter model IDs follow the format: `provider/model-name`

| Provider | Model ID Pattern | Example |
|----------|-----------------|---------|
| OpenAI | `openai/*` | `openai/gpt-5-mini` |
| Anthropic | `anthropic/*` | `anthropic/claude-sonnet-4-5` |
| Google | `google/*` | `google/gemini-2.5-flash` |
| Meta | `meta-llama/*` | `meta-llama/llama-3.1-70b-instruct` |
| Mistral AI | `mistralai/*` | `mistralai/mixtral-8x7b-instruct` |
| Cohere | `cohere/*` | `cohere/command-r-plus` |
| xAI | `x-ai/*` | `x-ai/grok-beta` |

### Popular Models

```go
// Flagship models
models := []string{
    "openai/gpt-5-mini",           // Efficient and fast
    "anthropic/claude-sonnet-4-5", // Balanced performance
    "google/gemini-2.5-flash",     // Fast with multimodal
    "meta-llama/llama-3.1-70b-instruct", // Open-source powerhouse
}

// Try multiple providers for the same prompt
for _, model := range models {
    response, err := client.Text().
        Model(model).
        Prompt("What is Go?").
        Generate(ctx)

    if err != nil {
        continue
    }

    fmt.Printf("%s: %s\n", model, response.Text)
}
```

## Capabilities

The OpenRouter provider supports the following capabilities (model-dependent):

- `CapabilityText` - Text generation
- `CapabilityChat` - Chat/completion
- `CapabilityStructured` - Structured output (JSON schema)
- `CapabilityStream` - Streaming responses
- `CapabilityFunctions` - Function/tool calling
- `CapabilityEmbeddings` - Text embeddings
- `CapabilityVision` - Image input (vision models)
- `CapabilityImages` - Image generation

**Note**: Capabilities vary by model. Check individual model documentation for supported features.

## OpenRouter-Specific Features

### Dynamic Model Support

OpenRouter supports **dynamic models**, meaning you can use any model without pre-registration:

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openrouter"),
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey:        os.Getenv("OPENROUTER_API_KEY"),
        DynamicModels: true, // Essential for OpenRouter
    }),
)

// Any model name works - no registration needed
response, err := client.Text().
    Model("deepseek/deepseek-chat").
    Prompt("Hello").
    Generate(ctx)
```

### Provider-Specific Headers

OpenRouter supports additional headers for request metadata:

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openrouter"),
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: os.Getenv("OPENROUTER_API_KEY"),
        Headers: map[string]string{
            "HTTP-Referer":           "https://your-app.com",
            "X-Title":                "My App",
            "X-Client-User-Agent":    "wormhole-sdk/1.0",
        },
    }),
)
```

### Model Discovery

Fetch available models dynamically:

```go
import "github.com/garyblankenship/wormhole/pkg/discovery"

fetcher := discovery.NewOpenRouterFetcher()
models, err := fetcher.FetchModels(context.Background())

for _, model := range models {
    fmt.Printf("Model: %s\n", model.ID)
    fmt.Printf("  Name: %s\n", model.Name)
    fmt.Printf("  Provider: %s\n", model.Provider)
    fmt.Printf("  Capabilities: %v\n", model.Capabilities)
}
```

### Cost Tracking

OpenRouter returns pricing information in responses:

```go
response, err := client.Text().
    Model("openai/gpt-5-mini").
    Prompt("Explain Go").
    Generate(ctx)

if response.Usage != nil {
    fmt.Printf("Prompt tokens: %d\n", response.Usage.PromptTokens)
    fmt.Printf("Completion tokens: %d\n", response.Usage.CompletionTokens)
    // OpenRouter adds cost info to response metadata
}
```

## Streaming

Stream responses in real-time with model comparison:

```go
models := []string{
    "openai/gpt-5-mini",
    "anthropic/claude-sonnet-4-5",
}

for _, model := range models {
    fmt.Printf("Testing %s...\n", model)

    chunks, err := client.Text().
        Model(model).
        Prompt("Write a haiku about Go").
        Stream(ctx)

    if err != nil {
        fmt.Printf("Error: %v\n", err)
        continue
    }

    for chunk := range chunks {
        if chunk.Delta != nil {
            fmt.Print(chunk.Delta.Content)
        }
    }
    fmt.Println("\n---")
}
```

## Tool Calling

Many OpenRouter models support function calling:

```go
tools := []types.Tool{
    {
        Type: "function",
        Function: &types.ToolFunction{
            Name:        "get_weather",
            Description: "Get current weather for a location",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]any{
                        "type":        "string",
                        "description": "City name, e.g. San Francisco, CA",
                    },
                },
                "required": []string{"location"},
            },
        },
    },
}

response, err := client.Text().
    Model("anthropic/claude-sonnet-4-5").
    Prompt("What's the weather in Tokyo?").
    Tools(tools).
    Generate(ctx)

if len(response.ToolCalls) > 0 {
    toolCall := response.ToolCalls[0]
    fmt.Printf("Tool: %s\n", toolCall.Name)
    fmt.Printf("Args: %v\n", toolCall.Arguments)
}
```

## Structured Output

Get JSON responses with schema validation:

```go
type Analysis struct {
    Sentiment   string   `json:"sentiment"`
    Topics      []string `json:"topics"`
    Confidence  float64  `json:"confidence"`
}

var result Analysis
err := client.Structured().
    Model("anthropic/claude-sonnet-4-5").
    Prompt("Analyze: Go is an amazing language for concurrent programming").
    SchemaName("analysis").
    GenerateAs(ctx, &result)

fmt.Printf("Sentiment: %s\n", result.Sentiment)
fmt.Printf("Topics: %v\n", result.Topics)
```

## Embeddings

Generate embeddings using various embedding models:

```go
response, err := client.Embeddings().
    Model("openai/text-embedding-3-small").
    Input([]string{
        "Go is awesome",
        "Rust is powerful",
        "Python is simple",
    }).
    Generate(ctx)

for _, embedding := range response.Embeddings {
    fmt.Printf("Index %d: %d dimensions\n", embedding.Index, len(embedding.Embedding))
}
```

## Pricing and Rate Limits

### Pricing

OpenRouter uses **pay-per-use** pricing with competitive rates:

- Pricing varies by model and provider
- Check [OpenRouter Models](https://openrouter.ai/models) for current rates
- Costs are calculated per 1M tokens (prompt + completion)

```go
// Example pricing (as of 2025):
// openai/gpt-5-mini:      $0.15/1M prompt, $0.60/1M completion
// anthropic/claude-3-5-haiku: $0.80/1M prompt, $1.00/1M completion
// meta-llama/llama-3.1-8b:   $0.05/1M prompt, $0.05/1M completion
```

### Rate Limits

Rate limits vary by model and provider:

- **Free tier**: 20 requests/minute, 200 requests/day
- **Paid tier**: Limits depend on credits purchased
- Headers returned: `x-ratelimit-limit-requests`, `x-ratelimit-remaining-requests`

```go
// Handle rate limits gracefully
response, err := client.Text().
    Model("anthropic/claude-sonnet-4-5").
    Prompt("Hello").
    Generate(ctx)

if err != nil {
    var apiErr *types.WormholeError
    if errors.As(err, &apiErr) && apiErr.StatusCode == 429 {
        // Rate limit exceeded - implement backoff
        time.Sleep(time.Second)
        // Retry logic...
    }
}
```

### Usage Optimization

```go
// Use cheaper models for simple tasks
cheapModels := []string{
    "meta-llama/llama-3.1-8b-instruct",
    "google/gemma-2-9b-it",
}

// Use premium models for complex reasoning
premiumModels := []string{
    "anthropic/claude-opus-4-5",
    "openai/gpt-5",
}
```

## Error Handling

```go
response, err := client.Text().
    Model("openai/gpt-5-mini").
    Prompt("Hello").
    Generate(ctx)

if err != nil {
    var apiErr *types.WormholeError
    if errors.As(err, &apiErr) {
        fmt.Printf("Provider: %s\n", apiErr.Provider)
        fmt.Printf("Status: %d\n", apiErr.StatusCode)
        fmt.Printf("Message: %s\n", apiErr.Message)

        switch apiErr.StatusCode {
        case 401:
            fmt.Println("Invalid API key")
        case 429:
            fmt.Println("Rate limit exceeded")
        case 402:
            fmt.Println("Insufficient credits")
        case 500:
            fmt.Println("OpenRouter service error")
        }
    }
    return
}
```

## Configuration Options

### Base URL

Override the default OpenRouter endpoint (not recommended):

```go
client := wormhole.New(
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey:  os.Getenv("OPENROUTER_API_KEY"),
        BaseURL: "https://custom-gateway.example.com",
    }),
)
```

### Timeout

Set request timeout:

```go
client := wormhole.New(
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey:  os.Getenv("OPENROUTER_API_KEY"),
        Timeout: 60, // 60 seconds
    }),
)
```

## Best Practices

### 1. Model Fallback Strategy

```go
models := []string{
    "anthropic/claude-sonnet-4-5", // Primary
    "openai/gpt-5-mini",           // Fallback 1
    "google/gemini-2.5-flash",     // Fallback 2
}

var lastErr error
for _, model := range models {
    response, err := client.Text().
        Model(model).
        Prompt("Explain microservices").
        Generate(ctx)

    if err == nil {
        return response, nil
    }
    lastErr = err
}

return nil, lastErr
```

### 2. Cost Optimization

```go
// Use cheaper models for drafts
draft, _ := client.Text().
    Model("meta-llama/llama-3.1-8b-instruct").
    Prompt("Draft an article about Go").
    Generate(ctx)

// Refine with premium model
final, _ := client.Text().
    Model("anthropic/claude-sonnet-4-5").
    Prompt(fmt.Sprintf("Improve: %s", draft.Text)).
    Generate(ctx)
```

### 3. Provider Diversity

```go
// Avoid vendor lock-in by testing across providers
providers := map[string][]string{
    "openai":     {"openai/gpt-5-mini", "openai/gpt-5.2"},
    "anthropic":  {"anthropic/claude-sonnet-4-5", "anthropic/claude-haiku-4-5"},
    "google":     {"google/gemini-2.5-flash", "google/gemini-2.5-pro"},
    "meta":       {"meta-llama/llama-3.1-70b-instruct"},
}
```

## Reference

- [OpenRouter Documentation](https://openrouter.ai/docs)
- [OpenRouter Models](https://openrouter.ai/models)
- [OpenRouter API Reference](https://openrouter.ai/docs/quick-start)
- [Pricing](https://openrouter.ai/docs#models)

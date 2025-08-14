# 🚀 Wormhole Quick Start

Get up and running with the fastest LLM SDK in 2 minutes.

## Installation

```bash
go get github.com/garyblankenship/wormhole@latest
```

## Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Create client
    client := wormhole.New(wormhole.Config{
        DefaultProvider: "openai",
        Providers: map[string]types.ProviderConfig{
            "openai": {APIKey: "your-api-key"},
        },
    })

    // Generate text
    response, err := client.Text().
        Model("gpt-4o").
        Prompt("Hello, world!").
        Generate(context.Background())

    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.Text)
}
```

## Next Steps

- **[Full Documentation](../README.md)** - Complete features and examples
- **[Provider Setup](PROVIDERS.md)** - Configure OpenAI, Anthropic, Gemini, etc.
- **[Examples](../examples/)** - Working code examples for every feature
- **[Advanced Features](ADVANCED.md)** - Middleware, streaming, custom providers

## Common First Steps

### 1. Text Generation
```go
response, err := client.Text().
    Model("gpt-4o").
    Prompt("Explain quantum computing").
    MaxTokens(100).
    Generate(ctx)
```

### 2. Streaming Responses
```go
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Write a story").
    Stream(ctx)

for chunk := range stream {
    fmt.Print(chunk.Text)
}
```

### 3. Multiple Providers
```go
config := wormhole.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{
        "openai":    {APIKey: "your-openai-key"},
        "anthropic": {APIKey: "your-anthropic-key"},
    },
}

client := wormhole.New(config)

// Use specific provider
response, err := client.Text().
    Using("anthropic").
    Model("claude-3-opus").
    Prompt("Hello from Anthropic").
    Generate(ctx)
```

## Need Help?

- **Issues**: [GitHub Issues](https://github.com/garyblankenship/wormhole/issues)
- **Examples**: Check the `examples/` directory
- **Documentation**: Full README with all features

**Performance**: Wormhole operates at 94.89ns overhead - 116x faster than alternatives. 🚀
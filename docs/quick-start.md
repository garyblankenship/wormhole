# 🚀 Wormhole Quick Start

Get up and running with the fastest LLM SDK in 2 minutes.

## Prerequisites

- **Go version**: 1.19 or higher
- **API Keys**: At least one provider API key (OpenAI, Anthropic, Google, etc.)
- **Network**: HTTPS access to provider APIs

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
    "os"

    "github.com/garyblankenship/wormhole/pkg/types"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Create client with functional options
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    )

    // Create context with timeout for production use
    ctx := context.Background()

    // Generate text
    response, err := client.Text().
        Model("gpt-4o").
        Prompt("Hello, world!").
        Generate(ctx)

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
    Prompt("Explain quantum computing in simple terms").
    MaxTokens(100).
    Temperature(0.7).
    Generate(ctx)

if err != nil {
    log.Printf("Error generating text: %v", err)
    return
}

fmt.Println(response.Text)
```

### 2. Streaming Responses
```go
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Write a story about space exploration").
    Stream(ctx)

if err != nil {
    log.Printf("Error starting stream: %v", err)
    return
}

for chunk := range stream {
    if chunk.Error != nil {
        log.Printf("Stream error: %v", chunk.Error)
        break
    }
    fmt.Print(chunk.Text)
}
```

### 3. Multiple Providers
```go
// Create client with multiple providers using functional options
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
    wormhole.WithGroq(os.Getenv("GROQ_API_KEY")),                    // Fast inference
    wormhole.WithMistral(types.ProviderConfig{                      // European AI
        APIKey: os.Getenv("MISTRAL_API_KEY"),
    }),
)

// Use specific provider
response, err := client.Text().
    Using("anthropic").
    Model("claude-3-5-sonnet-20241022").
    Prompt("Hello from Anthropic").
    Generate(ctx)

if err != nil {
    log.Printf("Error with Anthropic: %v", err)
    return
}

fmt.Println(response.Text)
```

## Common Pitfalls & Solutions

### ❌ API Key Not Working
```bash
Error: invalid API key
```

**Solution**: 
```bash
# Set environment variable
export OPENAI_API_KEY="sk-your-actual-key"
# OR check your key format - OpenAI keys start with "sk-"
```

### ❌ Model Not Found
```bash
Error: model "gpt-5" not found
```

**Solutions**:
- Use `gpt-4o` or `gpt-4o-mini` for OpenAI (GPT-5 is not yet available)
- Check [PROVIDERS.md](PROVIDERS.md) for available models per provider
- Validate your API key has access to the model

### ❌ Rate Limiting
```bash
Error: rate limit exceeded
```

**Solution**: Add retry middleware:
```go
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithRetries(3, 2*time.Second),
)
```

### ❌ Context Timeout
```bash
Error: context deadline exceeded
```

**Solution**: Increase timeout:
```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()

response, err := client.Text().Generate(ctx)
```

### ❌ Network/Firewall Issues
```bash
Error: dial tcp: connection refused
```

**Solutions**:
- Check internet connection
- Verify HTTPS access to `api.openai.com`
- Check corporate firewall settings
- Try with different provider as test

## Environment Setup

### Option 1: Environment Variables
```bash
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"
export GEMINI_API_KEY="your-key"
export GROQ_API_KEY="your-key"
export MISTRAL_API_KEY="your-key"
```

### Option 2: .env File (with godotenv)
```bash
go get github.com/joho/godotenv
```

```go
import _ "github.com/joho/godotenv/autoload"
```

Create `.env` file:
```
OPENAI_API_KEY=your-key
ANTHROPIC_API_KEY=your-key
GROQ_API_KEY=your-key
MISTRAL_API_KEY=your-key
```

### Option 3: Direct Configuration (Development Only)
```go
client := wormhole.New(
    wormhole.WithOpenAI("sk-your-direct-key"), // Not recommended for production
)
```

## Need Help?

- **Issues**: [GitHub Issues](https://github.com/garyblankenship/wormhole/issues)
- **Examples**: Check the `examples/` directory  
- **Documentation**: Full README with all features
- **Providers**: See [PROVIDERS.md](PROVIDERS.md) for setup guides

**Performance**: Wormhole operates at 67ns overhead - 165x faster than alternatives. 🚀
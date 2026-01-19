# Anthropic Provider

The Anthropic provider provides access to Claude models via the Anthropic Messages API.

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
    // Create a new client with Anthropic configuration
    client := wormhole.New(
        wormhole.WithDefaultProvider("anthropic"),
        wormhole.WithProviderConfig("anthropic", types.ProviderConfig{
            APIKey: os.Getenv("ANTHROPIC_API_KEY"),
        }),
    )

    ctx := context.Background()

    // Simple text generation
    response, err := client.Text().
        Model("claude-sonnet-4-5").
        Prompt("Hello, Claude!").
        Generate(ctx)

    if err != nil {
        panic(err)
    }

    fmt.Println(response.Text)
}
```

### Direct Provider Initialization

For more control, you can create the Anthropic provider directly:

```go
import anthropic "github.com/garyblankenship/wormhole/pkg/providers/anthropic"

provider := anthropic.New(types.ProviderConfig{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    // BaseURL is optional; defaults to https://api.anthropic.com/v1
})
```

## Supported Models

### Claude 4.5 Family (Latest)

| Model ID | Description |
|----------|-------------|
| `claude-sonnet-4-5` | Latest Sonnet (alias) |
| `claude-sonnet-4-5-20250929` | Sonnet 4.5 (dated) |
| `claude-haiku-4-5` | Latest Haiku (alias) |
| `claude-haiku-4-5-20251001` | Haiku 4.5 (dated) |
| `claude-opus-4-5` | Latest Opus (alias) |
| `claude-opus-4-5-20251101` | Opus 4.5 (dated) |

### Claude 4.x Family (Legacy)

| Model ID | Description |
|----------|-------------|
| `claude-opus-4-1` | Opus 4.1 (alias) |
| `claude-opus-4-1-20250805` | Opus 4.1 (dated) |
| `claude-sonnet-4` | Sonnet 4.0 (alias) |
| `claude-sonnet-4-0-20250514` | Sonnet 4.0 (dated) |

### Claude 3.x Family (Legacy)

| Model ID | Description |
|----------|-------------|
| `claude-3-7-sonnet` | Sonnet 3.7 (alias) |
| `claude-3-7-sonnet-20250219` | Sonnet 3.7 (dated) |
| `claude-3-5-haiku` | Haiku 3.5 (alias) |
| `claude-3-5-haiku-20241022` | Haiku 3.5 (dated) |
| `claude-3-haiku-20240307` | Haiku 3 (dated) |

**Note**: Prefer using model aliases (e.g., `claude-sonnet-4-5`) over dated versions for better maintainability.

## Capabilities

The Anthropic provider supports the following capabilities:

- `CapabilityText` - Text generation
- `CapabilityChat` - Chat/completion
- `CapabilityStructured` - Structured output (via tool calling)
- `CapabilityStream` - Streaming responses
- `CapabilityFunctions` - Function/tool calling

## Anthropic-Specific Features

### System Prompts

Anthropic requires system prompts to be in a separate field rather than as a message. The SDK handles this automatically:

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    SystemPrompt("You are a helpful assistant specializing in Go programming.").
    Prompt("Explain goroutines").
    Generate(ctx)
```

### Max Tokens (Required)

Anthropic requires the `max_tokens` field to be set. The SDK defaults to 4096 if not specified:

```go
maxTokens := 1024
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Summarize this text").
    MaxTokens(&maxTokens).
    Generate(ctx)
```

### Stop Sequences

Anthropic uses `stop_sequences` instead of `stop`. The SDK automatically renames this parameter:

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Count from 1 to 10").
    Stop([]string{"\n\n"}).
    Generate(ctx)
```

### Tool Calling

Anthropic supports tool/function calling for structured output and function execution:

```go
tools := []types.Tool{
    {
        Type: "function",
        Function: &types.ToolFunction{
            Name:        "get_weather",
            Description: "Get the current weather for a location",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]any{
                        "type":        "string",
                        "description": "The city and state, e.g. San Francisco, CA",
                    },
                },
                "required": []string{"location"},
            },
        },
    },
}

response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("What's the weather in Tokyo?").
    Tools(tools).
    Generate(ctx)

if len(response.ToolCalls) > 0 {
    // Handle tool call
    toolCall := response.ToolCalls[0]
    fmt.Printf("Tool: %s, Args: %s\n", toolCall.Function.Name, toolCall.Function.Arguments)
}
```

### Structured Output

Use the `Structured()` method for type-safe structured responses:

```go
type Person struct {
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
}

var result Person
err := client.Structured().
    Model("claude-sonnet-4-5").
    Prompt("Extract: John is 30 years old and can be reached at john@example.com").
    SchemaName("person").
    GenerateAs(ctx, &result)

fmt.Printf("%+v\n", result)
// Output: {Name:John Age:30 Email:john@example.com}
```

### Streaming

Stream responses in real-time:

```go
chunks, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Tell me a short story").
    Stream(ctx)

if err != nil {
    panic(err)
}

for chunk := range chunks {
    if chunk.Delta != nil {
        fmt.Print(chunk.Delta.Content)
    }
}
```

## Error Handling

The provider returns typed errors for common issues:

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Hello").
    Generate(ctx)

if err != nil {
    var apiErr *types.ProviderError
    if errors.As(err, &apiErr) {
        fmt.Printf("Provider error: %s\n", apiErr.Message)
        fmt.Printf("Error type: %s\n", apiErr.Type)
    }
    return
}
```

## Headers

The SDK automatically adds required Anthropic headers:

- `anthropic-version: 2023-06-01`
- `x-api-key: <your-api-key>`

You can add custom headers via `ProviderConfig.Headers` if needed:

```go
client := wormhole.New(
    wormhole.WithProviderConfig("anthropic", types.ProviderConfig{
        APIKey: os.Getenv("ANTHROPIC_API_KEY"),
        Headers: map[string]string{
            "anthropic-beta": "prompt-caching-2024-07-31", // Enable beta features
        },
    }),
)
```

## Unsupported Features

The following features are not supported by Anthropic and will return `NotImplementedError`:

- `Embeddings()` - Use a dedicated embeddings provider
- `Audio()` - Audio transcription/generation
- `Images()` - Image generation

## Configuration Options

### Base URL

Override the default Anthropic API endpoint:

```go
client := wormhole.New(
    wormhole.WithProviderConfig("anthropic", types.ProviderConfig{
        APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
        BaseURL: "https://custom-api.example.com/v1",
    }),
)
```

### Provider Options

Pass Anthropic-specific options via `ProviderOptions`:

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Hello").
    ProviderOptions(map[string]any{
        "top_k": 40,
    }).
    Generate(ctx)
```

## Reference

- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)
- [Claude Models](https://docs.anthropic.com/en/docs/about-claude/models)
- [Tool Use](https://docs.anthropic.com/en/docs/tool-use)

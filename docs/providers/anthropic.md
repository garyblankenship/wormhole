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

    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Create a new client with Anthropic configuration
    client := wormhole.New(
        wormhole.WithDefaultProvider("anthropic"),
        wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
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

    fmt.Println(response.Content())
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

## Models

Anthropic's catalog and aliases change independently of Wormhole. Use the
[official Claude models reference](https://platform.claude.com/docs/en/about-claude/models/overview)
for current IDs and lifecycle status. Prefer a stable alias, such as
`claude-sonnet-4-5`, when you do not need a dated snapshot.

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
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Summarize this text").
    MaxTokens(1024).
    Generate(ctx)
```

### Stop Sequences

Anthropic uses `stop_sequences` instead of `stop`. The SDK automatically renames this parameter:

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Count from 1 to 10").
    Stop([]string{"\n\n"}...).
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
    Tools(tools...).
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
    fmt.Print(chunk.Content())
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
    var apiErr *types.WormholeError
    if errors.As(err, &apiErr) {
        fmt.Printf("Provider error: %s\n", apiErr.Message)
        fmt.Printf("Error code: %s\n", apiErr.Code)
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
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY"), types.ProviderConfig{
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
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY"), types.ProviderConfig{
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
- [Claude Models](https://platform.claude.com/docs/en/about-claude/models/overview)
- [Tool Use](https://docs.anthropic.com/en/docs/tool-use)

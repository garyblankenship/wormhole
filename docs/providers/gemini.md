# Gemini Provider

The Gemini provider provides access to Google's Gemini models via the Generative Language API.

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
    // Create a new client with Gemini configuration
    client := wormhole.New(
        wormhole.WithDefaultProvider("gemini"),
        wormhole.WithProviderConfig("gemini", types.ProviderConfig{
            APIKey: os.Getenv("GEMINI_API_KEY"),
        }),
    )

    ctx := context.Background()

    // Simple text generation
    response, err := client.Text().
        Model("gemini-2.5-flash").
        Prompt("Hello, Gemini!").
        Generate(ctx)

    if err != nil {
        panic(err)
    }

    fmt.Println(response.Text)
}
```

### Direct Provider Initialization

For more control, you can create the Gemini provider directly:

```go
import "github.com/garyblankenship/wormhole/pkg/providers/gemini"

provider := gemini.New(os.Getenv("GEMINI_API_KEY"), types.ProviderConfig{
    // BaseURL is optional; defaults to https://generativelanguage.googleapis.com/v1beta
})
```

## Supported Models

### Gemini 3 Series (Latest)

| Model ID | Description |
|----------|-------------|
| `gemini-3-pro-preview` | Latest Gemini 3 Pro (preview) |
| `gemini-3-pro-image-preview` | Gemini 3 Pro with vision (preview) |

### Gemini 2.5 Series

| Model ID | Description |
|----------|-------------|
| `gemini-2.5-flash` | Flash 2.5 (stable) |
| `gemini-2.5-flash-preview-09-2025` | Flash 2.5 (Sept 2025 preview) |
| `gemini-2.5-flash-image` | Flash 2.5 with vision |
| `gemini-2.5-flash-native-audio-preview-12-2025` | Flash 2.5 with audio (Dec) |
| `gemini-2.5-flash-native-audio-preview-09-2025` | Flash 2.5 with audio (Sept) |
| `gemini-2.5-flash-preview-tts` | Flash 2.5 text-to-speech (preview) |
| `gemini-2.5-flash-lite` | Flash 2.5 Lite |
| `gemini-2.5-flash-lite-preview-09-2025` | Flash 2.5 Lite (Sept 2025) |
| `gemini-2.5-pro` | Gemini 2.5 Pro |
| `gemini-2.5-pro-preview-tts` | Gemini 2.5 Pro with TTS (preview) |

### Gemini 2.0 Series

| Model ID | Description |
|----------|-------------|
| `gemini-2.0-flash` | Flash 2.0 |
| `gemini-2.0-flash-001` | Flash 2.0 (v001) |
| `gemini-2.0-flash-exp` | Flash 2.0 (experimental) |
| `gemini-2.0-flash-preview-image-generation` | Flash 2.0 with image generation |
| `gemini-2.0-flash-lite` | Flash 2.0 Lite |
| `gemini-2.0-flash-lite-001` | Flash 2.0 Lite (v001) |

**Note**: Prefer stable versions (e.g., `gemini-2.5-flash`) over preview/exp versions for production use.

## Capabilities

The Gemini provider supports the following capabilities:

- `CapabilityText` - Text generation
- `CapabilityChat` - Chat/completion
- `CapabilityStructured` - Structured output (JSON schema)
- `CapabilityStream` - Streaming responses
- `CapabilityFunctions` - Function/tool calling
- `CapabilityEmbeddings` - Text embeddings

## Gemini-Specific Features

### API Key Authentication

Gemini uses API key authentication via URL query parameter instead of Authorization headers. The SDK handles this automatically:

```go
provider := gemini.New("your-api-key-here", types.ProviderConfig{})
```

The API key is appended to every request: `?key=your-api-key-here`

### System Instructions

Gemini supports system instructions via the `systemInstruction` field:

```go
response, err := client.Text().
    Model("gemini-2.5-flash").
    SystemPrompt("You are a helpful assistant specializing in Go programming.").
    Prompt("Explain goroutines").
    Generate(ctx)
```

### Multimodal Input

Gemini supports image and audio input alongside text:

```go
imageData, _ := os.ReadFile("image.jpg")

response, err := client.Text().
    Model("gemini-2.5-flash-image").
    Prompt("What do you see in this image?").
    Media([]types.Media{
        &types.ImageMedia{
            MimeType: "image/jpeg",
            Data:     imageData,
        },
    }).
    Generate(ctx)
```

### Tool Calling

Gemini supports function calling for structured output and function execution:

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
                    "units": map[string]any{
                        "type":        "string",
                        "enum":        []string{"celsius", "fahrenheit"},
                        "description": "The units for temperature",
                    },
                },
                "required": []string{"location"},
            },
        },
    },
}

response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("What's the weather in Tokyo?").
    Tools(tools).
    ToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAuto}).
    Generate(ctx)

if len(response.ToolCalls) > 0 {
    // Handle tool call
    toolCall := response.ToolCalls[0]
    fmt.Printf("Tool: %s, Args: %v\n", toolCall.Name, toolCall.Arguments)
}
```

### Structured Output

Use the `Structured()` method for type-safe JSON responses:

```go
type Person struct {
    Name  string   `json:"name"`
    Age   int      `json:"age"`
    Email string   `json:"email"`
    Skills []string `json:"skills"`
}

var result Person
err := client.Structured().
    Model("gemini-2.5-flash").
    Prompt("Extract: John is 30 years old, can be reached at john@example.com, and knows Go and Python").
    SchemaName("person").
    GenerateAs(ctx, &result)

fmt.Printf("%+v\n", result)
// Output: {Name:John Age:30 Email:john@example.com Skills:[Go Python]}
```

### Embeddings

Generate embeddings for semantic search and similarity:

```go
response, err := client.Embeddings().
    Model("text-embedding-004").
    Input([]string{
        "Hello world",
        "Natural language processing",
        "Machine learning embeddings",
    }).
    ProviderOptions(map[string]any{
        "taskType": "SEMANTIC_SIMILARITY",
        "title":    "Document similarity analysis",
    }).
    Generate(ctx)

for _, embedding := range response.Embeddings {
    fmt.Printf("Index %d: %d dimensions\n", embedding.Index, len(embedding.Embedding))
}
```

### Streaming

Stream responses in real-time:

```go
chunks, err := client.Text().
    Model("gemini-2.5-flash").
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

## Generation Config

Gemini uses `generationConfig` for parameters. The SDK automatically maps standard parameters:

| Standard Parameter | Gemini Parameter |
|--------------------|------------------|
| `max_tokens` | `maxOutputTokens` |
| `temperature` | `temperature` |
| `top_p` | `topP` |
| `stop` | `stopSequences` |

Example:

```go
maxTokens := 1024
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Summarize this text").
    MaxTokens(&maxTokens).
    Temperature(0.7).
    TopP(0.9).
    Stop([]string{"\n\n"}).
    Generate(ctx)
```

## Error Handling

The provider returns typed errors for common issues:

```go
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Hello").
    Generate(ctx)

if err != nil {
    var apiErr *types.WormholeError
    if errors.As(err, &apiErr) {
        fmt.Printf("Provider: %s\n", apiErr.Provider)
        fmt.Printf("Status: %d\n", apiErr.StatusCode)
        fmt.Printf("Message: %s\n", apiErr.Message)
    }
    return
}
```

Common error status codes:
- `401` - Invalid API key
- `429` - Rate limit exceeded
- `400` - Invalid request (model not found, payload too large)
- `500` - Internal server error

## Configuration Options

### Base URL

Override the default Gemini API endpoint:

```go
client := wormhole.New(
    wormhole.WithProviderConfig("gemini", types.ProviderConfig{
        APIKey:  os.Getenv("GEMINI_API_KEY"),
        BaseURL: "https://custom-api.example.com/v1beta",
    }),
)
```

### Timeout

Set request timeout (in seconds):

```go
client := wormhole.New(
    wormhole.WithProviderConfig("gemini", types.ProviderConfig{
        APIKey:  os.Getenv("GEMINI_API_KEY"),
        Timeout: 30, // 30 seconds
    }),
)
```

## Unsupported Features

The following features are not supported by Gemini and will return `NotImplementedError`:

- `Audio()` - Audio transcription (use native audio models instead)
- `Images()` - Image generation (use `gemini-2.0-flash-preview-image-generation` for text-to-image)

## Provider Options

Pass Gemini-specific options via `ProviderOptions`:

```go
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Hello").
    ProviderOptions(map[string]any{
        // Add any provider-specific options here
    }).
    Generate(ctx)
```

## Reference

- [Google Gemini API](https://ai.google.dev/gemini-api/docs)
- [Gemini Models](https://ai.google.dev/gemini-api/docs/models)
- [Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)
- [Embeddings](https://ai.google.dev/gemini-api/docs/models/text-embedding)

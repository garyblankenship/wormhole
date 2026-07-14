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

    "github.com/garyblankenship/wormhole/v2"
)

func main() {
    // Create a new client with Gemini configuration
    client := wormhole.New(
        wormhole.WithDefaultProvider("gemini"),
        wormhole.WithGemini(os.Getenv("GEMINI_API_KEY")),
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

    fmt.Println(response.Content())
}
```

### Direct Provider Initialization

For more control, you can create the Gemini provider directly:

```go
import "github.com/garyblankenship/wormhole/v2/providers/gemini"

provider := gemini.New(os.Getenv("GEMINI_API_KEY"), types.ProviderConfig{
    // BaseURL is optional; defaults to https://generativelanguage.googleapis.com/v1beta
})
```

## Models

Google's model catalog and preview lifecycle change independently of Wormhole.
Use the [official Gemini models reference](https://ai.google.dev/gemini-api/docs/models)
for current IDs and availability. Prefer a stable model ID, such as
`gemini-2.5-flash`, when a preview-specific feature is not required.

## Capabilities

The Gemini provider supports the following capabilities:

- `CapabilityText` - Text generation
- `CapabilityChat` - Chat/completion
- `CapabilityStructured` - Structured output (JSON schema)
- `CapabilityStream` - Streaming responses
- `CapabilityFunctions` - Function/tool calling
- `CapabilityEmbeddings` - Text embeddings
- `CapabilityImages` - Text-to-image generation through Gemini `generateContent`

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

Gemini supports inline image input alongside text:

```go
imageData, _ := os.ReadFile("image.jpg")

response, err := client.Text().
    Model("gemini-2.5-flash-image").
    Messages(&types.UserMessage{
        Content: "What do you see in this image?",
        Media: []types.Media{
            &types.ImageMedia{
                MimeType: "image/jpeg",
                Data:     imageData,
            },
        },
    }).
    Generate(ctx)
```

Gemini native requests require inline bytes or base64 data. URL-only images are
rejected by the Gemini provider.

### Image Generation

Use `Image()` with a Gemini image-capable model for text-to-image generation:

```go
img, err := client.Image().
    Using("gemini").
    Model("gemini-2.5-flash-image").
    Prompt("A clean isometric diagram of a Go service gateway").
    Generate(ctx)
if err != nil {
    panic(err)
}

fmt.Println(img.Images[0].B64JSON) // base64 image data from Gemini inlineData
```

Gemini image requests are sent to
`/models/{model}:generateContent?key=...` with
`generationConfig.responseModalities` set to `["TEXT", "IMAGE"]`.
Provider-specific image options can be passed with `ProviderOptions`:

```go
img, err := client.Image().
    Using("gemini").
    Model("gemini-2.5-flash-image").
    Prompt("A square app icon for a provider gateway").
    ProviderOptions(map[string]any{
        "aspect_ratio": "1:1",
        "image_size":   "2K",
        "generationConfig": map[string]any{
            "candidateCount": 1,
        },
    }).
    Generate(ctx)
```

Reference images are sent as inline Gemini `inlineData` parts. Import
`github.com/garyblankenship/wormhole/v2/providers/gemini` for the image input
type:

```go
img, err := client.Image().
    Using("gemini").
    Model("gemini-2.5-flash-image").
    Prompt("Make this product photo look like a watercolor illustration").
    ProviderOptions(map[string]any{
        "images": []gemini.ImageInput{
            {Data: imageBytes, MimeType: "image/png"},
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
    Tools(tools...).
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

For text requests that need Gemini JSON mode without a full schema, pass the
native generation config through `ProviderOptions`. Gemini expects JSON control
inside `generationConfig`, not as top-level OpenAI `response_format`:

```go
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Return exactly one JSON object with key ok=true.").
    MaxTokens(64).
    ProviderOptions(map[string]any{
        "generationConfig": map[string]any{
            "responseMimeType": "application/json",
        },
    }).
    Generate(ctx)
```

### Embeddings

Generate embeddings for semantic search and similarity:

```go
response, err := client.Embeddings().
    Model("gemini-embedding-001").
    Input([]string{
        "Hello world",
        "Natural language processing",
        "Machine learning embeddings",
    }...).
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
    fmt.Print(chunk.Content())
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
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Summarize this text").
    MaxTokens(1024).
    Temperature(0.7).
    TopP(0.9).
    Stop([]string{"\n\n"}...).
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
    wormhole.WithGemini(os.Getenv("GEMINI_API_KEY"), types.ProviderConfig{
        BaseURL: "https://custom-api.example.com/v1beta",
    }),
)
```

### Timeout

Set request timeout (in seconds):

```go
client := wormhole.New(
    wormhole.WithGemini(os.Getenv("GEMINI_API_KEY"), types.ProviderConfig{
        Timeout: 30, // 30 seconds
    }),
)
```

## Unsupported Features

The following features are not supported by Gemini and will return `NotImplementedError`:

- `Audio()` - Audio transcription (use native audio models instead)

## Provider Options

Pass Gemini-specific options via `ProviderOptions`. Text requests merge
`generationConfig` with the SDK-generated Gemini config, so options such as
`responseMimeType` preserve standard settings like `maxOutputTokens`,
`temperature`, `topP`, and `stopSequences`. Other text options are merged into
the Gemini request body. Image requests merge `generationConfig` with the SDK's
required image response modalities and pass other Gemini fields through:

```go
response, err := client.Text().
    Model("gemini-2.5-flash").
    Prompt("Hello").
    ProviderOptions(map[string]any{
        "generationConfig": map[string]any{
            "responseMimeType": "application/json",
        },
    }).
    Generate(ctx)
```

### Thinking Tokens

Gemini responses may report hidden thinking as `usageMetadata.thoughtsTokenCount`.
Wormhole includes those tokens in `response.Usage.CompletionTokens` and also
breaks them out as `response.Usage.ReasoningTokens`. These tokens are part of the
model's output budget even when `response.Text` is empty; a `length` finish reason
can therefore mean the request spent its output budget on thinking before
assistant-visible text was produced. Use a larger `MaxTokens` budget or Gemini
`generationConfig.thinkingConfig.thinkingBudget` when a task needs both thinking
room and structured visible output.

## Reference

- [Google Gemini API](https://ai.google.dev/gemini-api/docs)
- [Gemini Image Generation](https://ai.google.dev/gemini-api/docs/image-generation)
- [Gemini Models](https://ai.google.dev/gemini-api/docs/models)
- [Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)
- [Embeddings](https://ai.google.dev/gemini-api/docs/models/text-embedding)

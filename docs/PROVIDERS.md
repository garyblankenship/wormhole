# Prism Go Providers

This document outlines the available LLM providers in Prism Go and their capabilities.

## Available Providers

### ✅ Gemini (Google)
- **Package**: `github.com/prism-php/prism-go/pkg/providers/gemini`
- **Models**: `gemini-1.5-pro`, `gemini-1.5-flash`, `text-embedding-004`
- **Features**:
  - ✅ Text generation with system prompts
  - ✅ Streaming responses
  - ✅ Structured output (JSON schema)
  - ✅ Embeddings generation
  - ✅ Tool calling
  - ✅ Multi-modal input (images, documents)
  - ✅ Search grounding and citations
  - ❌ Audio (TTS/STT)
  - ❌ Image generation

### ✅ Groq
- **Package**: `github.com/prism-php/prism-go/pkg/providers/groq`
- **Models**: `llama3-8b-8192`, `llama3-70b-8192`, `mixtral-8x7b-32768`
- **Features**:
  - ✅ Text generation
  - ✅ Streaming responses
  - ✅ Structured output (JSON mode)
  - ✅ Speech-to-text (Whisper)
  - ✅ Tool calling
  - ✅ Multi-modal image input
  - ❌ Embeddings
  - ❌ Text-to-speech
  - ❌ Image generation

### 🚧 OpenAI (Updated for new types)
- **Package**: `github.com/prism-php/prism-go/pkg/providers/openai`
- **Status**: Being updated for new type system
- **Models**: `gpt-5`, `gpt-5-mini`, `text-embedding-ada-002`, `dall-e-3`

### 🚧 Anthropic (Updated for new types)
- **Package**: `github.com/prism-php/prism-go/pkg/providers/anthropic`
- **Status**: Being updated for new type system
- **Models**: `claude-3-5-sonnet-20241022`, `claude-3-opus-20240229`

### 📋 Planned Providers
- **Mistral AI**: Text generation, embeddings
- **Ollama**: Local model support
- **OpenRouter**: Multi-provider routing
- **xAI**: Grok models

## Usage Examples

### Basic Text Generation

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/prism-php/prism-go/pkg/providers/gemini"
    "github.com/prism-php/prism-go/pkg/types"
)

func main() {
    ctx := context.Background()
    
    // Initialize provider
    provider := gemini.New("your-api-key", types.ProviderConfig{})
    
    // Create request
    request := types.TextRequest{
        Model: "gemini-1.5-flash",
        Messages: []types.Message{
            types.NewUserMessage("What is Go programming language?"),
        },
        MaxTokens: 200,
    }
    
    // Generate response
    response, err := provider.Text(ctx, request)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(response.Text)
}
```

### Streaming Responses

```go
stream, err := provider.Stream(ctx, request)
if err != nil {
    log.Fatal(err)
}

for chunk := range stream {
    if chunk.Error != nil {
        log.Printf("Error: %v", chunk.Error)
        break
    }
    
    if chunk.Text != "" {
        fmt.Print(chunk.Text)
    }
    
    if chunk.FinishReason != nil {
        fmt.Printf("\\n[Finished: %s]\\n", *chunk.FinishReason)
        break
    }
}
```

### Structured Output

```go
import "github.com/prism-php/prism-go/pkg/types"

// Define schema
schema := &types.ObjectSchema{
    BaseSchema: types.BaseSchema{Type: "object"},
    Properties: map[string]types.Schema{
        "name": &types.StringSchema{
            BaseSchema: types.BaseSchema{Type: "string"},
        },
        "age": &types.NumberSchema{
            BaseSchema: types.BaseSchema{Type: "number"},
        },
    },
    Required: []string{"name", "age"},
}

request := types.StructuredRequest{
    Model: "gemini-1.5-flash",
    Messages: []types.Message{
        types.NewUserMessage("Generate a person with name and age"),
    },
    Schema: schema,
}

response, err := provider.Structured(ctx, request)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Structured data: %+v\\n", response.Data)
```

### Tool Calling

```go
// Define a tool
tool := types.NewTool(
    "get_weather",
    "Get current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]interface{}{
                "type": "string",
                "description": "City name",
            },
        },
        "required": []string{"location"},
    },
)

request := types.TextRequest{
    Model: "gemini-1.5-flash",
    Messages: []types.Message{
        types.NewUserMessage("What's the weather in Paris?"),
    },
    Tools: []types.Tool{*tool},
    ToolChoice: &types.ToolChoice{
        Type: types.ToolChoiceTypeAuto,
    },
}

response, err := provider.Text(ctx, request)
if err != nil {
    log.Fatal(err)
}

// Check for tool calls
for _, toolCall := range response.ToolCalls {
    fmt.Printf("Tool called: %s with args: %+v\\n", toolCall.Name, toolCall.Arguments)
}
```

### Multi-modal Input

```go
// Load image data
imageData, err := os.ReadFile("image.jpg")
if err != nil {
    log.Fatal(err)
}

// Create user message with image
userMsg := &types.UserMessage{
    Content: "What do you see in this image?",
    Media: []types.Media{
        &types.ImageMedia{
            Data:     imageData,
            MimeType: "image/jpeg",
        },
    },
}

request := types.TextRequest{
    Model:    "gemini-1.5-flash",
    Messages: []types.Message{userMsg},
}

response, err := provider.Text(ctx, request)
if err != nil {
    log.Fatal(err)
}

fmt.Println(response.Text)
```

### Embeddings

```go
request := types.EmbeddingsRequest{
    Model: "text-embedding-004",
    Input: []string{
        "The quick brown fox jumps over the lazy dog",
        "Machine learning is a subset of artificial intelligence",
    },
}

response, err := provider.Embeddings(ctx, request)
if err != nil {
    log.Fatal(err)
}

for i, embedding := range response.Embeddings {
    fmt.Printf("Embedding %d: %d dimensions\\n", i, len(embedding.Embedding))
}
```

## Provider Configuration

### Basic Configuration

```go
config := types.ProviderConfig{
    BaseURL:    "https://custom-endpoint.com/v1", // Override default URL
    Headers:    map[string]string{
        "Custom-Header": "value",
    },
    Timeout:    30, // seconds
    MaxRetries: 3,
    RetryDelay: 1, // seconds
}

provider := gemini.New("api-key", config)
```

### Provider-Specific Options

Use `ProviderOptions` in requests for provider-specific parameters:

```go
request := types.TextRequest{
    Model:    "gemini-1.5-flash",
    Messages: messages,
    ProviderOptions: map[string]interface{}{
        "taskType": "SEMANTIC_SIMILARITY", // Gemini-specific
        "title":    "Document Title",      // Gemini-specific
    },
}
```

## Error Handling

All providers return structured errors:

```go
response, err := provider.Text(ctx, request)
if err != nil {
    if prismErr, ok := err.(types.PrismError); ok {
        fmt.Printf("Provider error: %s (code: %s)\\n", prismErr.Message, prismErr.Code)
    } else {
        fmt.Printf("Generic error: %v\\n", err)
    }
    return
}
```

## Model Support Matrix

| Provider | Text | Stream | Structured | Embeddings | Tools | Audio | Images | Multi-modal |
|----------|------|--------|------------|------------|-------|-------|--------|-------------|
| Gemini   | ✅   | ✅     | ✅         | ✅         | ✅    | ❌    | ❌     | ✅          |
| Groq     | ✅   | ✅     | ✅         | ❌         | ✅    | 🔄    | ❌     | ✅          |
| OpenAI*  | ✅   | ✅     | ✅         | ✅         | ✅    | ✅    | ✅     | ✅          |
| Anthropic* | ✅   | ✅     | ✅         | ❌         | ✅    | ❌    | ❌     | ✅          |

*Currently being updated for new type system

Legend:
- ✅ Fully implemented
- 🔄 Partially implemented 
- ❌ Not supported by provider
- 📋 Planned

## Best Practices

1. **Always handle errors** - Network requests can fail
2. **Use context with timeout** - Prevent hanging requests
3. **Validate schemas** - For structured output
4. **Handle streaming properly** - Check for errors in chunks
5. **Respect rate limits** - Implement backoff strategies
6. **Use appropriate models** - Match model capabilities to use case

## Contributing

To add a new provider:

1. Create package in `pkg/providers/yourprovider/`
2. Implement the `types.Provider` interface
3. Add transformation functions for requests/responses
4. Add tests and examples
5. Update this documentation

See existing providers for reference implementation patterns.
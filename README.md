# Prism Go

A powerful Go package for integrating Large Language Models (LLMs) into your applications. Inspired by the PHP Laravel package, Prism Go provides a unified interface for working with multiple LLM providers.

## Features

- 🚀 **Unified API** - Single interface for multiple LLM providers
- 🔄 **Streaming Support** - Real-time streaming responses with Go channels
- 🏗️ **Builder Pattern** - Intuitive request building with method chaining
- 📊 **Structured Output** - Type-safe structured responses with JSON schema
- 🎯 **Multiple Modalities** - Support for text, embeddings, images, and audio
- 🔧 **Tool/Function Calling** - Enable LLMs to call functions
- ⚡ **Concurrent Safe** - Built for concurrent Go applications

## Installation

```bash
go get github.com/prism-php/prism-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/prism-php/prism-go/pkg/prism"
    "github.com/prism-php/prism-go/pkg/types"
)

func main() {
    // Initialize Prism with configuration
    p := prism.New(prism.Config{
        DefaultProvider: "openai",
        Providers: map[string]types.ProviderConfig{
            "openai": {
                APIKey: "your-api-key",
            },
            "anthropic": {
                APIKey: "your-api-key",
            },
        },
    })
    
    // Simple text generation
    response, err := p.Text().
        Model("gpt-4").
        Prompt("Write a haiku about Go programming").
        Temperature(0.7).
        Generate(context.Background())
    
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(response.Text)
}
```

## Examples

### Text Generation with Messages

```go
messages := []types.Message{
    types.NewSystemMessage("You are a helpful assistant"),
    types.NewUserMessage("What is the capital of France?"),
}

response, err := p.Text().
    Using("anthropic").
    Model("claude-3-opus-20240229").
    Messages(messages...).
    MaxTokens(100).
    Generate(context.Background())
```

### Streaming Responses

```go
chunks, err := p.Text().
    Model("gpt-4").
    Prompt("Tell me a story").
    Stream(context.Background())

if err != nil {
    log.Fatal(err)
}

for chunk := range chunks {
    if chunk.Error != nil {
        log.Fatal(chunk.Error)
    }
    fmt.Print(chunk.Delta)
}
```

### Structured Output

```go
type Person struct {
    Name    string `json:"name"`
    Age     int    `json:"age"`
    City    string `json:"city"`
}

schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "name":  map[string]string{"type": "string"},
        "age":   map[string]string{"type": "integer"},
        "city":  map[string]string{"type": "string"},
    },
    "required": []string{"name", "age", "city"},
}

var person Person
err := p.Structured().
    Model("gpt-4").
    Prompt("Extract: John is 30 years old and lives in NYC").
    Schema(schema).
    GenerateAs(context.Background(), &person)

fmt.Printf("%+v\n", person)
```

### Tool/Function Calling

```go
weatherTool, _ := types.NewTool(
    "get_weather",
    "Get the current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]string{
                "type":        "string",
                "description": "City name",
            },
        },
        "required": []string{"location"},
    },
)

response, err := p.Text().
    Model("gpt-4").
    Prompt("What's the weather in Paris?").
    Tools(*weatherTool).
    Generate(context.Background())

// Check if the model wants to call a tool
if len(response.ToolCalls) > 0 {
    for _, call := range response.ToolCalls {
        fmt.Printf("Tool: %s, Args: %s\n", 
            call.Function.Name, 
            call.Function.Arguments)
    }
}
```

### Embeddings

```go
embeddings, err := p.Embeddings().
    Model("text-embedding-3-small").
    Input("Hello world", "Goodbye world").
    Dimensions(512).
    Generate(context.Background())

for i, embedding := range embeddings.Embeddings {
    fmt.Printf("Text %d has %d dimensions\n", 
        i, len(embedding.Embedding))
}
```

### Image Generation

```go
images, err := p.Image().
    Model("dall-e-3").
    Prompt("A serene landscape with mountains").
    Size("1024x1024").
    Quality("hd").
    Generate(context.Background())

fmt.Println("Generated image URL:", images.Images[0].URL)
```

### Audio Operations

```go
// Text to Speech
audio, err := p.Audio().TextToSpeech().
    Model("tts-1").
    Input("Hello, this is a test").
    Voice("alloy").
    Generate(context.Background())

// Save audio to file
os.WriteFile("output.mp3", audio.Audio, 0644)

// Speech to Text
audioData, _ := os.ReadFile("speech.mp3")
transcript, err := p.Audio().SpeechToText().
    Model("whisper-1").
    Audio(audioData, "mp3").
    Language("en").
    Transcribe(context.Background())

fmt.Println("Transcript:", transcript.Text)
```

## Supported Providers

| Provider | Text | Stream | Structured | Embeddings | Images | Audio | Tools |
|----------|------|--------|------------|------------|--------|-------|-------|
| OpenAI   | ✅   | ✅     | ✅         | ✅         | ✅     | ✅    | ✅    |
| Anthropic| ✅   | ✅     | ✅         | ❌         | ❌     | ❌    | ✅    |
| Gemini   | ✅   | ✅     | ✅         | ✅         | ❌     | ❌    | ✅    |
| Groq     | ✅   | ✅     | ✅         | ❌         | ❌     | ⚠️    | ✅    |
| Mistral  | ✅   | ✅     | ✅         | ✅         | ❌     | ⚠️    | ✅    |
| Ollama   | ✅   | ✅     | ✅         | ✅         | ❌     | ❌    | ❌    |
| **OpenAI-Compatible** | ✅   | ✅     | ✅         | ✅         | ⚠️     | ⚠️    | ✅    |

✅ Fully supported | ❌ Not supported by provider | ⚠️ Partial support

### OpenAI-Compatible Providers
The SDK includes support for **any OpenAI-compatible API**, including:
- **LMStudio** - Local model serving
- **vLLM** - High-performance inference server
- **Ollama** (via OpenAI API) - Local model management
- **Text Generation WebUI** - Popular local interface
- **FastChat** - Multi-model serving system
- **Any hosted service** with OpenAI-compatible endpoints

## Configuration

### Provider Configuration

```go
config := prism.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{
        "openai": {
            APIKey:  os.Getenv("OPENAI_API_KEY"),
            BaseURL: "https://api.openai.com/v1", // optional
            Timeout: 30, // seconds
            Headers: map[string]string{
                "Custom-Header": "value",
            },
        },
    },
}
```

### Using Multiple Providers

```go
// Initialize with multiple providers
p := prism.New()
p.WithOpenAI("your-openai-key")
p.WithAnthropic("your-anthropic-key")
p.WithGemini("your-gemini-key")
p.WithGroq("your-groq-key")
p.WithMistral(types.ProviderConfig{APIKey: "your-mistral-key"})
p.WithOllama(types.ProviderConfig{}) // Local, no key needed
p.WithLMStudio(types.ProviderConfig{}) // LMStudio local
p.WithVLLM(types.ProviderConfig{BaseURL: "http://localhost:8000/v1"}) // vLLM
p.WithOpenAICompatible("custom", "https://api.custom.com/v1", types.ProviderConfig{
    APIKey: "key-if-needed",
}) // Generic OpenAI-compatible

// Use different providers for different tasks
textResponse, _ := p.Text().
    Using("openai").
    Model("gpt-4").
    Prompt("Hello").
    Generate(ctx)

analysisResponse, _ := p.Text().
    Using("anthropic").
    Model("claude-3-opus-20240229").
    Prompt("Analyze this text: " + textResponse.Text).
    Generate(ctx)

embeddings, _ := p.Embeddings().
    Using("mistral").
    Model("mistral-embed").
    Input([]string{"Some text to embed"}).
    Generate(ctx)
```

## Advanced Usage

### Custom Provider Options

```go
response, err := p.Text().
    Model("gpt-4").
    Prompt("Hello").
    ProviderOptions(map[string]interface{}{
        "logprobs": true,
        "top_logprobs": 5,
    }).
    Generate(context.Background())
```

### Error Handling

```go
response, err := p.Text().
    Model("gpt-4").
    Prompt("Hello").
    Generate(context.Background())

if err != nil {
    var prismErr types.PrismError
    if errors.As(err, &prismErr) {
        fmt.Printf("API Error: %s (Code: %s)\n", 
            prismErr.Message, prismErr.Code)
    } else {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Context and Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

response, err := p.Text().
    Model("gpt-4").
    Prompt("Write a long story").
    Generate(ctx)
```

## Testing

The package includes comprehensive testing utilities:

```go
// TODO: Add testing examples
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Credits

Inspired by the [Prism PHP](https://github.com/prism-php/prism) package by TJ Miller.
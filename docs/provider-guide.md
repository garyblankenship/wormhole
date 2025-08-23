# Wormhole Go Providers

This document outlines the available LLM providers in Wormhole Go and their capabilities.

## Available Providers

### ✅ Gemini (Google)
- **Package**: `github.com/garyblankenship/wormhole/pkg/providers/gemini`
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
- **Implementation**: Uses OpenAI-compatible provider via `WithGroq()`
- **API Endpoint**: `https://api.groq.com/openai/v1`
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

### ✅ OpenAI
- **Package**: `github.com/garyblankenship/wormhole/pkg/providers/openai`
- **Status**: ✅ Fully updated for new type system
- **Models**: `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `text-embedding-3-small`, `text-embedding-3-large`, `dall-e-3`
- **Features**:
  - ✅ Text generation with system prompts
  - ✅ Streaming responses
  - ✅ Structured output (JSON mode and schema)
  - ✅ Embeddings generation
  - ✅ Function/Tool calling
  - ✅ Multi-modal input (vision)
  - ✅ Audio (Whisper STT, TTS)
  - ✅ Image generation (DALL-E)

### ✅ Anthropic
- **Package**: `github.com/garyblankenship/wormhole/pkg/providers/anthropic`
- **Status**: ✅ Fully updated for new type system
- **Models**: `claude-3-5-sonnet-20241022`, `claude-3-5-haiku-20241022`, `claude-3-opus-20240229`
- **Features**:
  - ✅ Text generation with system prompts
  - ✅ Streaming responses
  - ✅ Structured output (JSON mode)
  - ✅ Function/Tool calling
  - ✅ Multi-modal input (vision, documents)
  - ❌ Embeddings (use OpenAI or Gemini)
  - ❌ Audio (TTS/STT)
  - ❌ Image generation

### ✅ OpenRouter (Multi-Provider Gateway)
- **Package**: `github.com/garyblankenship/wormhole/pkg/providers/openai_compatible`
- **Setup**: Use as OpenAI-compatible provider with OpenRouter base URL
- **Models**: 200+ models from OpenAI, Anthropic, Google, Meta, Mistral, and more
- **Features**:
  - ✅ Text generation from 50+ providers
  - ✅ Streaming responses
  - ✅ Structured output (JSON mode)
  - ✅ Function calling
  - ✅ Embeddings (multiple providers)
  - ✅ Cost tracking and usage analytics
  - ✅ Fallback routing and load balancing
  - ✅ Pay-per-use with competitive pricing
  - ❌ Audio (varies by underlying provider)
  - ❌ Image generation (varies by underlying provider)

### ✅ Mistral AI
- **Implementation**: Uses OpenAI-compatible provider via `WithMistral()`
- **API Endpoint**: `https://api.mistral.ai/v1`
- **Models**: `mistral-large-latest`, `mistral-medium`, `mistral-small`, `mistral-embed`
- **Features**:
  - ✅ Text generation
  - ✅ Streaming responses
  - ✅ Structured output (JSON mode)
  - ✅ Embeddings generation
  - ✅ Tool calling
  - ❌ Audio (TTS/STT)
  - ❌ Image generation

### 📋 Planned Providers
- **Ollama**: Local model support
- **xAI**: Grok models

## Usage Examples

### OpenRouter: Multi-Provider Access

OpenRouter provides access to 200+ models from different providers through a single API. Perfect for model comparison, fallback strategies, and cost optimization.

#### Setup

```go
import (
    "os"
    
    "github.com/garyblankenship/wormhole/pkg/wormhole"
    "github.com/garyblankenship/wormhole/pkg/types"
)

// Configure as OpenAI-compatible provider with functional options
client := wormhole.New(
    wormhole.WithDefaultProvider("openrouter"),
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: os.Getenv("OPENROUTER_API_KEY"),
    }),
)
```

#### Multi-Model Text Generation

```go
models := []string{
    "openai/gpt-4o-mini",              // OpenAI via OpenRouter
    "anthropic/claude-3.5-sonnet",     // Anthropic via OpenRouter  
    "meta-llama/llama-3.1-8b-instruct", // Meta via OpenRouter
    "google/gemini-pro",               // Google via OpenRouter
    "mistralai/mixtral-8x7b-instruct", // Mistral via OpenRouter
}

for _, model := range models {
    response, err := client.Text().
        Model(model).
        Prompt("Explain quantum computing in one sentence").
        MaxTokens(100).
        Generate(ctx)
    
    if err != nil {
        log.Printf("Error with %s: %v", model, err)
        continue
    }
    
    fmt.Printf("Model: %s\nResponse: %s\n\n", model, response.Content)
}
```

#### Cost-Optimized Model Selection

```go
// Use cheaper models for simple tasks
cheapModels := []string{
    "openai/gpt-4o-mini",
    "anthropic/claude-3-haiku",
    "meta-llama/llama-3.1-8b-instruct",
}

// Use powerful models for complex tasks
premiumModels := []string{
    "openai/gpt-4o",
    "anthropic/claude-3.5-sonnet",
    "google/gemini-pro-1.5",
}

func generateResponse(prompt string, complexity string) (*types.TextResponse, error) {
    var models []string
    if complexity == "simple" {
        models = cheapModels
    } else {
        models = premiumModels
    }
    
    // Try models in order of preference/cost
    for _, model := range models {
        response, err := client.Text().
            Model(model).
            Prompt(prompt).
            Generate(ctx)
        
        if err == nil {
            return response, nil
        }
        
        log.Printf("Model %s failed, trying next: %v", model, err)
    }
    
    return nil, errors.New("all models failed")
}
```

#### Function Calling with Multiple Providers

```go
// Some models have better function calling than others
functionModels := []string{
    "openai/gpt-4o-mini",        // Excellent function calling
    "anthropic/claude-3.5-sonnet", // Good function calling
    "mistralai/mixtral-8x7b-instruct", // Basic function calling
}

weatherTool := types.NewTool(
    "get_weather",
    "Get current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]interface{}{
                "type": "string",
                "description": "City and state/country",
            },
        },
        "required": []string{"location"},
    },
)

// Try function calling with fallback
for _, model := range functionModels {
    response, err := client.Text().
        Model(model).
        Messages(types.NewUserMessage("What's the weather in Tokyo?")).
        Tools([]*types.Tool{weatherTool}).
        Generate(ctx)
    
    if err == nil && len(response.ToolCalls) > 0 {
        fmt.Printf("Success with %s: %d tool calls\n", model, len(response.ToolCalls))
        break
    }
}
```

#### Embeddings from Multiple Providers

```go
embeddingModels := []string{
    "openai/text-embedding-3-small",    // High quality, moderate cost
    "openai/text-embedding-ada-002",    // Lower cost option
    "voyage/voyage-large-2-instruct",   // Specialized for retrieval
}

text := "The universe is vast and full of possibilities"

for _, model := range embeddingModels {
    response, err := client.Embeddings().
        Model(model).
        Input(text).
        Generate(ctx)
    
    if err != nil {
        log.Printf("Embedding failed for %s: %v", model, err)
        continue
    }
    
    fmt.Printf("Model: %s, Dimensions: %d\n", 
        model, len(response.Embeddings[0]))
}
```

#### Streaming with Model Comparison

```go
func compareStreamingPerformance(prompt string) {
    models := []string{
        "openai/gpt-4o-mini",
        "anthropic/claude-3.5-sonnet", 
        "meta-llama/llama-3.1-8b-instruct",
    }
    
    for _, model := range models {
        fmt.Printf("\n--- Streaming with %s ---\n", model)
        
        start := time.Now()
        stream, err := client.Text().
            Model(model).
            Prompt(prompt).
            MaxTokens(200).
            Stream(ctx)
        
        if err != nil {
            fmt.Printf("Failed to start stream: %v\n", err)
            continue
        }
        
        tokenCount := 0
        for chunk := range stream {
            if chunk.Error != nil {
                fmt.Printf("Stream error: %v\n", chunk.Error)
                break
            }
            
            fmt.Print(chunk.Content)
            tokenCount++
        }
        
        duration := time.Since(start)
        fmt.Printf("\nTokens: %d, Time: %v\n", tokenCount, duration)
    }
}
```

### Basic Text Generation

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    ctx := context.Background()
    
    // Initialize client with Gemini provider
    client := wormhole.New(
        wormhole.WithDefaultProvider("gemini"),
        wormhole.WithGemini(os.Getenv("GEMINI_API_KEY")),
    )
    
    // Generate response using builder pattern
    response, err := client.Text().
        Model("gemini-1.5-flash").
        Prompt("What is Go programming language?").
        MaxTokens(200).
        Generate(ctx)
    
    if err != nil {
        log.Printf("Error generating text: %v", err)
        return
    }
    
    fmt.Println(response.Content)
}
```

### Streaming Responses

```go
stream, err := client.Text().
    Model("gemini-1.5-flash").
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
    
    if chunk.Content != "" {
        fmt.Print(chunk.Content)
    }
    
    if chunk.FinishReason != nil {
        fmt.Printf("\n[Finished: %s]\n", *chunk.FinishReason)
        break
    }
}
```

### Structured Output

```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

// Define schema using map format
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "name": map[string]interface{}{"type": "string"},
        "age":  map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 120},
    },
    "required": []string{"name", "age"},
    "additionalProperties": false,
}

var person Person
err := client.Structured().
    Model("gemini-1.5-flash").
    Prompt("Generate a realistic person with name and age").
    Schema(schema).
    GenerateAs(ctx, &person)

if err != nil {
    log.Printf("Error generating structured output: %v", err)
    return
}

fmt.Printf("Generated person: %+v\n", person)
```

### Tool Calling

```go
import "github.com/garyblankenship/wormhole/pkg/types"

// Define a tool using the NewTool helper
weatherTool := types.NewTool(
    "get_weather",
    "Get current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]interface{}{
                "type": "string",
                "description": "City name with country if needed",
            },
        },
        "required": []string{"location"},
    },
)

response, err := client.Text().
    Model("gemini-1.5-flash").
    Prompt("What's the weather like in Paris?").
    Tools([]*types.Tool{weatherTool}).
    Generate(ctx)

if err != nil {
    log.Printf("Error with tool calling: %v", err)
    return
}

// Check for tool calls in response
if len(response.ToolCalls) > 0 {
    for _, toolCall := range response.ToolCalls {
        fmt.Printf("Tool called: %s with args: %+v\n", toolCall.Name, toolCall.Arguments)
        // Here you would implement the actual weather API call
    }
} else {
    fmt.Printf("Response: %s\n", response.Content)
}
```

### Multi-modal Input

```go
import (
    "io"
    "os"
    
    "github.com/garyblankenship/wormhole/pkg/types"
)

// Load image from file
file, err := os.Open("image.jpg")
if err != nil {
    log.Printf("Error opening image: %v", err)
    return
}
defer file.Close()

imageData, err := io.ReadAll(file)
if err != nil {
    log.Printf("Error reading image: %v", err)
    return
}

// Create message with image
response, err := client.Text().
    Model("gemini-1.5-flash").
    UserMessage("What do you see in this image?").
    AddImage(imageData, "image/jpeg").
    Generate(ctx)

if err != nil {
    log.Printf("Error with multi-modal request: %v", err)
    return
}

fmt.Printf("Image description: %s\n", response.Content)
```

### Embeddings

```go
texts := []string{
    "The quick brown fox jumps over the lazy dog",
    "Machine learning is a subset of artificial intelligence",
    "Wormhole makes LLM integration simple and fast",
}

response, err := client.Embeddings().
    Model("text-embedding-004").
    Input(texts).
    Generate(ctx)

if err != nil {
    log.Printf("Error generating embeddings: %v", err)
    return
}

for i, embedding := range response.Embeddings {
    fmt.Printf("Text %d: %d dimensions, first 5 values: %v\n", 
        i+1, len(embedding.Embedding), embedding.Embedding[:5])
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
    if wormholeErr, ok := err.(types.WormholeProviderError); ok {
        fmt.Printf("Provider error: %s (code: %s)\\n", wormholeErr.Message, wormholeErr.Code)
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
| OpenRouter | ✅   | ✅     | ✅         | ✅         | ✅    | 🔄    | 🔄     | ✅          |
| OpenAI*  | ✅   | ✅     | ✅         | ✅         | ✅    | ✅    | ✅     | ✅          |
| Anthropic* | ✅   | ✅     | ✅         | ❌         | ✅    | ❌    | ❌     | ✅          |

*Currently being updated for new type system

Legend:
- ✅ Fully implemented
- 🔄 Partially implemented 
- ❌ Not supported by provider
- 📋 Planned

## Provider-Specific Setup Guide

### OpenAI Setup
```bash
# Get API key from https://platform.openai.com/api-keys
export OPENAI_API_KEY="sk-your-key-here"
```

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
)
```

### Anthropic Setup
```bash
# Get API key from https://console.anthropic.com/
export ANTHROPIC_API_KEY="sk-ant-your-key-here"
```

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("anthropic"),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
)
```

### Google Gemini Setup
```bash
# Get API key from https://makersuite.google.com/app/apikey
export GEMINI_API_KEY="your-key-here"
```

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("gemini"),
    wormhole.WithGemini(os.Getenv("GEMINI_API_KEY")),
)
```

### Groq Setup
```bash
# Get API key from https://console.groq.com/keys
export GROQ_API_KEY="gsk_your-key-here"
```

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("groq"),
    wormhole.WithGroq(os.Getenv("GROQ_API_KEY")),
)
```

### OpenRouter Setup
```bash
# Get API key from https://openrouter.ai/keys
export OPENROUTER_API_KEY="sk-or-your-key-here"
```

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openrouter"),
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: os.Getenv("OPENROUTER_API_KEY"),
    }),
)
```

## Common Provider Issues & Solutions

### API Key Problems
```bash
Error: 401 Unauthorized / Invalid API key
```

**Solutions**:
- Verify API key format (OpenAI: `sk-`, Anthropic: `sk-ant-`, etc.)
- Check environment variable is set: `echo $OPENAI_API_KEY`
- Ensure API key has correct permissions/credits
- For Anthropic: Verify you're using the correct console (Claude.ai vs API console)

### Model Not Available
```bash
Error: model "gpt-5" is not available
```

**Solutions**:
- Check model name spelling and availability
- OpenAI: Use `gpt-4o`, `gpt-4o-mini` instead of `gpt-5`
- Anthropic: Use `claude-3-5-sonnet-20241022` for latest models
- Verify your API key has access to the requested model

### Rate Limiting
```bash
Error: Rate limit exceeded
```

**Solutions**:
```go
// Add retry middleware
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithRetries(3, 2*time.Second),
    wormhole.WithMiddleware(
        middleware.RateLimitMiddleware(10), // 10 requests per second
    ),
)
```

### Network/Regional Issues
```bash
Error: dial tcp: connection refused
```

**Solutions**:
- Check internet connectivity
- For China/restricted regions: Use OpenRouter as proxy
- Verify corporate firewall allows HTTPS to provider domains
- Try different provider as fallback

### Structured Output Failures
```bash
Error: response is not valid JSON
```

**Solutions**:
- Use models that support structured output (GPT-4o, Claude-3.5-Sonnet, Gemini-1.5)
- Simplify your schema - complex nested structures may fail
- Add schema validation and retry logic
- Use Wormhole's built-in JSON parsing with error recovery

## Best Practices

1. **Always handle errors** - Network requests can fail
2. **Use context with timeout** - Prevent hanging requests
3. **Validate schemas** - For structured output
4. **Handle streaming properly** - Check for errors in chunks
5. **Respect rate limits** - Implement backoff strategies
6. **Use appropriate models** - Match model capabilities to use case
7. **Set up fallback providers** - For high availability
8. **Monitor API usage and costs** - Especially with premium models
9. **Use environment variables** - Never hardcode API keys
10. **Test with multiple providers** - Ensure compatibility

## Contributing

To add a new provider:

1. Create package in `pkg/providers/yourprovider/`
2. Implement the `types.Provider` interface
3. Add transformation functions for requests/responses
4. Add tests and examples
5. Update this documentation

See existing providers for reference implementation patterns.
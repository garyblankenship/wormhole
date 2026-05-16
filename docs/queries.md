# Wormhole SDK Query Patterns

## Request/Response Patterns

### Text Generation
**Request flow**:
```go
// Builder pattern
resp, err := client.Text().
    Model("gpt-5.2").
    Prompt("Hello, world!").
    Temperature(0.7).
    MaxTokens(100).
    Generate(ctx)

// Conversation pattern
conv := types.NewConversation().
    System("You are helpful").
    User("What is Go?").
    Assistant("Go is a programming language")
resp, err := client.Text().
    Conversation(conv).
    Generate(ctx)
```

**Response structure**:
```go
type TextResponse struct {
    Text         string
    Model        string
    Usage        *Usage
    FinishReason FinishReason
    ToolCalls    []ToolCall
    Metadata     map[string]any
}
```

Use `resp.Content()` when you want the response text through the common response accessor.

### Structured Output
**Schema definition**:
```go
schema := map[string]any{
    "type": "object",
    "properties": map[string]any{
        "name": map[string]any{"type": "string"},
        "age":  map[string]any{"type": "number"},
    },
    "required": []string{"name"},
}
```

**Request pattern**:
```go
var person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

err := client.Structured().
    Model("gpt-5.2").
    Schema(schema).
    Prompt("Describe a person").
    GenerateAs(ctx, &person)
```

### Embeddings
**Request pattern**:
```go
resp, err := client.Embeddings().
    Model("text-embedding-3-small").
    Input("Hello, world!").
    Generate(ctx)

// Batch embeddings
resp, err := client.Embeddings().
    Model("text-embedding-3-small").
    Input("text1", "text2").
    Generate(ctx)
```

**Response structure**:
```go
type EmbeddingsResponse struct {
    Embeddings []Embedding
    Model      string
    Usage      *Usage
}

type Embedding struct {
    Index     int
    Embedding []float64
}
```

### Image Generation
**Request pattern**:
```go
resp, err := client.Image().
    Model("dall-e-3").
    Prompt("A cat in space").
    Size("1024x1024").
    ResponseFormat("url").
    Generate(ctx)
```

The current image builder exposes text-to-image generation fields only: `Model`, `Prompt`, `Size`, `Quality`, `Style`, `N`, `ResponseFormat`, `Using`, `BaseURL`, and `Generate`.

### Audio Operations
**Speech-to-text**:
```go
resp, err := client.Audio().
    SpeechToText().
    Audio(audioData, "mp3").
    Model("whisper-1").
    Language("en").
    Transcribe(ctx)
```

**Text-to-speech**:
```go
resp, err := client.Audio().
    TextToSpeech().
    Input("Hello world").
    Model("tts-1").
    Voice("alloy").
    Generate(ctx)
```

## Provider-Specific Patterns

### OpenAI Compatibility
```go
// Standard OpenAI
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithDefaultProvider("openai"),
)

// OpenAI-compatible endpoints (e.g., local models)
resp, err := client.Text().
    Model("local-model").
    BaseURL("http://localhost:8080/v1").
    Prompt("Hello").
    Generate(ctx)
```

### Anthropic Claude
```go
client := wormhole.New(
    wormhole.WithAnthropic(apiKey),
    wormhole.WithDefaultProvider("anthropic"),
)

resp, err := client.Text().
    Model("claude-sonnet-4-5").
    Prompt("Write Go code").
    Generate(ctx)
```

### Google Gemini
```go
client := wormhole.New(
    wormhole.WithGemini(apiKey),
    wormhole.WithDefaultProvider("gemini"),
)

resp, err := client.Text().
    Model("gemini-2.5-pro").
    Prompt("Explain Go interfaces").
    Generate(ctx)
```

The current text builder does not expose a first-class image prompt helper. Use text requests for Gemini through `Text()`, and keep multimodal inputs behind provider-specific extensions until a first-class builder method exists.

### Ollama (Local)
```go
client := wormhole.New(
    wormhole.WithOllama(types.ProviderConfig{BaseURL: "http://localhost:11434"}),
    wormhole.WithDefaultProvider("ollama"),
)

resp, err := client.Text().
    Model("llama3.2").
    Prompt("Hello").
    Generate(ctx)
```

## Batch Processing Patterns

### Simple Batch
```go
results := client.Batch().
    Add(client.Text().Model("gpt-5.2").Prompt("Q1")).
    Add(client.Text().Model("gpt-5.2").Prompt("Q2")).
    Concurrency(5).
    Execute(ctx)
```

### Batch with Error Collection
```go
responses, errors := client.Batch().
    AddAll(requests...).
    ExecuteCollect(ctx)
```

### Race Multiple Models
```go
// First successful response wins
resp, err := client.Batch().
    Add(client.Text().Model("gpt-5.2").Prompt("Q")).
    Add(client.Text().Using("anthropic").Model("claude-sonnet-4-5").Prompt("Q")).
    Add(client.Text().Using("gemini").Model("gemini-2.5-pro").Prompt("Q")).
    ExecuteFirst(ctx)
```

## Tool Calling Patterns

### Tool Registration
```go
client.RegisterTool(
    "get_weather",
    "Get current weather for a city",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string"},
            "unit": map[string]any{
                "type": "string",
                "enum": []string{"celsius", "fahrenheit"},
            },
        },
        "required": []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        city := args["city"].(string)
        return map[string]any{"city": city, "temp": 72, "condition": "sunny"}, nil
    },
)
```

### Tool-Enabled Text Generation
```go
resp, err := client.Text().
    Model("gpt-5.2").
    Prompt("What's the weather in Paris?").
    WithToolsEnabled().
    Generate(ctx)

// resp.ToolCalls contains any tool calls from the final response.
```

### Multi-Iteration Tool Execution
```go
resp, err := client.Text().
    Model("gpt-5.2").
    Prompt("Plan a vacation").
    WithToolsEnabled().
    WithMaxToolIterations(5). // Up to 5 rounds of tool calling
    Generate(ctx)
```

## Streaming Patterns

### Text Streaming
```go
ch, err := client.Text().
    Model("gpt-5.2").
    Prompt("Tell a story").
    Stream(ctx)

for chunk := range ch {
    if chunk.HasError() {
        // Handle error
        break
    }
    fmt.Print(chunk.Content())
    if chunk.IsDone() {
        // Stream complete
        break
    }
}
```

For applications that need both real-time output and final text, use `StreamAndAccumulate(ctx)`:

```go
chunks, getText, err := client.Text().
    Model("gpt-5.2").
    Prompt("Tell a story").
    StreamAndAccumulate(ctx)

for chunk := range chunks {
    fmt.Print(chunk.Content())
}

fullText := getText()
```

## Error Handling Patterns

### Structured Error Checking
```go
resp, err := client.Text().Model("gpt-5.2").Prompt("Hello").Generate(ctx)
if err != nil {
    if wormholeErr, ok := err.(*types.WormholeError); ok {
        switch wormholeErr.Code {
        case types.ErrorCodeAuth:
            // Handle authentication error
        case types.ErrorCodeRateLimit:
            // Handle rate limit
        case types.ErrorCodeTimeout:
            // Handle timeout
        default:
            // Handle other errors
        }
    }
}
```

### Retry Configuration
```go
openAIConfig := types.NewProviderConfig(apiKey).
    WithRetries(3, 500*time.Millisecond).
    WithMaxRetryDelay(5*time.Second)

client := wormhole.New(
    wormhole.WithOpenAI(apiKey, openAIConfig),
)
```

### Fallback Models
```go
resp, err := client.Text().
    Model("gpt-5.2").
    WithFallback("gpt-5.1-mini", "gpt-5").
    Prompt("Important question").
    Generate(ctx)
```

## Configuration Patterns

### Multi-Provider Setup
```go
client := wormhole.New(
    wormhole.WithOpenAI(openaiKey),
    wormhole.WithAnthropic(anthropicKey),
    wormhole.WithGemini(geminiKey),
    wormhole.WithDefaultProvider("openai"),
)
```

### Provider-Specific Configuration
```go
openAIConfig := types.NewProviderConfig(openaiKey).
    WithTimeoutDuration(30*time.Second).
    WithRetries(3, 1*time.Second)

anthropicConfig := types.NewProviderConfig(anthropicKey).
    WithTimeoutDuration(60*time.Second)

client := wormhole.New(
    wormhole.WithOpenAI(openaiKey, openAIConfig),
    wormhole.WithAnthropic(anthropicKey, anthropicConfig),
)
```

### Middleware Configuration
```go
metrics := middleware.NewMetrics()

client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithMiddleware(
        middleware.LoggingMiddleware(logger),
        middleware.MetricsMiddleware(metrics),
        middleware.TimeoutMiddleware(30*time.Second),
    ),
)
```

### TLS Configuration
```go
openAIConfig := types.NewProviderConfig(apiKey).
    WithTLSConfigParam("min_version", tls.VersionTLS12).
    WithTLSConfigParam("cipher_suites", []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256})

client := wormhole.New(
    wormhole.WithOpenAI(apiKey, openAIConfig),
)
```

## Performance Patterns

### Request Pooling
```go
// Reuse builder configuration
baseBuilder := client.Text().
    Model("gpt-5.2").
    Temperature(0.7)

resp1, _ := baseBuilder.Clone().Prompt("Q1").Generate(ctx)
resp2, _ := baseBuilder.Clone().Prompt("Q2").Generate(ctx)
```

### Caching Patterns
```go
cache := middleware.NewMemoryCache(1000)

client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithMiddleware(middleware.CacheMiddleware(middleware.CacheConfig{
        Cache: cache,
        TTL:   5 * time.Minute,
    })),
)
```

### Provider Parameters
```go
openAIConfig := types.NewProviderConfig(apiKey).
    WithParam("max_idle_conns", 100).
    WithParam("idle_conn_timeout", 90*time.Second)

client := wormhole.New(
    wormhole.WithOpenAI(apiKey, openAIConfig),
)
```

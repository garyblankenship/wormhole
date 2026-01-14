# Wormhole SDK Query Patterns

## Request/Response Patterns

### Text Generation
**Request flow**:
```go
// Builder pattern
resp, err := client.Text().
    Model("gpt-4o").
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
    Model         string
    Usage         *Usage
    FinishReason  string
    ToolCalls     []ToolCall  // For function calling
    ProviderInfo  map[string]any
}
```

### Structured Output
**Schema definition**:
```go
schema := types.ObjectSchema{
    Type: "object",
    Properties: map[string]types.Schema{
        "name": types.StringSchema{Type: "string"},
        "age":  types.NumberSchema{Type: "number"},
    },
    Required: []string{"name"},
}
```

**Request pattern**:
```go
var person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

resp, err := client.Structured().
    Model("gpt-4o").
    Schema(schema).
    Prompt("Describe a person").
    GenerateInto(ctx, &person)
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
    Inputs([]string{"text1", "text2"}).
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
**Request patterns**:
```go
// Text-to-image
resp, err := client.Image().
    Model("dall-e-3").
    Prompt("A cat in space").
    Size("1024x1024").
    Generate(ctx)

// Image-to-image (variations/edits)
resp, err := client.Image().
    Model("dall-e-3").
    Image(base64Image).
    Prompt("Add glasses to the cat").
    Generate(ctx)
```

### Audio Operations
**Speech-to-text**:
```go
resp, err := client.Audio().
    SpeechToText(audioData).
    Model("whisper-1").
    Language("en").
    Generate(ctx)
```

**Text-to-speech**:
```go
resp, err := client.Audio().
    TextToSpeech("Hello world").
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

// Claude-specific features (tool calling, code execution)
resp, err := client.Text().
    Model("claude-3-5-sonnet").
    Prompt("Write Go code").
    Generate(ctx)
```

### Google Gemini
```go
client := wormhole.New(
    wormhole.WithGemini(apiKey),
    wormhole.WithDefaultProvider("gemini"),
)

// Gemini multimodal support
resp, err := client.Text().
    Model("gemini-1.5-pro").
    PromptWithImage("Describe this image", imageBytes).
    Generate(ctx)
```

### Ollama (Local)
```go
client := wormhole.New(
    wormhole.WithOllama("http://localhost:11434"),
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
    Add(client.Text().Model("gpt-4o").Prompt("Q1")).
    Add(client.Text().Model("gpt-4o").Prompt("Q2")).
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
    Add(client.Text().Model("gpt-4o").Prompt("Q")).
    Add(client.Text().Model("claude-3-5-sonnet").Prompt("Q")).
    Add(client.Text().Model("gemini-1.5-pro").Prompt("Q")).
    ExecuteFirst(ctx)
```

## Tool Calling Patterns

### Tool Registration
```go
client.RegisterTool(
    "get_weather",
    "Get current weather for a city",
    types.ObjectSchema{
        Type: "object",
        Properties: map[string]types.Schema{
            "city": types.StringSchema{Type: "string"},
            "unit": types.StringSchema{
                Type: "string",
                Enum: []string{"celsius", "fahrenheit"},
            },
        },
        Required: []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        city := args["city"].(string)
        return map[string]any{"temp": 72, "condition": "sunny"}, nil
    },
)
```

### Tool-Enabled Text Generation
```go
resp, err := client.Text().
    Model("gpt-4o").
    Prompt("What's the weather in Paris?").
    EnableToolExecution().
    Generate(ctx)

// resp.ToolCalls contains executed tool results
```

### Multi-Iteration Tool Execution
```go
resp, err := client.Text().
    Model("gpt-4o").
    Prompt("Plan a vacation").
    EnableToolExecution().
    MaxToolIterations(5). // Up to 5 rounds of tool calling
    Generate(ctx)
```

## Streaming Patterns

### Text Streaming
```go
ch, err := client.Text().
    Model("gpt-4o").
    Prompt("Tell a story").
    Stream(ctx)

for chunk := range ch {
    if chunk.Error != nil {
        // Handle error
        break
    }
    fmt.Print(chunk.Text)
    if chunk.Done {
        // Stream complete
        break
    }
}
```

### Structured Streaming
```go
ch, err := client.Structured().
    Model("gpt-4o").
    Schema(schema).
    Prompt("Generate structured data").
    Stream(ctx)

for chunk := range ch {
    // Partial structured data
    processPartial(chunk.Data)
}
```

## Error Handling Patterns

### Structured Error Checking
```go
resp, err := client.Text().Prompt("Hello").Generate(ctx)
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
client := wormhole.New(
    wormhole.WithOpenAI(apiKey).
        WithRetries(3, 500*time.Millisecond).
        WithMaxRetryDelay(5*time.Second),
)
```

### Fallback Models
```go
resp, err := client.Text().
    Model("gpt-4o").
    FallbackModels("gpt-4o-mini", "gpt-3.5-turbo").
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
client := wormhole.New(
    wormhole.WithOpenAI(openaiKey).
        WithTimeout(30*time.Second).
        WithRetries(3, 1*time.Second),
    wormhole.WithAnthropic(anthropicKey).
        WithTimeout(60*time.Second),
)
```

### Middleware Configuration
```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithLoggingMiddleware(logger),
    wormhole.WithMetricsMiddleware(metrics),
    wormhole.WithTimeoutMiddleware(30*time.Second),
)
```

### TLS Configuration
```go
client := wormhole.New(
    wormhole.WithOpenAI(apiKey).
        WithTLSConfigParam("min_version", tls.VersionTLS12).
        WithTLSConfigParam("cipher_suites", []uint16{...}),
)
```

## Performance Patterns

### Request Pooling
```go
// Reuse builder configuration
baseBuilder := client.Text().
    Model("gpt-4o").
    Temperature(0.7)

resp1, _ := baseBuilder.Clone().Prompt("Q1").Generate(ctx)
resp2, _ := baseBuilder.Clone().Prompt("Q2").Generate(ctx)
```

### Caching Patterns
```go
// Enable response caching via middleware
cache := NewCache(5*time.Minute)
client := wormhole.New(
    wormhole.WithOpenAI(apiKey),
    wormhole.WithCacheMiddleware(cache),
)
```

### Connection Pooling
```go
// Configure HTTP client with connection pool
client := wormhole.New(
    wormhole.WithOpenAI(apiKey).
        WithParam("max_idle_conns", 100).
        WithParam("idle_conn_timeout", 90*time.Second),
)
```

# Prism Go Examples & Getting Started Guide

Comprehensive examples demonstrating all features of Prism Go, the fastest Go SDK for LLM integration.

## 🚀 Quick Setup

### 1. Installation
```bash
go get github.com/prism-php/prism-go
```

### 2. Environment Configuration
```bash
# Set API keys for providers you want to use
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export GEMINI_API_KEY="your-gemini-key"
export GROQ_API_KEY="your-groq-key"
export MISTRAL_API_KEY="your-mistral-key"
# Ollama and LMStudio don't require API keys for local usage
```

### 3. Your First Request
```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/prism-php/prism-go/pkg/prism"
)

func main() {
    // Ultra-fast initialization (67 ns overhead)
    client := prism.SimpleFactory().
        WithOpenAI("your-api-key").
        Build()
    
    // Fluent API with sub-microsecond performance
    response, err := client.Text().
        Model("gpt-5").
        Prompt("Hello, Prism Go!").
        Generate(context.Background())
    
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Response:", response.Text)
}
```

## 📚 Core Examples

### Basic Text Generation
```go
// Simple prompt-based generation
response, err := client.Text().
    Model("gpt-5").
    Prompt("Write a haiku about Go programming").
    Temperature(0.7).
    MaxTokens(100).
    Generate(ctx)

if err != nil {
    log.Fatal(err)
}

fmt.Println(response.Text)
```

### Conversation with Messages
```go
messages := []types.Message{
    types.NewSystemMessage("You are a helpful coding assistant"),
    types.NewUserMessage("How do I optimize Go performance?"),
}

response, err := client.Text().
    Model("gpt-5").
    Messages(messages...).
    MaxTokens(500).
    Generate(ctx)

fmt.Println("Assistant:", response.Text)
```

### Streaming Responses
```go
stream, err := client.Text().
    Model("gpt-5").
    Prompt("Tell me a long story about a Go developer").
    Stream(ctx)

if err != nil {
    log.Fatal(err)
}

fmt.Print("Story: ")
for chunk := range stream {
    if chunk.Error != nil {
        log.Printf("Stream error: %v", chunk.Error)
        continue
    }
    
    if chunk.Delta != nil {
        fmt.Print(chunk.Delta.Content)
    }
    
    // Check for completion
    if chunk.FinishReason != nil {
        fmt.Printf("\n[Finished: %s]\n", *chunk.FinishReason)
        break
    }
}
```

## 🏗️ Advanced Features

### Structured Output with JSON Schema
```go
// Define your expected structure
type PersonAnalysis struct {
    Name       string   `json:"name"`
    Age        int      `json:"age"`
    Skills     []string `json:"skills"`
    Confidence float64  `json:"confidence"`
}

// Define JSON schema
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "name": map[string]string{
            "type": "string",
            "description": "Person's full name",
        },
        "age": map[string]string{
            "type": "integer",
            "description": "Person's age in years",
        },
        "skills": map[string]interface{}{
            "type": "array",
            "items": map[string]string{"type": "string"},
            "description": "List of technical skills",
        },
        "confidence": map[string]string{
            "type": "number",
            "description": "Confidence score 0-1",
        },
    },
    "required": []string{"name", "age", "skills", "confidence"},
}

// Generate structured output
var analysis PersonAnalysis
err := client.Structured().
    Model("gpt-5").
    Prompt("Analyze this resume: John Doe, 30, software engineer with Go, Python, Docker experience").
    Schema(schema).
    GenerateAs(ctx, &analysis)

if err != nil {
    log.Fatal(err)
}

fmt.Printf("Parsed: %+v\n", analysis)
fmt.Printf("Skills: %v (Confidence: %.2f)\n", analysis.Skills, analysis.Confidence)
```

### Tool/Function Calling
```go
// Define a weather tool
weatherTool := types.Tool{
    Type: "function",
    Function: &types.ToolFunction{
        Name: "get_weather",
        Description: "Get current weather for a location",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "location": map[string]interface{}{
                    "type": "string",
                    "description": "City name (e.g., 'New York')",
                },
                "units": map[string]interface{}{
                    "type": "string",
                    "enum": []string{"celsius", "fahrenheit"},
                    "description": "Temperature units",
                },
            },
            "required": []string{"location"},
        },
    },
}

// Make request with tools
response, err := client.Text().
    Model("gpt-5").
    Prompt("What's the weather like in Tokyo? Use Celsius.").
    Tools(weatherTool).
    Generate(ctx)

if err != nil {
    log.Fatal(err)
}

// Handle tool calls
fmt.Println("Assistant:", response.Text)
if len(response.ToolCalls) > 0 {
    for _, call := range response.ToolCalls {
        fmt.Printf("Tool Call: %s\n", call.Function.Name)
        fmt.Printf("Arguments: %s\n", call.Function.Arguments)
        
        // In real implementation, you'd call your actual weather API
        weatherResult := `{"temperature": 22, "condition": "sunny", "units": "celsius"}`
        
        // Continue conversation with tool result
        followUp, err := client.Text().
            Model("gpt-5").
            Messages(
                types.NewUserMessage("What's the weather like in Tokyo? Use Celsius."),
                types.NewAssistantMessage(response.Text, response.ToolCalls),
                types.NewToolMessage(call.ID, weatherResult),
            ).
            Generate(ctx)
        
        if err == nil {
            fmt.Printf("Final Response: %s\n", followUp.Text)
        }
    }
}
```

### Embeddings Generation
```go
// Generate embeddings for semantic search
texts := []string{
    "Go is a programming language developed by Google",
    "Python is popular for data science and AI",
    "JavaScript runs in web browsers and Node.js",
    "Rust focuses on memory safety and performance",
}

embeddings, err := client.Embeddings().
    Model("text-embedding-3-small").
    Input(texts...).
    Dimensions(512). // Optional: reduce dimensions for storage efficiency
    Generate(ctx)

if err != nil {
    log.Fatal(err)
}

fmt.Printf("Generated %d embeddings:\n", len(embeddings.Embeddings))
for i, emb := range embeddings.Embeddings {
    fmt.Printf("Text %d: %d dimensions, first values: %.4f, %.4f, %.4f...\n",
        i, len(emb.Embedding), 
        emb.Embedding[0], emb.Embedding[1], emb.Embedding[2])
}

// Calculate similarity (dot product example)
if len(embeddings.Embeddings) >= 2 {
    similarity := dotProduct(embeddings.Embeddings[0].Embedding, embeddings.Embeddings[1].Embedding)
    fmt.Printf("Similarity between Go and Python texts: %.4f\n", similarity)
}
```

### Audio Processing
```go
// Text-to-Speech
tts, err := client.Audio().TextToSpeech().
    Model("tts-1").
    Input("Hello from Prism Go! This is a test of text-to-speech.").
    Voice("alloy").
    ResponseFormat("mp3").
    Generate(ctx)

if err != nil {
    log.Fatal(err)
}

// Save audio file
err = os.WriteFile("output.mp3", tts.Audio, 0644)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Audio saved to output.mp3")

// Speech-to-Text
audioData, err := os.ReadFile("input.mp3")
if err != nil {
    log.Fatal(err)
}

transcript, err := client.Audio().SpeechToText().
    Model("whisper-1").
    Audio(audioData, "mp3").
    Language("en").
    ResponseFormat("text").
    Transcribe(ctx)

if err != nil {
    log.Fatal(err)
}

fmt.Printf("Transcript: %s\n", transcript.Text)
```

### Image Generation
```go
// Generate images with DALL-E
images, err := client.Image().
    Model("dall-e-3").
    Prompt("A serene mountain landscape with a crystal clear lake, painted in the style of Bob Ross").
    Size("1024x1024").
    Quality("hd").
    Style("vivid").
    Generate(ctx)

if err != nil {
    log.Fatal(err)
}

if len(images.Images) > 0 {
    fmt.Printf("Generated image URL: %s\n", images.Images[0].URL)
    
    // If you requested base64 format instead:
    if images.Images[0].B64JSON != "" {
        // Decode base64 and save to file
        imageData, _ := base64.StdEncoding.DecodeString(images.Images[0].B64JSON)
        os.WriteFile("generated_image.png", imageData, 0644)
        fmt.Println("Image saved as generated_image.png")
    }
}
```

## 🚀 Production Examples

### Multi-Provider Setup
```go
// Configure multiple providers for different use cases
client := prism.SimpleFactory().
    WithOpenAI("openai-key").                    // For general tasks
    WithAnthropic("anthropic-key").              // For analysis and reasoning
    WithGroq("groq-key").                        // For ultra-fast responses
    WithMistral("mistral-key").                  // For embeddings
    WithOllama(types.ProviderConfig{}).          // For local/private models
    WithLMStudio(types.ProviderConfig{}).        // For custom local models
    Build()

// Route different tasks to optimal providers
func processUserQuery(query string) {
    // Fast classification with Groq
    classification, _ := client.Text().
        Using("groq").
        Model("mixtral-8x7b-32768").
        Prompt("Classify this query type: " + query).
        MaxTokens(50).
        Generate(ctx)
    
    // Detailed analysis with Anthropic
    if strings.Contains(classification.Text, "analysis") {
        analysis, _ := client.Text().
            Using("anthropic").
            Model("claude-3-opus-20240229").
            Prompt("Provide detailed analysis: " + query).
            MaxTokens(1000).
            Generate(ctx)
        
        fmt.Println("Analysis:", analysis.Text)
    }
    
    // Generate embeddings for semantic storage
    embedding, _ := client.Embeddings().
        Using("mistral").
        Model("mistral-embed").
        Input(query).
        Generate(ctx)
    
    // Store embedding for future similarity search
    storeEmbedding(query, embedding.Embeddings[0].Embedding)
}
```

### Production Middleware Stack
```go
// Enterprise configuration with full reliability features
client := prism.SimpleFactory().
    WithOpenAI("openai-key").
    WithMiddleware(
        "circuit-breaker",  // Prevent cascade failures
        "rate-limiter",     // Control traffic flow
        "retry",            // Automatic retry with exponential backoff
        "timeout",          // Request timeout enforcement
        "metrics",          // Performance monitoring
        "health-check",     // Provider health monitoring
        "logging",          // Structured request/response logging
    ).
    WithLoadBalancing("adaptive").      // Smart load balancing
    WithCaching("memory", "5m").        // Response caching
    WithFailover([]string{"openai", "anthropic", "groq"}).  // Automatic failover
    Build()

// This configuration adds measured overhead but provides enterprise reliability
response, err := client.Text().
    Model("gpt-5").
    Prompt("Process this critical request").
    Generate(ctx)

// Access comprehensive metrics
metrics := client.Metrics()
log.Printf("Total requests: %d, Error rate: %.2f%%, Avg latency: %v",
    metrics.TotalRequests,
    float64(metrics.TotalErrors)/float64(metrics.TotalRequests)*100,
    metrics.AverageLatency)
```

### High-Performance Concurrent Processing
```go
func processBulkData(items []string) {
    // Use minimal configuration for maximum speed (67ns overhead)
    fastClient := prism.New(prism.Config{
        DefaultProvider: "openai",
        Providers: map[string]types.ProviderConfig{
            "openai": {APIKey: os.Getenv("OPENAI_API_KEY")},
        },
    })
    
    // Process up to 10,000 requests/second
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, 100) // Limit concurrent requests
    
    for _, item := range items {
        wg.Add(1)
        go func(text string) {
            defer wg.Done()
            semaphore <- struct{}{} // Acquire
            defer func() { <-semaphore }() // Release
            
            // Ultra-fast processing with minimal overhead
            result, err := fastClient.Text().
                Model("gpt-5-mini").
                Prompt("Analyze: " + text).
                MaxTokens(100).
                Generate(context.Background())
            
            if err != nil {
                log.Printf("Error processing item: %v", err)
                return
            }
            
            // Process result
            handleResult(text, result.Text)
        }(item)
    }
    
    wg.Wait()
    fmt.Printf("Processed %d items\n", len(items))
}
```

## 🧪 Testing & Development

### Mock Provider for Testing
```go
func TestMyLLMFeature(t *testing.T) {
    // Create mock client for testing
    client := prism.NewWithMockProvider(prism.MockConfig{
        TextResponse: "This is a mocked response for testing",
        Latency:      time.Millisecond * 10, // Simulate network delay
    })
    
    // Your code under test
    result, err := myLLMFeature(client, "test input")
    
    // Assertions
    assert.NoError(t, err)
    assert.Contains(t, result, "mocked response")
}

func myLLMFeature(client *prism.Prism, input string) (string, error) {
    response, err := client.Text().
        Model("gpt-5").
        Prompt("Process: " + input).
        Generate(context.Background())
    
    if err != nil {
        return "", err
    }
    
    return response.Text, nil
}
```

### Performance Benchmarking
```go
func BenchmarkMyLLMIntegration(b *testing.B) {
    client := setupTestClient()
    ctx := context.Background()
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        _, err := client.Text().
            Model("gpt-5").
            Prompt("Test prompt").
            Generate(ctx)
        
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## 🔧 Error Handling & Recovery

### Comprehensive Error Handling
```go
func robustLLMCall(client *prism.Prism, prompt string) (string, error) {
    response, err := client.Text().
        Model("gpt-5").
        Prompt(prompt).
        Generate(context.Background())
    
    if err != nil {
        var prismErr *types.PrismError
        if errors.As(err, &prismErr) {
            switch prismErr.Code {
            case "rate_limit_exceeded":
                log.Printf("Rate limited, waiting %d seconds", prismErr.RetryAfter)
                time.Sleep(time.Duration(prismErr.RetryAfter) * time.Second)
                return robustLLMCall(client, prompt) // Retry
                
            case "model_overloaded":
                log.Println("Model overloaded, trying backup provider")
                return tryWithBackupProvider(client, prompt)
                
            case "context_length_exceeded":
                log.Println("Context too long, truncating")
                return robustLLMCall(client, truncatePrompt(prompt))
                
            case "content_filter":
                return "", fmt.Errorf("content was filtered: %s", prismErr.Message)
                
            default:
                return "", fmt.Errorf("API error [%s]: %s", prismErr.Code, prismErr.Message)
            }
        }
        
        // Handle network/system errors
        if errors.Is(err, context.DeadlineExceeded) {
            return "", fmt.Errorf("request timed out")
        }
        
        return "", fmt.Errorf("unexpected error: %w", err)
    }
    
    return response.Text, nil
}

func tryWithBackupProvider(client *prism.Prism, prompt string) (string, error) {
    // Try with a different provider
    response, err := client.Text().
        Using("anthropic").
        Model("claude-3-sonnet-20240229").
        Prompt(prompt).
        Generate(context.Background())
    
    if err != nil {
        return "", fmt.Errorf("backup provider also failed: %w", err)
    }
    
    return response.Text, nil
}
```

### Context and Cancellation
```go
func cancellableLLMRequest(prompt string) (string, error) {
    // Create cancellable context
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Set up cancellation handling
    done := make(chan struct{})
    var result string
    var err error
    
    go func() {
        defer close(done)
        response, e := client.Text().
            Model("gpt-5").
            Prompt(prompt).
            Generate(ctx)
        
        if e != nil {
            err = e
            return
        }
        result = response.Text
    }()
    
    select {
    case <-done:
        return result, err
    case <-ctx.Done():
        return "", fmt.Errorf("request cancelled: %w", ctx.Err())
    }
}
```

## 📊 Performance Optimization

### Benchmark Your Setup
```bash
# Run built-in performance benchmarks
go test -bench=. -benchmem ./pkg/prism/

# Profile memory usage
go test -bench=BenchmarkTextRequestBuilder -memprofile=mem.prof ./pkg/prism/
go tool pprof mem.prof

# Profile CPU usage
go test -bench=BenchmarkConcurrentRequests -cpuprofile=cpu.prof ./pkg/prism/
go tool pprof cpu.prof

# Test different concurrency levels
go test -bench=BenchmarkConcurrentRequests -cpu=1,2,4,8,16 ./pkg/prism/
```

### Custom Performance Testing
```go
func measureRequestLatency() {
    client := prism.New(prism.Config{
        DefaultProvider: "openai",
        Providers: map[string]types.ProviderConfig{
            "openai": {APIKey: os.Getenv("OPENAI_API_KEY")},
        },
    })
    
    // Warm up
    for i := 0; i < 10; i++ {
        client.Text().Model("gpt-5").Prompt("warmup").Generate(context.Background())
    }
    
    // Measure
    const numRequests = 100
    start := time.Now()
    
    for i := 0; i < numRequests; i++ {
        _, err := client.Text().
            Model("gpt-5").
            Prompt("fast test").
            MaxTokens(10).
            Generate(context.Background())
        
        if err != nil {
            log.Printf("Request %d failed: %v", i, err)
        }
    }
    
    duration := time.Since(start)
    avgLatency := duration / numRequests
    
    fmt.Printf("Average latency: %v (%d requests in %v)\n", avgLatency, numRequests, duration)
    fmt.Printf("Requests per second: %.2f\n", float64(numRequests)/duration.Seconds())
}
```

## 🎯 Use Case Examples

### Chatbot Implementation
```go
type ChatBot struct {
    client   *prism.Prism
    history  []types.Message
}

func NewChatBot() *ChatBot {
    client := prism.SimpleFactory().
        WithOpenAI(os.Getenv("OPENAI_API_KEY")).
        WithMiddleware("retry", "timeout").
        Build()
    
    return &ChatBot{
        client: client,
        history: []types.Message{
            types.NewSystemMessage("You are a helpful assistant. Be concise but informative."),
        },
    }
}

func (c *ChatBot) Chat(userMessage string) (string, error) {
    // Add user message to history
    c.history = append(c.history, types.NewUserMessage(userMessage))
    
    // Get response
    response, err := c.client.Text().
        Model("gpt-5").
        Messages(c.history...).
        MaxTokens(500).
        Temperature(0.7).
        Generate(context.Background())
    
    if err != nil {
        return "", err
    }
    
    // Add assistant response to history
    c.history = append(c.history, types.NewAssistantMessage(response.Text, nil))
    
    // Keep conversation history manageable
    if len(c.history) > 20 {
        c.history = append(c.history[:1], c.history[3:]...) // Keep system message + recent messages
    }
    
    return response.Text, nil
}

// Usage
func main() {
    bot := NewChatBot()
    
    response1, _ := bot.Chat("What is Go programming language?")
    fmt.Println("Bot:", response1)
    
    response2, _ := bot.Chat("Can you give me a simple example?")
    fmt.Println("Bot:", response2)
}
```

### Document Analysis Pipeline
```go
func analyzeDocument(filePath string) (*DocumentAnalysis, error) {
    // Read document
    content, err := os.ReadFile(filePath)
    if err != nil {
        return nil, err
    }
    
    client := prism.SimpleFactory().
        WithOpenAI(os.Getenv("OPENAI_API_KEY")).
        WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")).
        Build()
    
    // Step 1: Extract key information with structured output
    type KeyInfo struct {
        Title    string   `json:"title"`
        Authors  []string `json:"authors"`
        KeyTerms []string `json:"key_terms"`
        Summary  string   `json:"summary"`
    }
    
    var info KeyInfo
    err = client.Structured().
        Using("openai").
        Model("gpt-5").
        Prompt("Extract key information from this document: " + string(content)).
        Schema(generateSchema(KeyInfo{})).
        GenerateAs(context.Background(), &info)
    
    if err != nil {
        return nil, err
    }
    
    // Step 2: Deep analysis with Anthropic
    analysis, err := client.Text().
        Using("anthropic").
        Model("claude-3-opus-20240229").
        Prompt(fmt.Sprintf(`Provide detailed analysis of this document:
        
        Title: %s
        Authors: %v
        Summary: %s
        
        Focus on:
        1. Main arguments and evidence
        2. Methodology if applicable
        3. Strengths and limitations
        4. Implications and significance
        `, info.Title, info.Authors, info.Summary)).
        MaxTokens(1500).
        Generate(context.Background())
    
    if err != nil {
        return nil, err
    }
    
    // Step 3: Generate embeddings for similarity search
    embeddings, err := client.Embeddings().
        Using("openai").
        Model("text-embedding-3-small").
        Input(info.Summary).
        Generate(context.Background())
    
    if err != nil {
        return nil, err
    }
    
    return &DocumentAnalysis{
        KeyInfo:     info,
        Analysis:    analysis.Text,
        Embedding:   embeddings.Embeddings[0].Embedding,
        ProcessedAt: time.Now(),
    }, nil
}

type DocumentAnalysis struct {
    KeyInfo     KeyInfo   `json:"key_info"`
    Analysis    string    `json:"analysis"`
    Embedding   []float64 `json:"embedding"`
    ProcessedAt time.Time `json:"processed_at"`
}
```

## 🚀 Running the Examples

### Basic Examples
```bash
# Simple text generation (no API calls)
go run ../cmd/simple/main.go

# Full feature demonstration
go run ../cmd/example/main.go

# Middleware demonstration
go run middleware_example/main.go

# Provider-specific examples
go run openai_example/main.go
go run anthropic_example/main.go
go run local_models_example/main.go
```

### Custom Examples
Create your own example:

1. **Create a new directory**: `mkdir my_example && cd my_example`
2. **Initialize Go module**: `go mod init my_example`
3. **Add Prism dependency**: `go get github.com/prism-php/prism-go`
4. **Create main.go** with your example code
5. **Run**: `go run main.go`

### Environment Setup
```bash
# Copy example environment file
cp .env.example .env

# Edit with your API keys
nano .env

# Source environment variables
source .env

# Or use direnv for automatic loading
echo "source .env" > .envrc
direnv allow
```

## 📖 Additional Resources

- **[Main Documentation](../README.md)** - Complete feature overview
- **[Performance Guide](../PERFORMANCE.md)** - Detailed performance analysis
- **[API Reference](https://pkg.go.dev/github.com/prism-php/prism-go)** - Full API documentation
- **[Provider Docs](../docs/PROVIDERS.md)** - Provider-specific information
- **[Architecture](../docs/ARCHITECTURE.md)** - System design overview

## 🤝 Contributing Examples

We welcome new examples! Please:

1. **Follow existing patterns** - Use consistent structure and error handling
2. **Add comprehensive comments** - Explain what each part does
3. **Include error handling** - Show proper error management
4. **Test your examples** - Ensure they work with current API keys
5. **Update this README** - Add your example to the appropriate section

---

**Ready to build with the fastest LLM SDK?** Start with these examples and experience sub-microsecond performance! 🚀
# Knowledge

**User guide, features, operations, and troubleshooting for Wormhole SDK**

---

## What Wormhole Can Do Today

Wormhole is a production-ready Go SDK providing a unified interface to multiple LLM providers. Core strengths:

- **Multi-provider support**: OpenAI, Anthropic, Gemini, OpenRouter, Groq, Mistral, Ollama
- **Performance**: 67ns core overhead (165x faster than alternatives)
- **Enterprise middleware**: Circuit breakers, rate limiting, retry logic
- **Thread-safe**: Concurrent operations without race conditions
- **Clean API**: Functional options + builder pattern

### Provider Capabilities

| Provider | Text | Streaming | Structured | Embeddings | Notes |
|----------|------|-----------|------------|------------|-------|
| **OpenAI** | ✅ | ✅ | ✅ | ✅ | GPT-4, GPT-5, full feature set |
| **Anthropic** | ✅ | ✅ | ✅ | ❌ | Claude 3.5 Sonnet/Opus |
| **Gemini** | ✅ | ✅ | ✅ | ✅ | Google's AI models |
| **OpenRouter** | ✅ | ✅ | ✅ | ✅ | 200+ models, unified access |
| **Groq** | ✅ | ✅ | ✅ | ❌ | Fast inference, OpenAI-compatible |
| **Mistral** | ✅ | ✅ | ✅ | ✅ | European AI, OpenAI-compatible |
| **Ollama** | ✅ | ✅ | ✅ | ✅ | Local models, no API key needed |

### Provider-Specific Notes

**Groq Limitations:**
- No embeddings support
- No image generation, TTS, STT

**Mistral Limitations:**
- No image generation, TTS
- Embeddings supported (no dimensions parameter)
- Unique OCR method available

**OpenAI Full Support:**
- Complete feature set: text, streaming, structured, embeddings, images, audio
- GPT-5 model constraints handled automatically (temperature=1.0)
- Robust error handling and validation

---

## Feature Roadmap & Missing Capabilities

Source: roadmap.md (consolidated 2025-11-16)

### Current State Analysis

Wormhole is exceptionally strong for its core purpose: providing a fast, reliable, and unified interface for text and structured data generation from multiple LLMs. The SDK excels at:

- Multi-provider support with unified API
- Sub-microsecond latency (67ns core overhead)
- Enterprise-grade middleware (circuit breakers, rate limiting, retry logic)
- Thread-safe concurrent operations
- Clean architecture with capability-based interfaces

### Missing Feature Categories

#### 1. Multi-Modal Capabilities
Current API focuses on `Text()` and `Structured()` generation. Missing:
- Image generation (text-to-image)
- ✅ **Embeddings** (ALREADY IMPLEMENTED - see Embeddings API below)
- Vision capabilities (image-to-text)
- Audio processing (speech-to-text/text-to-speech)

#### 2. Advanced Orchestration
Provides foundational blocks but lacks higher-level abstractions:
- Native function calling/tool use
- Chain orchestration
- RAG components and helpers

#### 3. Developer Experience
- Token counting utilities
- Prompt templating system
- Context window management

---

## Tier 1: Core AI Capabilities (Highest Priority)

### ✅ Embeddings API (ALREADY IMPLEMENTED)

**Status**: **COMPLETE** - Full production implementation in SDK

**Implementation**:
```go
response, err := client.Embeddings().
    Provider("openai").
    Model("text-embedding-3-small").
    Input("Turn text into vectors", "Because math is beautiful").
    Dimensions(512). // Optional: customize dimensions
    Generate(ctx)
```

**Supported Providers**:

| Provider | Models | Dimensions | Notes |
|----------|---------|-----------|-------|
| **OpenAI** | `text-embedding-3-small`<br/>`text-embedding-3-large`<br/>`text-embedding-ada-002` | 1536 (small/ada)<br/>3072 (large)<br/>Customizable for v3 models | Best quality, customizable dimensions |
| **Gemini** | `models/embedding-001` | 768 | Good performance, Google ecosystem |
| **Ollama** | `nomic-embed-text`<br/>`all-minilm` | Varies by model | Free, local, no API limits |
| **Mistral** | `mistral-embed` | 1024 | Via `.BaseURL("https://api.mistral.ai/v1")` |
| **Any OpenAI-Compatible** | Provider-specific | Varies | Via `.BaseURL("https://provider-url/v1")` |

**Real-World Applications**:
- **Semantic Search**: Find documents by meaning, not just keywords
- **Recommendation Systems**: "Users who liked X also liked..." but smarter
- **RAG (Retrieval-Augmented Generation)**: Give LLMs relevant context from your data
- **Content Classification**: Automatically categorize text by semantic similarity
- **Duplicate Detection**: Find similar content even with different wording

**Example: Semantic Search**:
```go
// Step 1: Embed your documents once (cache these)
documents := []string{
    "Go is a programming language by Google",
    "Python is great for data science",
    "Machine learning requires lots of math",
    "Databases store structured information",
}

docResponse, _ := client.Embeddings().
    Provider("openai").
    Model("text-embedding-3-small").
    Input(documents...).
    Generate(ctx)

// Step 2: Embed user queries and find similar documents
query := "coding languages"
queryResponse, _ := client.Embeddings().
    Provider("openai").
    Model("text-embedding-3-small").
    Input(query).
    Generate(ctx)

// Step 3: Calculate cosine similarity
queryVector := queryResponse.Embeddings[0].Embedding
for i, docEmbedding := range docResponse.Embeddings {
    similarity := cosineSimilarity(queryVector, docEmbedding.Embedding)
    fmt.Printf("Document %d similarity: %.3f\n", i, similarity)
}
// Output: Document 0 (Go language) will have highest similarity score
```

### 2. Native Tool Use / Function Calling

**Implementation**: `client.ToolCall()` or similar API
**Priority**: High - Standard for agent applications
**Features**: Go function/struct registration, dynamic model selection
**Advantage**: More powerful than fixed structured output

### 3. Vision API (Image Input)

**Implementation**: Enhance `client.Text()` and `client.Structured()` for image inputs
**Priority**: High - Multi-modal is standard for flagship models
**Input Types**: URLs, byte arrays, base64
**Providers**: OpenAI GPT-4V, Claude 3.5 Sonnet, Gemini

### 4. Image Generation API

**Implementation**: `client.Images().Model(...).Prompt(...).Generate(ctx)`
**Priority**: Medium-High - Opens new application categories
**Providers**: OpenAI DALL-E, Stability AI, Midjourney
**Features**: Size control, style parameters, batch generation

---

## ✅ Already Supported via OpenAI-Compatible API

### Groq and Mistral Support

**Status**: ✅ **Already Available** - No separate implementation needed

**Implementation**: Use existing OpenAI provider with `BaseURL()` method

**Groq Example**:
```go
response, _ := client.Text().
    BaseURL("https://api.groq.com/openai/v1").
    Model("mixtral-8x7b-32768").
    Prompt("Fast inference via Groq").
    Generate(ctx)
```

**Mistral Example**:
```go
response, _ := client.Text().
    BaseURL("https://api.mistral.ai/v1").
    Model("mistral-large-latest").
    Prompt("European AI via Mistral").
    Generate(ctx)
```

**Embeddings Example**:
```go
// Mistral embeddings
response, _ := client.Embeddings().
    BaseURL("https://api.mistral.ai/v1").
    Model("mistral-embed").
    Input("Text to embed").
    Generate(ctx)
```

**Why This Approach**:
- Follows Wormhole's philosophy of eliminating complexity
- No separate provider packages needed for APIs that speak OpenAI protocol
- One code path = one security audit surface
- Instant support for new OpenAI-compatible providers

---

## Tier 2: Application Layer Abstractions

### 5. RAG Helpers

**Implementation**: `rag` sub-package with core utilities
**Priority**: High - Most common LLM application pattern
**Features**: Context management, document stuffing, result re-ranking
**Scope**: Utilities, not full framework

### 6. Token-Aware Utilities

**Implementation**: `client.CountTokens(model, text)` and text splitters
**Priority**: High - Fundamental context window management
**Features**: Model-specific counting, document chunking, smart splitting
**Use Cases**: Context limit avoidance, cost optimization

### 7. Conversation History Management

**Implementation**: Helper object or middleware for chat history
**Priority**: Medium - Common chatbot requirement
**Features**: Automatic history management, summarization strategies, token windowing
**Integration**: Works with existing middleware system

---

## Tier 3: Enterprise & Ecosystem Enhancements

### 8. Deeper Observability (OpenTelemetry)

**Implementation**: `OpenTelemetryMiddleware` with standardized traces/metrics
**Priority**: Medium - Enterprise integration requirement
**Features**: Distributed tracing, performance metrics, error tracking
**Integration**: Existing enterprise monitoring platforms

### 9. Content Moderation & Security

**Implementation**: `ModerationMiddleware` and `PIIRedactionMiddleware`
**Priority**: Medium - Production safety requirement
**Features**: Prompt/response checking, PII detection, content filtering
**Providers**: OpenAI Moderation API, custom filters

### 10. Extensible Agent Framework

**Implementation**: `AgentExecutor` for orchestrating tool calls and reasoning
**Priority**: Low-Medium - Advanced use case
**Features**: Agentic loops, multi-step reasoning, autonomous task completion
**Positioning**: Compete with LangChain agent capabilities

---

## Common Operations

### Running Tests

```bash
# Full test suite
go test ./...

# Specific package
go test ./pkg/wormhole

# With coverage
go test -cover ./...

# With race detection
go test -race ./...

# Benchmarks
make bench
```

### Performance Benchmarking

```bash
# Run all benchmarks
make bench

# Detailed quantum analysis
go test -bench=. -benchmem -cpuprofile=quantum.prof ./pkg/wormhole/
go tool pprof quantum.prof

# Stress test across parallel dimensions
go test -bench=BenchmarkConcurrent -cpu=1,2,4,8,16,32,64,128
```

### Adding New Providers

**Option 1: OpenAI-Compatible (Recommended)**
```go
// For providers using OpenAI API format
client := wormhole.New(
    wormhole.WithOpenAICompatible("provider-name", "https://api.provider.com/v1", types.ProviderConfig{
        APIKey: "your-api-key",
    }),
)
```

**Option 2: Custom Provider**
```go
// Step 1: Implement Provider interface
type CustomProvider struct {
    config types.ProviderConfig
}

func (p *CustomProvider) Text(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
    // Custom implementation
}

// Step 2: Create factory function
func NewCustomProvider(config types.ProviderConfig) (types.Provider, error) {
    return &CustomProvider{config: config}, nil
}

// Step 3: Register provider
client.RegisterProvider("custom", NewCustomProvider)

// Step 4: Use it
response, err := client.Text().Using("custom").Generate(ctx)
```

---

## Configuration

### Environment Variables

**Provider API Keys:**
```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENROUTER_API_KEY="sk-or-..."
export GROQ_API_KEY="gsk_..."
```

**Wormhole Configuration Overrides:**
```bash
export WORMHOLE_DEFAULT_TIMEOUT="5m"      # Default: 300s
export WORMHOLE_MAX_RETRIES="3"          # Default: 3
export WORMHOLE_INITIAL_RETRY_DELAY="500ms"  # Default: 500ms
export WORMHOLE_MAX_RETRY_DELAY="30s"    # Default: 30s
```

### Production Setup Example

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),

    // Primary provider
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY"), types.ProviderConfig{
        MaxRetries:    &[]int{3}[0],
        RetryDelay:    &[]time.Duration{500 * time.Millisecond}[0],
        RetryMaxDelay: &[]time.Duration{30 * time.Second}[0],
    }),

    // Backup provider
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY"), types.ProviderConfig{
        MaxRetries: &[]int{5}[0],
    }),

    // Global timeout
    wormhole.WithTimeout(2*time.Minute),

    // Production middleware
    wormhole.WithMiddleware(
        middleware.CircuitBreakerMiddleware(5, 30*time.Second),
        middleware.RateLimitMiddleware(100),
        middleware.LoggingMiddleware(logger),
    ),
)
```

---


## Why the SDK Works This Way

Understanding design decisions helps you use Wormhole effectively:

### API Design Choices

### Developer Experience

**Lesson 1**: Functional options pattern beats complex constructors
- **Why**: Optional parameters with sensible defaults
- **Result**: Clear, self-documenting API
- **Example**: WithOpenAI, WithAnthropic, WithTimeout

**Lesson 2**: Builder pattern improves readability for complex requests
- **Why**: Method chaining is clearer than config structs
- **Result**: client.Text().Model().Prompt().Generate()
- **Example**: Progressive disclosure (only specify what you need)

### Error Handling

**Lesson 1**: Per-provider retry configuration beats global middleware
- **Why**: Different providers have different reliability profiles
- **Result**: Fine-grained control (OpenAI 2 retries, Anthropic 5)
- **Evidence**: Respects Retry-After headers automatically

**Lesson 2**: Typed errors improve debugging
- **Why**: WormholeError with error codes vs generic errors
- **Result**: Users can switch on error type (rate limit, auth, model not found)
- **Example**: ErrorCodeRateLimit, ErrorCodeAuth, ErrorCodeModel
## Security Best Practices

### API Key Management

✅ **DO**:
- Use environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`)
- Rotate keys regularly
- Use different keys for dev/staging/production

❌ **DON'T**:
- Hardcode keys in source code
- Commit `.env` files to version control
- Share keys in error messages

### Error Message Sanitization

Wormhole automatically masks API keys in error messages:

**Before**:
```
Error: HTTP 401 at https://api.openai.com?key=sk-1234567890abcdef
```

**After**:
```
Error: HTTP 401 at https://api.openai.com?key=sk-1****cdef
```

### HTTPS-Only Policy

- All production providers use HTTPS
- Ollama (local) supports HTTP (localhost exception only)
- Custom providers should validate SSL certificates

---

## Troubleshooting

### Common Issues

#### "Provider not found"

**Cause**: Provider not registered or default provider not set

**Solution**:
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),  // Set default
    wormhole.WithOpenAI("your-key"),         // Register provider
)
```

#### "Model not supported by provider"

**Cause**: Using model with wrong provider (e.g., `claude-sonnet-4-5` with OpenAI)

**Solution**:
```go
// Option 1: Specify provider
response, _ := client.Text().
    Using("anthropic").
    Model("claude-sonnet-4-5").
    Generate(ctx)

// Option 2: Set default provider
client := wormhole.New(
    wormhole.WithDefaultProvider("anthropic"),
)
```

#### Streaming not working

**Cause**: Provider doesn't support streaming or network issues

**Solution**:
1. Check provider supports streaming (all built-in providers do)
2. Verify network connectivity
3. Enable debug logging:
```go
client := wormhole.New(
    wormhole.WithDebugLogging(true),
)
```

#### Rate limiting errors

**Cause**: Exceeding provider's rate limits

**Solution**:
```go
// Increase per-provider retry count
client := wormhole.New(
    wormhole.WithOpenAI("key", types.ProviderConfig{
        MaxRetries:    &[]int{5}[0],  // More retries
        RetryDelay:    &[]time.Duration{1 * time.Second}[0],  // Longer delays
        RetryMaxDelay: &[]time.Duration{60 * time.Second}[0],
    }),
)

// Add rate limiting middleware
client := wormhole.New(
    wormhole.WithMiddleware(
        middleware.RateLimitMiddleware(10),  // Max 10 requests/sec
    ),
)
```

---

## Quick Actions Reference

**Generate text**:
```go
response, _ := client.Text().Model("gpt-5").Prompt("...").Generate(ctx)
```

**Stream responses**:
```go
chunks, _ := client.Text().Model("gpt-5").Stream(ctx)
for chunk := range chunks { fmt.Print(chunk.Text) }
```

**Generate embeddings**:
```go
response, _ := client.Embeddings().Model("text-embedding-3-small").Input("...").Generate(ctx)
```

**Switch providers**:
```go
response, _ := client.Text().Using("anthropic").Model("claude-sonnet-4-5").Generate(ctx)
```

**Use OpenRouter (200+ models)**:
```go
client := wormhole.QuickOpenRouter()  // Uses OPENROUTER_API_KEY env var
response, _ := client.Text().Model("anthropic/claude-opus-4").Generate(ctx)
```

**Custom OpenAI-compatible provider**:
```go
response, _ := client.Text().
    BaseURL("https://api.custom.com/v1").
    Model("custom-model").
    Generate(ctx)
```

---

**Last Updated**: 2025-11-16
**Status**: Consolidated from memory.md and roadmap.md

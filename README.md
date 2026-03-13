# 🌀 Wormhole

*BURP* Listen up, because I'm only explaining this once.

While every vybe coder on the planet is copy-pasting LLM wrappers from ChatGPT and calling themselves "AI engineers," I went and built an actual SDK. One that works. One that doesn't fall apart the second you throw concurrent requests at it. One that doesn't make me want to collapse the multiverse every time I read the source code.

This is Wormhole. A Go SDK for talking to every LLM provider that matters — OpenAI, Anthropic, Gemini, Ollama, OpenRouter, and anything else that speaks the OpenAI protocol. Functional options. Thread-safe. Middleware stack. Adaptive rate limiting with a PID controller because I'm not an animal.

You get one API surface. You point it at any provider. It works. That's the pitch. If you need more convincing than that, maybe stick to dragging blocks around in Langflow.

[![Go](https://img.shields.io/badge/Go-1.23+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> *"It's like having a portal gun for AI APIs, but without the risk of accidentally creating a Jerry."* — Rick Sanchez, C-137

---

## Three Lines to AI

```go
response, _ := wormhole.QuickText("gpt-5.2", "Hello world", os.Getenv("OPENAI_API_KEY"))
fmt.Println(response.Content())
```

Or with a proper client:

```go
client := wormhole.New(wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")))
response, _ := client.Text().Model("gpt-5.2").Prompt("Hello").Generate(ctx)
```

> [!TIP]
> `QuickText()` spins up a throwaway client for one request. Good for scripts. For anything real, create a client once and reuse it.

---

## Quick Reference

| Task | Code |
|------|------|
| Text generation | `client.Text().Model("gpt-5.2").Prompt("...").Generate(ctx)` |
| Streaming | `client.Text().Model("gpt-5.2").Prompt("...").Stream(ctx)` |
| Stream + accumulate | `chunks, getText, _ := builder.StreamAndAccumulate(ctx)` |
| Structured output | `client.Structured().Model("gpt-5.2").Schema(s).GenerateAs(ctx, &result)` |
| Embeddings | `client.Embeddings().Model("text-embedding-3-small").Input("...").Generate(ctx)` |
| Multi-turn chat | `client.Text().Conversation(conv).Generate(ctx)` |
| Tool calling | `wormhole.RegisterTypedTool(client, name, desc, handler)` |
| Model fallback | `client.Text().Model("gpt-5.2").WithFallback("gpt-5.1-mini").Generate(ctx)` |
| Batch execution | `client.Batch().Add(req1).Add(req2).Concurrency(5).Execute(ctx)` |
| Clone builder | `base.Clone().Prompt("new").Generate(ctx)` |
| Switch provider | `client.Text().Using("anthropic").Model("claude-sonnet-4-5").Generate(ctx)` |
| Custom endpoint | `client.Text().BaseURL("http://localhost:11434/v1").Generate(ctx)` |
| Validate config | `if err := builder.Validate(); err != nil { ... }` |
| Check capabilities | `client.ProviderCapabilities("openai").SupportsToolCalling()` |
| Graceful shutdown | `client.Shutdown(ctx)` |

---

## Why This Exists

Every other Go LLM library I've seen falls into one of two categories: abandoned wrappers around a single provider, or thousand-line monstrosities that some vybe coder generated in one shot and never tested under real load.

Wormhole exists because I got tired of watching people build rickety bridges across the API gap and then act surprised when they collapse. Here's what you actually get:

- **One API, every provider.** OpenAI, Anthropic, Gemini, Ollama, OpenRouter, plus anything OpenAI-compatible. Same builder pattern. Same response types. Swap a provider name and keep moving.
- **Adaptive rate limiting.** Not the "sleep for a second and hope" kind. A PID controller that monitors latency, adjusts concurrency per-provider, and tracks p50/p90/p99 percentiles. Because math works better than guessing.
- **Thread-safe everything.** Concurrent map access, provider caching with reference counting, atomic request tracking. I fixed the race conditions so you don't have to discover them in production at 3 AM.
- **Tool calling that doesn't require a PhD in JSON Schema.** Define a Go struct, register it, and the SDK handles schema generation, execution, multi-turn loops, and error recovery. Type-safe. No `map[string]any` roulette.
- **Production middleware.** Circuit breakers, rate limiters, caching, health checks, retry logic with exponential backoff. Per-provider configuration because OpenAI and Anthropic don't fail the same way.
- **Graceful shutdown.** Request lifecycle tracking, in-flight request draining, idempotency keys. The stuff you need when your service handles real traffic and not just demo requests in a blog post.

```
BenchmarkTextGeneration-16     12566146    67 ns/op    0 B/op    0 allocs/op
BenchmarkWithMiddleware-16      5837629   171.5 ns/op  0 B/op    0 allocs/op
BenchmarkConcurrent-16          6826171   146.4 ns/op  0 B/op    0 allocs/op
```

67 nanoseconds of overhead per request. Zero allocations in the hot path. I didn't achieve this by vibing — I achieved it by profiling.

---

## Installation

```bash
go get github.com/garyblankenship/wormhole@latest
```

Requirements: Go 1.23+, API keys for the providers you want, and the bare minimum of self-respect required to not hardcode secrets in source files.

---

## Usage

### Text Generation

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
)

response, err := client.Text().
    Model("gpt-5.2").
    Prompt("Explain wormholes to someone who thinks AI wrote all their code for them").
    MaxTokens(200).
    Temperature(0.7).
    Generate(context.Background())

if err != nil {
    log.Fatalf("Portal malfunction: %v", err)
}

fmt.Println(response.Content())
fmt.Printf("Tokens: %d in, %d out\n", response.Usage.PromptTokens, response.Usage.CompletionTokens)
```

> [!WARNING]
> Never hardcode API keys. `os.Getenv("OPENAI_API_KEY")`. Always. I shouldn't have to say this.

### Streaming

```go
chunks, _ := client.Text().
    Model("gpt-5.2").
    Prompt("Write a mass resignation letter from every SDK I'm replacing").
    Stream(ctx)

for chunk := range chunks {
    fmt.Print(chunk.Content())
    if chunk.HasError() {
        log.Fatal(chunk.Error)
    }
}
```

Need the full text after streaming finishes? `StreamAndAccumulate` gives you both:

```go
chunks, getFullText, err := client.Text().
    Model("gpt-5.2").
    Prompt("Write something worth accumulating").
    StreamAndAccumulate(ctx)

for chunk := range chunks {
    fmt.Print(chunk.Content())  // real-time output
}

fullText := getFullText()  // complete text after stream ends
```

### Structured Output

```go
type Analysis struct {
    Verdict    string  `json:"verdict"`
    Confidence float64 `json:"confidence"`
    Reasoning  string  `json:"reasoning"`
}

var result Analysis
_, err := client.Structured().
    Model("gpt-5.2").
    Prompt("Is this README better than what a vybe coder would produce?").
    Schema(wormhole.MustSchemaFromStruct(Analysis{})).
    GenerateAs(ctx, &result)
```

### Conversations

```go
conv := types.NewConversation().
    System("You are a code reviewer who doesn't tolerate vibes-based development").
    User("Review my Go code").
    Assistant("Show me what you've got.").
    User("Here's a function that uses 14 nested if statements...")

response, _ := client.Text().Conversation(conv).Generate(ctx)
```

Few-shot prompting:

```go
conv := types.FewShot(
    "You translate English to Spanish",
    []types.ExamplePair{
        {User: "Hello", Assistant: "Hola"},
        {User: "Goodbye", Assistant: "Adiós"},
    },
).User("How are you?")
```

### Type-Safe Tool Calling

The old way required building JSON schemas by hand and praying your type assertions didn't panic in production. The new way uses reflection to generate schemas from Go structs, because I believe in automation that actually works.

```go
type WeatherArgs struct {
    City string `json:"city" tool:"required" desc:"City name"`
    Unit string `json:"unit" tool:"enum=celsius,fahrenheit" desc:"Temperature unit"`
}

wormhole.RegisterTypedTool(client, "get_weather", "Get current weather",
    func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
        return getWeather(args.City, args.Unit), nil
    },
)

// Now just ask. The SDK handles tool detection, execution,
// result forwarding, and multi-turn loops automatically.
response, _ := client.Text().
    Model("gpt-5.2").
    Prompt("What's the weather in San Francisco?").
    WithToolsEnabled().
    Generate(ctx)
```

What happens behind the curtain: AI decides to call your function → SDK executes it → SDK sends the result back → AI generates a final answer. Multiple tools can fire in parallel. Errors get routed back to the model for recovery. Loop protection prevents runaway execution.

You can also do it manually if you need control:

```go
response, _ := client.Text().
    Prompt("What's the weather?").
    WithToolsDisabled().
    Generate(ctx)

for _, call := range response.ToolCalls {
    fmt.Printf("AI wants: %s(%v)\n", call.Name, call.Arguments)
}
```

### Embeddings

```go
response, _ := client.Embeddings().
    Model("text-embedding-3-small").
    Input("Turn this into a vector", "And this one too").
    Dimensions(512).
    Generate(ctx)

for i, emb := range response.Embeddings {
    fmt.Printf("Text %d: %d dimensions\n", i, len(emb.Embedding))
}
```

| Provider | Models | Notes |
|----------|--------|-------|
| OpenAI | `text-embedding-3-small`, `text-embedding-3-large` | Customizable dimensions |
| Gemini | `models/embedding-001` | 768 dimensions |
| Ollama | `nomic-embed-text`, `all-minilm` | Local, free, no rate limits |
| Any OpenAI-compatible | Via `.BaseURL()` | Mistral, etc. |

### Batch Operations

Process multiple requests concurrently:

```go
results := client.Batch().
    Add(client.Text().Model("gpt-5.2").Prompt("Question 1")).
    Add(client.Text().Model("gpt-5.2").Prompt("Question 2")).
    Add(client.Text().Model("gpt-5.2").Prompt("Question 3")).
    Concurrency(5).
    Execute(ctx)

for _, r := range results {
    if r.Error != nil {
        log.Printf("Request %d failed: %v", r.Index, r.Error)
    } else {
        fmt.Printf("Response %d: %s\n", r.Index, r.Response.Content())
    }
}
```

Race multiple models, take the first response:

```go
resp, _ := client.Batch().
    Add(client.Text().Model("gpt-5.2").Prompt("Task")).
    Add(client.Text().Using("anthropic").Model("claude-sonnet-4-5").Prompt("Task")).
    ExecuteFirst(ctx)
```

### Builder Cloning & Fallbacks

```go
// Build once, clone for variations
base := client.Text().Model("gpt-5.2").Temperature(0.7)
resp1, _ := base.Clone().Prompt("Question 1").Generate(ctx)
resp2, _ := base.Clone().Prompt("Question 2").Generate(ctx)

// Automatic fallback chain
response, _ := client.Text().
    Model("gpt-5.2").
    WithFallback("gpt-5.1-mini", "gpt-5").
    Prompt("Important task").
    Generate(ctx)  // tries each model in order on failure
```

### BaseURL: One Client, Any Endpoint

Every OpenAI-compatible API works with a single client. Just change the URL.

```go
client := wormhole.New(wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")))

// OpenRouter
response, _ := client.Text().
    BaseURL("https://openrouter.ai/api/v1").
    Model("anthropic/claude-sonnet-4-5").
    Generate(ctx)

// Local Ollama
response, _ := client.Text().
    BaseURL("http://localhost:11434/v1").
    Model("llama3.2").
    Generate(ctx)

// LM Studio, vLLM, or literally anything else
response, _ := client.Text().
    BaseURL("http://localhost:1234/v1").
    Model("whatever-you-loaded").
    Generate(ctx)
```

### OpenRouter

Instant access to 200+ models from every major provider through one API key:

```go
client, _ := wormhole.QuickOpenRouter()  // reads OPENROUTER_API_KEY

models := []string{
    "openai/gpt-5.2",
    "anthropic/claude-sonnet-4-5",
    "google/gemini-2.5-pro",
    "meta-llama/llama-3.3-70b-instruct",
}

for _, model := range models {
    response, err := client.Text().
        Model(model).
        Prompt("One sentence about quantum computing").
        MaxTokens(100).
        Generate(ctx)

    if err != nil {
        continue
    }
    fmt.Printf("%s: %s\n", model, response.Content())
}
```

Wormhole doesn't block unknown model names with a local registry. If OpenRouter supports it, you can use it. If the model doesn't exist, you get a proper error from the API — not a validation wall from some outdated list a vybe coder forgot to update six months ago.

---

## Provider Support

| Provider | Type | Configuration |
|----------|------|---------------|
| **OpenAI** | Native | `WithOpenAI(key)` |
| **Anthropic** | Native | `WithAnthropic(key)` |
| **Gemini** | Native | `WithGemini(key)` |
| **Ollama** | Native | `WithOllama(config)` |
| **Groq** | OpenAI-compatible | `WithGroq(key)` |
| **Mistral** | OpenAI-compatible | `WithMistral(config)` |
| **LM Studio** | OpenAI-compatible | `WithLMStudio(config)` |
| **vLLM** | OpenAI-compatible | `WithVLLM(config)` |
| **OpenRouter** | OpenAI-compatible | `QuickOpenRouter()` or `WithOpenAICompatible(...)` |
| **Any other** | Custom | `WithCustomProvider(name, factory)` or `WithOpenAICompatible(name, url, config)` |

---

## Production Configuration

This is what a real deployment looks like. Not a demo. Not a tutorial. Not something a vybe coder pasted from a "10 LLM tricks" Medium article.

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("anthropic"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY"), types.ProviderConfig{
        MaxRetries:    &[]int{2}[0],
        RetryDelay:    &[]time.Duration{200 * time.Millisecond}[0],
        RetryMaxDelay: &[]time.Duration{5 * time.Second}[0],
    }),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY"), types.ProviderConfig{
        MaxRetries:    &[]int{5}[0],
        RetryDelay:    &[]time.Duration{500 * time.Millisecond}[0],
        RetryMaxDelay: &[]time.Duration{30 * time.Second}[0],
    }),
    wormhole.WithOllama(types.ProviderConfig{
        MaxRetries: &[]int{0}[0],  // local provider, no retries
    }),
    wormhole.WithTimeout(30*time.Second),
    wormhole.WithMiddleware(
        middleware.CircuitBreakerMiddleware(5, 30*time.Second),
        middleware.RateLimitMiddleware(100),
    ),
)
```

### Per-Provider Retries

Different providers fail differently. OpenAI is usually stable. Anthropic can be temperamental. Local models don't need network retries at all. So retries are configured per-provider at the transport level, not as a blanket middleware that treats every failure the same way.

Retryable status codes: `429` (rate limit — respects `Retry-After`), `500`, `502`, `503`, `504`.

Non-retryable: `400`, `401`, `403`, `404`, `422`. If your API key is wrong, retrying won't fix it.

### Adaptive Rate Limiting

The SDK includes a PID-controlled concurrency limiter that adjusts per-provider throughput based on observed latency and error rates. This isn't a fixed token bucket. It watches how the provider is actually performing and tunes capacity up or down in real time.

```go
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
)

client.EnableAdaptiveConcurrency(&wormhole.EnhancedAdaptiveConfig{
    MinCapacity:   2,
    MaxCapacity:   50,
    TargetLatency: 500 * time.Millisecond,
})

// Check how it's performing
stats := client.GetAdaptiveConcurrencyStats()
```

### Graceful Shutdown & Lifecycle

```go
// Tracks all in-flight requests
// Drains cleanly on shutdown with configurable timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
client.Shutdown(ctx)
```

### Idempotency

Client-side request deduplication. Same key within the TTL window returns the cached response instead of burning another API call.

```go
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithIdempotencyKey("unique-request-id", 5*time.Minute),
)
```

---

## Validation

Catch configuration mistakes before they cost you an API call:

```go
builder := client.Text().Model("gpt-5.2").Temperature(0.7).MaxTokens(1000)

if err := builder.Validate(); err != nil {
    if vErr, ok := types.AsValidationError(err); ok {
        fmt.Printf("Field '%s': %s\n", vErr.Field, vErr.Message)
    }
    return err
}

response, _ := builder.Prompt("Now I know it won't fail on config").Generate(ctx)
```

Validates: model specified, temperature range (0-2), top_p range (0-1), max_tokens positive, penalty ranges (-2 to 2).

---

## Unified Response Accessors

Every response type has `.Content()`. No more remembering different field names for different providers.

```go
// Text
textResp.Content()           // string
textResp.HasToolCalls()      // bool
textResp.IsComplete()        // bool — finished normally
textResp.WasTruncated()      // bool — hit max_tokens

// Structured
structResp.Content()         // any
structResp.ContentAs(&person) // type-safe unmarshal

// Embeddings
embResp.Content()            // []float64 — first vector
embResp.Vector(1)            // specific vector by index
embResp.Count()              // number of embeddings

// Streaming chunks
chunk.Content()              // works for text and delta
chunk.IsDone()
chunk.HasError()
```

---

## Error Handling

Typed errors with codes you can switch on, because `strings.Contains(err.Error(), "rate")` is not error handling — it's a cry for help.

```go
response, err := client.Text().Generate(ctx)
if err != nil {
    var wErr *types.WormholeError
    if errors.As(err, &wErr) {
        switch wErr.Code {
        case types.ErrorCodeRateLimit:
            // per-provider retries handle this automatically,
            // but you can add your own logic here
        case types.ErrorCodeAuth:
            return fmt.Errorf("fix your API key: %w", wErr)
        case types.ErrorCodeModel:
            // model not found — try a fallback
        case types.ErrorCodeTimeout:
            // increase your context timeout
        }
    }
}
```

---

## Custom Providers

If your provider speaks the OpenAI protocol, it's one line:

```go
client := wormhole.New(
    wormhole.WithOpenAICompatible("perplexity", "https://api.perplexity.ai", types.ProviderConfig{
        APIKey: os.Getenv("PERPLEXITY_API_KEY"),
    }),
)
```

If it doesn't, implement the `Provider` interface and register a factory:

```go
client.RegisterProvider("my-provider", func(config types.ProviderConfig) (types.Provider, error) {
    return &MyProvider{config: config}, nil
})

response, _ := client.Text().Using("my-provider").Model("custom-model").Generate(ctx)
```

No core modifications. No PRs begging me to add support for your niche provider. You do it yourself, and it works with every feature in the SDK — middleware, retries, streaming, tool calling, all of it.

---

## Testing

Mock provider included. Don't burn API credits testing your business logic.

```go
import wmtest "github.com/garyblankenship/wormhole/pkg/testing"

func TestSomething(t *testing.T) {
    mock := wmtest.NewMockProvider("openai").
        WithTextResponse(wmtest.TextResponseWith("Mock response"))

    client := wormhole.New(
        wormhole.WithCustomProvider("openai", wmtest.MockProviderFactory(mock)),
        wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
        wormhole.WithDefaultProvider("openai"),
    )

    result, err := client.Text().
        Model("gpt-5.2").
        Prompt("test").
        Generate(context.Background())

    assert.NoError(t, err)
    assert.Equal(t, "Mock response", result.Content())
}
```

---

## Benchmarking

```bash
make bench                    # standard benchmarks
make bench-profile            # CPU + memory profiling
make perf-test                # regression suite (5 iterations)

# or manually
go test -bench=. -benchmem ./pkg/wormhole/
```

---

## Model Discovery

The SDK maintains a registry of known models with capabilities and constraints, but doesn't use it as a gate. Known models get validated and auto-configured (GPT-5 temperature constraints, etc.). Unknown models pass through to the provider — because the registry being a week behind shouldn't block you from using a model that launched yesterday.

```go
models := types.ListAvailableModels("openai")
cost, _ := types.EstimateModelCost("gpt-5.2", 1000, 500)
constraints, _ := types.GetModelConstraints("gpt-5.2")

caps := client.ProviderCapabilities("openai")
if caps.SupportsToolCalling() {
    // safe to register tools
}
```

---

## Security

```go
// Do this
client := wormhole.New(wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")))

// Not this. Ever.
client := wormhole.New(wormhole.WithOpenAI("sk-hardcoded-key"))
```

API keys in error messages are automatically masked (`sk-1****cdef`). Add `.env*` and `*.key` to your `.gitignore`. Use HTTPS endpoints. Set timeouts to prevent resource exhaustion. This is basic stuff — but I've seen enough leaked keys on GitHub to know it needs saying.

---

## Contributing

1. Don't break the tests.
2. Don't regress the benchmarks.
3. Follow the functional options pattern.
4. If you add a provider, add tests for it.
5. No JavaScript. This is Go. Act like it.

```bash
git clone https://github.com/garyblankenship/wormhole
cd wormhole
make test
make bench
make lint
```

---

## License

MIT. Use it however you want. If it breaks your production environment, that's between you and your incident review — I gave you the tools, not the judgment to use them correctly.

---

## Credits

Built by someone who got tired of watching vybe coders ship LLM wrappers held together with hope and `interface{}`. Powered by spite, benchmarks, and the firm belief that nanoseconds matter.

*Now leave me alone. I have science to do.*

**Wubba lubba dub dub!** 🛸

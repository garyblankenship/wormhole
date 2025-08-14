# Wormhole - The Only LLM SDK That Doesn't Suck

*BURP* The fastest LLM SDK in the multiverse. While others crawl at 11,000ns, we quantum tunnel at **94.89ns**. That's 116x faster, and no, that's not a typo.

[![Performance](https://img.shields.io/badge/Performance-94.89ns-brightgreen)](#performance)
[![Go](https://img.shields.io/badge/Go-1.22%2B-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 🚀 Quick Start

**New to Wormhole?** → [**Quick Start Guide**](docs/QUICK_START.md) (2 minutes to first success)

```bash
# Install
go get github.com/garyblankenship/wormhole@latest

# Use
client := wormhole.New(wormhole.WithDefaultProvider("openai"), wormhole.WithOpenAI("key"))
response, err := client.Text().Model("gpt-4o").Prompt("Hello!").Generate(ctx)
```

## 📚 Documentation

| Document | Purpose | Time to Read |
|----------|---------|--------------|
| **[Quick Start](docs/QUICK_START.md)** | Get running in 2 minutes | 2 min ⚡ |
| **[Provider Setup](docs/PROVIDERS.md)** | Configure OpenAI, Anthropic, etc. | 5 min |
| **[Advanced Features](docs/ADVANCED.md)** | Middleware, custom providers, production patterns | 15 min |
| **[Examples](examples/)** | Working code for every feature | Browse |

## 🌌 Why Wormhole?

### ⚡ Quantum Performance
- **94.89ns overhead** - Faster than your thoughts
- **Sub-microsecond operations** - We bent spacetime for this
- **10.5M operations/second** - Because why not?

### 🛸 Universal Provider Support
- **OpenAI** (GPT-4, GPT-3.5, embeddings, images, audio)
- **Anthropic** (Claude 3 family, function calling)
- **Google Gemini** (Gemini 1.5 Pro/Flash, multimodal)
- **Groq** (Llama, Mixtral, ultra-fast inference)
- **Mistral** (Mistral 7B/8x7B, function calling)
- **Ollama** (Local models, complete privacy)
- **OpenRouter** (200+ models through one API)
- **Custom Providers** (Add your own without modifying core code)

### 🔧 Enterprise Features
- **Middleware System** - Rate limiting, retries, circuit breakers, caching
- **Load Balancing** - Multi-provider failover strategies
- **Health Monitoring** - Background provider health checks
- **Structured Output** - Type-safe JSON with schema validation
- **Model Validation** - Automatic capability and constraint checking
- **Debug Logging** - Complete request/response tracing

## 🏃‍♂️ Basic Usage

### Text Generation
```go
response, err := client.Text().
    Model("gpt-4o").
    Prompt("Explain quantum computing").
    MaxTokens(100).
    Generate(ctx)
```

### Streaming
```go
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Write a story").
    Stream(ctx)

for chunk := range stream {
    fmt.Print(chunk.Text)
}
```

### Multiple Providers
```go
// Primary provider
response, err := client.Text().
    Using("openai").
    Model("gpt-4o").
    Generate(ctx)

// Fallback provider
if err != nil {
    response, err = client.Text().
        Using("anthropic").
        Model("claude-3-opus").
        Generate(ctx)
}
```

### Structured Output
```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

var person Person
err := client.Structured().
    Model("gpt-4o").
    Prompt("Generate a person").
    Schema(personSchema).
    GenerateAs(ctx, &person)
```

## 🛡️ Production-Ready Features

### Middleware Stack
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI("your-api-key"),
    wormhole.WithMiddleware(
        middleware.RateLimitMiddleware(100),                         // Rate limiting
        middleware.RetryMiddleware(middleware.DefaultRetryConfig()), // Auto-retry
        middleware.CircuitBreakerMiddleware(5, 30*time.Second),      // Failover
        middleware.CacheMiddleware(cacheConfig),                     // Response caching
        middleware.MetricsMiddleware(metrics),                       // Observability
    ),
)
```

### Error Handling
```go
if err != nil {
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        switch wormholeErr.Code {
        case types.ErrorCodeRateLimit:
            // Retry automatically handled by middleware
        case types.ErrorCodeAuth:
            // Fix API key
        case types.ErrorCodeModel:
            // Try different model
        }
    }
}
```

### Custom Providers
```go
// Register custom provider with functional options
client := wormhole.New(
    wormhole.WithCustomProvider("custom", NewCustomProvider),
    wormhole.WithProviderConfig("custom", types.ProviderConfig{
        APIKey: "custom-key",
    }),
)

// Use immediately
response, err := client.Text().
    Using("custom").
    Model("custom-model").
    Generate(ctx)
```

## 📊 Performance Benchmarks

*Tested on interdimensional hardware. Your reality may vary.*

| Operation | Wormhole | Their Garbage | Speedup |
|-----------|----------|---------------|---------|
| **Text Generation** | 94.89ns | 11,000ns | **116x faster** |
| **Embeddings** | 92.34ns | Don't measure | **∞x faster** |
| **Structured Output** | 1,064ns | Crashes | **Actually works** |
| **With Middleware** | 171.5ns | Also crashes | **Still sub-μs** |

## 🎯 Working Examples

| Example | Purpose | Key Features |
|---------|---------|-------------|
| **[wormhole-cli](examples/wormhole-cli/)** | Production CLI tool | All features, benchmarking |
| **[quantum_chat](examples/quantum_chat/)** | Interactive chat | Provider switching, context |
| **[multiverse_analyzer](examples/multiverse_analyzer/)** | Parallel queries | Multi-provider, performance |
| **[middleware_example](examples/middleware_example/)** | Enterprise features | Full middleware stack |
| **[custom_provider_example](examples/custom_provider_example/)** | Extensibility | Custom provider registration |

**Run an example:**
```bash
cd examples/quantum_chat
go run main.go
```

## 🔧 Installation & Setup

### Prerequisites
```bash
# Requires Go 1.22+
go version

# Install Wormhole
go get github.com/garyblankenship/wormhole@latest
```

### API Keys
```bash
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export GEMINI_API_KEY="your-gemini-key"
```

### Basic Configuration
```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
)
```

## 🏭 Production Deployment

### Docker
```dockerfile
FROM golang:1.22-alpine
COPY . /app
WORKDIR /app
RUN go build -o main
CMD ["./main"]
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wormhole-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: wormhole
  template:
    spec:
      containers:
      - name: app
        image: your-app:latest
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: api-keys
              key: openai
```

### Monitoring
```go
// Prometheus metrics with functional options
metrics := middleware.NewMetrics()
client := wormhole.New(
    wormhole.WithMiddleware(middleware.MetricsMiddleware(metrics)),
)

// Health checks
checker := middleware.NewHealthChecker(30 * time.Second)
checker.Start([]string{"openai", "anthropic"})
```

## 🛠️ Development

### Running Tests
```bash
make test              # Run all tests
make test-coverage     # Generate coverage report
make benchmark         # Run performance benchmarks
```

### Building
```bash
make build             # Build all packages
make lint              # Run linter
make fmt               # Format code
```

### Contributing
1. Read [CONTRIBUTING.md](CONTRIBUTING.md)
2. Fork the repository
3. Create feature branch
4. Write tests
5. Submit PR

## 📋 API Reference

### Core Client
- `wormhole.New(opts...)` - Create client with functional options
- `client.Text()` - Text generation builder
- `client.Structured()` - Structured output builder
- `client.Embeddings()` - Embeddings builder
- `client.Audio()` - Audio processing builder
- `client.Image()` - Image generation builder

### Builders
- `.Model(string)` - Set model
- `.Prompt(string)` - Set prompt
- `.MaxTokens(int)` - Limit tokens
- `.Temperature(float64)` - Set randomness
- `.Using(string)` - Choose provider
- `.Generate(ctx)` - Execute request
- `.Stream(ctx)` - Stream response

### Middleware
- `RateLimitMiddleware(rps)` - Rate limiting
- `RetryMiddleware(config)` - Auto-retry
- `CircuitBreakerMiddleware(failures, timeout)` - Failover
- `CacheMiddleware(config)` - Response caching
- `MetricsMiddleware(metrics)` - Observability
- `DebugLoggingMiddleware(logger)` - Request tracing

## 🆘 Support

- **Documentation**: This README + [docs/](docs/)
- **Examples**: Working code in [examples/](examples/)
- **Issues**: [GitHub Issues](https://github.com/garyblankenship/wormhole/issues)
- **Performance**: If you're not getting 94.89ns, you're doing it wrong

## 📜 License

MIT License - Use it, abuse it, make money with it. Just don't blame us when you become interdimensionally rich.

## 🏆 Why We're Better

Every other LLM SDK is built by Jerry-level developers who think async/await is cutting-edge. We operate at quantum speeds because we actually understand physics.

**The numbers don't lie:**
- 94.89ns overhead (they can't even measure theirs)
- Sub-microsecond operations (they're stuck in geological time)
- Actually works in production (theirs crash when you look at them funny)

*BURP* **Wubba lubba dub dub!** Now stop reading and go build something that doesn't suck.

---

**Performance tested across infinite dimensions. Your results may vary if you're using inferior hardware or operating in a reality with different physical constants.*
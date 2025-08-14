# 🌀 Wormhole Examples

Working examples demonstrating every feature of the fastest LLM SDK in the multiverse.

## 🚀 Quick Start

```bash
# Clone and setup
git clone https://github.com/garyblankenship/wormhole.git
cd wormhole/examples

# Set API keys
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"

# Run any example
cd quantum_chat && go run main.go
```

## 📚 Examples by Category

### 🎯 **Getting Started**

#### [Basic Usage](cmd/simple/)
**Purpose**: Minimal example showing core functionality  
**Time**: 2 minutes  
**Features**: Text generation, basic configuration

```bash
cd cmd/simple && go run main.go
```

#### [Multi-Provider Setup](multi_provider/)
**Purpose**: Using multiple AI providers  
**Time**: 5 minutes  
**Features**: Provider switching, configuration, error handling

```bash
cd multi_provider && go run main.go
```

### 🏭 **Production Ready**

#### [wormhole-cli](wormhole-cli/)
**Purpose**: Complete CLI application  
**Time**: 10 minutes  
**Features**: All SDK features, error handling, benchmarking

```bash
cd wormhole-cli
go build && ./wormhole-cli generate -prompt "Hello world" -verbose
```

**Commands:**
- `generate` - Text generation with options
- `stream` - Real-time streaming
- `embedding` - Vector embeddings
- `benchmark` - Performance testing

#### [Middleware Stack](middleware_example/)
**Purpose**: Enterprise reliability features  
**Time**: 10 minutes  
**Features**: Rate limiting, retries, circuit breakers, metrics

```bash
cd middleware_example && go run main.go
```

**Demonstrates:**
- Production middleware configuration
- Error handling and recovery
- Performance monitoring
- Custom middleware creation

### 🔧 **Advanced Features**

#### [Custom Provider](custom_provider_example/)
**Purpose**: Extending Wormhole with new providers  
**Time**: 15 minutes  
**Features**: Provider registration, model registration, full integration

```bash
cd custom_provider_example && go run main.go
```

**Shows how to:**
- Implement Provider interface
- Register custom providers
- Add model validation
- Use with existing middleware

#### [Structured Output](user_feedback_demo/)
**Purpose**: Type-safe JSON responses  
**Time**: 5 minutes  
**Features**: Schema validation, structured data extraction

```bash
cd user_feedback_demo && go run main.go
```

### 🌐 **Provider-Specific**

#### [OpenRouter Integration](openrouter_example/)
**Purpose**: Access 200+ models through one API  
**Time**: 5 minutes  
**Features**: Multi-model access, cost optimization, fallback strategies

```bash
cd openrouter_example && go run main.go
```

#### [Ollama Local Models](ollama_example/)
**Purpose**: Private, local AI models  
**Time**: 5 minutes (requires Ollama)  
**Features**: Local inference, privacy, custom models

```bash
cd ollama_example && go run main.go
```

#### [LMStudio Integration](lmstudio_example/)
**Purpose**: Local model serving  
**Time**: 5 minutes (requires LMStudio)  
**Features**: Local API, custom models

```bash
cd lmstudio_example && go run main.go
```

### 🎮 **Interactive Demos**

#### [Quantum Chat](quantum_chat/)
**Purpose**: Multi-provider chat interface  
**Time**: Interactive  
**Features**: Provider switching, conversation context, real-time metrics

```bash
cd quantum_chat && go run main.go
```

**Commands:**
- `/switch <provider>` - Change AI provider
- `/speed` - Show performance metrics
- `/help` - List all commands
- `/exit` - Quit

#### [Portal Stream](portal_stream/)
**Purpose**: Real-time streaming demonstration  
**Time**: Interactive  
**Features**: Token streaming, latency metrics, visual feedback

```bash
cd portal_stream && go run main.go "Write a story about AI"
```

### 📊 **Performance & Analysis**

#### [Multiverse Analyzer](multiverse_analyzer/)
**Purpose**: Parallel provider queries  
**Time**: 30 seconds  
**Features**: Concurrent requests, performance comparison, speedup analysis

```bash
cd multiverse_analyzer && go run main.go "What is consciousness?"
```

**Output:**
- Response from each provider
- Individual latencies
- Parallel speedup metrics
- Provider comparison

#### [Benchmark Suite](wormhole-cli/)
**Purpose**: Performance testing and validation  
**Time**: 2 minutes  
**Features**: Latency measurement, throughput testing, regression detection

```bash
cd wormhole-cli && ./wormhole-cli benchmark -iterations 100
```

### 🔬 **Specialized Use Cases**

#### [Feedback Improvements](feedback_improvements/)
**Purpose**: User feedback integration patterns  
**Time**: 5 minutes  
**Features**: Model validation, cost estimation, constraint handling

```bash
cd feedback_improvements && go run main.go
```

#### [Audio Processing](wormhole-cli/)
**Purpose**: Speech-to-text and text-to-speech  
**Time**: 5 minutes  
**Features**: Audio transcription, voice synthesis

```bash
cd wormhole-cli && ./wormhole-cli audio -file audio.wav
```

## 🏃‍♂️ Running Examples

### Prerequisites
```bash
# Go 1.22+
go version

# API Keys (as needed)
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"
export GEMINI_API_KEY="your-key"
```

### Build All Examples
```bash
# Build everything
find . -name "main.go" -execdir go build \;

# Or build specific example
cd quantum_chat && go build
```

### Common Environment Variables
```bash
# Default provider
export WORMHOLE_DEFAULT_PROVIDER=openai

# Debug mode
export WORMHOLE_DEBUG=true

# Custom endpoints
export OPENAI_BASE_URL=https://your-proxy.com/v1
```

## 📋 Example Comparison

| Example | Complexity | Features | Best For |
|---------|------------|----------|----------|
| **cmd/simple** | ⭐ | Basic text generation | First-time users |
| **multi_provider** | ⭐⭐ | Multiple providers | Provider comparison |
| **quantum_chat** | ⭐⭐⭐ | Interactive UI | Demos, testing |
| **wormhole-cli** | ⭐⭐⭐⭐ | Complete application | Production reference |
| **middleware_example** | ⭐⭐⭐⭐ | Enterprise features | Production deployment |
| **custom_provider_example** | ⭐⭐⭐⭐⭐ | SDK extension | Advanced integration |

## 🔧 Development Patterns

### Error Handling
```go
response, err := client.Text().Generate(ctx)
if err != nil {
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        // Handle specific error types
        switch wormholeErr.Code {
        case types.ErrorCodeRateLimit:
            // Retry logic
        case types.ErrorCodeAuth:
            // Authentication fix
        }
    }
}
```

### Configuration Management
```go
// Environment-based config
config := wormhole.Config{
    DefaultProvider: os.Getenv("WORMHOLE_DEFAULT_PROVIDER"),
    Providers: map[string]types.ProviderConfig{
        "openai": {APIKey: os.Getenv("OPENAI_API_KEY")},
    },
}
```

### Performance Monitoring
```go
// Add metrics to any example
metrics := middleware.NewMetrics()
client.Use(middleware.MetricsMiddleware(metrics))

// Check performance
requests, errors, avgDuration := metrics.GetStats()
```

## 🆘 Troubleshooting

### Common Issues

**Build Errors:**
```bash
go mod tidy && go build
```

**Missing API Keys:**
```bash
export OPENAI_API_KEY="your-actual-key"
```

**Slow Performance:**
- Check network connection
- Verify API endpoint
- Use local providers (Ollama) for testing

**Provider Errors:**
- Verify API key validity
- Check rate limits
- Try different provider

### Debug Mode
```bash
export WORMHOLE_DEBUG=true
go run main.go
```

## 📊 Performance Expectations

| Metric | Expected | If Slower |
|--------|----------|-----------|
| **SDK Overhead** | ~95ns | Check setup |
| **API Round-trip** | <2s | Provider/network issue |
| **Streaming TTFT** | <1s | Provider/network issue |
| **Parallel Speedup** | 2-3x | Increase concurrency |

## 🎯 Next Steps

1. **Start Simple**: Run `cmd/simple/main.go`
2. **Try Interactive**: Run `quantum_chat/main.go`  
3. **Test Performance**: Run `wormhole-cli benchmark`
4. **Go Production**: Study `middleware_example/main.go`
5. **Extend SDK**: Build custom provider with `custom_provider_example/`

## 📚 Related Documentation

- **[Quick Start Guide](../docs/QUICK_START.md)** - Get running in 2 minutes
- **[Provider Setup](../docs/PROVIDERS.md)** - Configure all providers
- **[Advanced Features](../docs/ADVANCED.md)** - Enterprise patterns
- **[Main README](../README.md)** - Complete documentation

---

*Every example has been tested across multiple dimensions. Your mileage may vary in realities with different physical constants.*
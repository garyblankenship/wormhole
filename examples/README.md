# 🌀 Wormhole Examples

Welcome to the comprehensive example collection for Wormhole, the ultra-fast Go LLM SDK. These examples demonstrate real-world usage patterns and best practices for integrating multiple AI providers with enterprise-grade reliability.

## 🚀 Getting Started

### Quick Examples
- **[basic/](./basic/)** - Simple API showcase without external dependencies
- **[comprehensive/](./comprehensive/)** - Full-featured demonstration with real API calls

### Core Functionality  
- **[middleware_example/](./middleware_example/)** - Enterprise middleware stack demonstration
- **[streaming-demo/](./streaming-demo/)** - Real-time streaming response handling
- **[interactive-chat/](./interactive-chat/)** - Terminal-based chat interface

## 🌐 Provider Examples

### Multi-Provider Support
- **[multi_provider/](./multi_provider/)** - Working with multiple providers simultaneously
- **[openrouter_example/](./openrouter_example/)** - OpenRouter integration with 200+ models

### Specific Providers
- **[ollama_example/](./ollama_example/)** - Local model serving with Ollama
- **[mistral_example/](./mistral_example/)** - Mistral AI integration
- **[lmstudio_example/](./lmstudio_example/)** - Local LMStudio server integration
- **[openai_compatible_example/](./openai_compatible_example/)** - Generic OpenAI-compatible APIs

## 🔧 Advanced Patterns

### Performance & Scaling
- **[concurrent-analysis/](./concurrent-analysis/)** - Concurrent request processing
- **[custom_provider_example/](./custom_provider_example/)** - Building custom provider implementations

### Developer Experience
- **[dx_improvements/](./dx_improvements/)** - Enhanced developer experience patterns
- **[feedback_improvements/](./feedback_improvements/)** - Production feedback implementations
- **[user_feedback_demo/](./user_feedback_demo/)** - User experience optimization demos

### CLI Tools
- **[wormhole-cli/](./wormhole-cli/)** - Command-line interface example

## 📋 Example Categories

### **Learning Path** 🎯
1. `basic/` - Understand the API structure
2. `comprehensive/` - See full feature implementation  
3. `middleware_example/` - Learn enterprise patterns
4. `openrouter_example/` - Explore provider ecosystem

### **Provider Integration** 🌐
- Multi-provider setups and fallback strategies
- Provider-specific optimizations and configurations
- OpenAI-compatible API integration patterns

### **Production Patterns** 🏭
- Middleware stacks for reliability and monitoring
- Error handling and retry logic
- Concurrent processing and performance optimization

### **Specialized Use Cases** ⚡
- Interactive applications and CLI tools
- Streaming responses and real-time processing
- Custom provider development

## 🔨 Building and Running

Each example is self-contained with its own `main.go` file:

```bash
# Run any example
cd examples/basic
go run main.go

# Build for distribution
go build -o example main.go
```

### Prerequisites
- Go 1.22+
- API keys for desired providers (OpenAI, Anthropic, etc.)
- Environment variables or direct configuration

## 📚 Documentation Context

These examples complement the comprehensive documentation in [`docs/`](../docs/):

- **[docs/quick-start.md](../docs/quick-start.md)** - Getting started guide
- **[docs/provider-guide.md](../docs/provider-guide.md)** - Provider configuration
- **[docs/advanced-features.md](../docs/advanced-features.md)** - Enterprise patterns
- **[docs/performance-benchmarks.md](../docs/performance-benchmarks.md)** - Performance analysis

## 🎯 Project Evolution

Wormhole has evolved from initial concept to production-ready SDK:

- **v1.0.0** - Initial release with core provider support
- **v1.2.0** - Architectural improvements and thread safety
- **v1.3.1** - Current version with comprehensive middleware and enterprise features

### Historical Context
Originally developed as part of the Meesix AI platform, Wormhole emerged from the need for:
- **Performance**: 67ns core latency (165x faster than alternatives)  
- **Reliability**: Enterprise middleware stack with circuit breakers and retry logic
- **Flexibility**: Universal provider interface supporting 6+ AI providers
- **Developer Experience**: Laravel-inspired SimpleFactory pattern for ease of use

## ⚠️ Important Notes

### Environment Setup
Most examples require API keys:
```bash
export OPENAI_API_KEY="your-key-here"
export ANTHROPIC_API_KEY="your-key-here"  
export OPENROUTER_API_KEY="your-key-here"
```

### Example Naming Convention
- **Descriptive names**: Clear purpose indication (e.g., `streaming-demo`, `interactive-chat`)
- **No quirky themes**: Professional naming for enterprise adoption
- **Consistent structure**: Each example follows the same organizational pattern

### Build Artifacts
Examples may create binaries when built. These are:
- **Not committed to git** (prevented by .gitignore)
- **User-generated** - build examples yourself from source
- **Platform-specific** - rebuild for your target environment

## 🚀 Contributing Examples

When adding new examples:
1. **Follow naming convention**: descriptive, professional names
2. **Include comprehensive comments**: Explain what and why
3. **Handle errors properly**: Use wormhole error patterns
4. **Test thoroughly**: Verify functionality before submission
5. **Document purpose**: Update this README with clear description

---

*Wormhole: Bending spacetime to reach AI models instantly since 2025*
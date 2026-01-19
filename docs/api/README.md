# API Reference

This guide provides a comprehensive overview of the Wormhole SDK API. For detailed function signatures and type definitions, see the official [pkg.go.dev documentation](https://pkg.go.dev/github.com/garyblankenship/wormhole).

## Package Structure

```
github.com/garyblankenship/wormhole
├── pkg/
│   ├── wormhole/       # Main client and request builders
│   ├── types/          # Shared types and interfaces
│   ├── providers/      # Provider implementations
│   ├── middleware/     # HTTP and provider middleware
│   ├── discovery/      # Dynamic model discovery
│   └── testing/        # Testing utilities and mocks
```

## Key Types

### Core Client

| Type | Description |
|------|-------------|
| `Wormhole` | Main client for interacting with LLM providers |
| `Config` | Client configuration options |
| `SimpleFactory` | Laravel-inspired factory for quick client creation |

### Request Builders

| Builder | Description |
|---------|-------------|
| `TextRequestBuilder` | Text generation with fluent builder pattern |
| `StructuredRequestBuilder` | Structured JSON output generation |
| `EmbeddingsRequestBuilder` | Vector embeddings generation |
| `ImageRequestBuilder` | Image generation |
| `AudioRequestBuilder` | Audio transcription and generation |
| `BatchBuilder` | Concurrent batch request execution |

### Response Types

| Type | Description |
|------|-------------|
| `TextResponse` | Text generation response with metadata |
| `TextChunk` | Streaming text chunk |
| `EmbeddingsResponse` | Vector embeddings response |
| `StructuredResponse` | Structured JSON output response |

### Provider Types

| Type | Description |
|------|-------------|
| `Provider` | Interface for LLM provider implementations |
| `ProviderConfig` | Provider-specific configuration |
| `Capabilities` | Provider capability flags (streaming, tools, etc.) |
| `ModelInfo` | Discovered model information |

## Key Functions

### Client Creation

| Function | Description |
|----------|-------------|
| `New(opts ...Option) *Wormhole` | Create client with functional options |
| `Quick.OpenAI(apiKey) *Wormhole` | Quick OpenAI client |
| `Quick.Anthropic(apiKey) *Wormhole` | Quick Anthropic client |
| `Quick.Gemini(apiKey) *Wormhole` | Quick Gemini client |
| `QuickOpenAI(apiKey) *Wormhole` | Convenience function for OpenAI |

### Request Building

| Function | Description |
|----------|-------------|
| `client.Text() *TextRequestBuilder` | Create text generation builder |
| `client.Structured() *StructuredRequestBuilder` | Create structured output builder |
| `client.Embeddings() *EmbeddingsRequestBuilder` | Create embeddings builder |
| `client.Image() *ImageRequestBuilder` | Create image generation builder |
| `client.Audio() *AudioRequestBuilder` | Create audio builder |
| `client.Batch() *BatchBuilder` | Create batch request builder |

### Provider Management

| Function | Description |
|----------|-------------|
| `client.Provider(name) (Provider, error)` | Get provider instance |
| `client.ProviderWithHandle(name) (*ProviderHandle, error)` | Get provider with reference counting |
| `client.ProviderCapabilities(name) *Capabilities` | Query provider capabilities |

### Tool Registration

| Function | Description |
|----------|-------------|
| `client.RegisterTool(name, description, schema, handler)` | Register a tool |
| `client.UnregisterTool(name) error` | Remove a tool |
| `client.ListTools() []Tool` | List all registered tools |
| `client.HasTool(name) bool` | Check if tool exists |
| `client.ToolCount() int` | Count registered tools |

### Model Discovery

| Function | Description |
|----------|-------------|
| `client.ListAvailableModels(provider) ([]*ModelInfo, error)` | List provider's models |
| `client.RefreshModels() error` | Refresh model cache |
| `client.ClearModelCache()` | Clear cached models |

### Lifecycle

| Function | Description |
|----------|-------------|
| `client.Close() error` | Close client and cleanup resources |
| `client.Shutdown(ctx) error` | Graceful shutdown with request draining |
| `client.IsShuttingDown() bool` | Check shutdown status |

## Configuration Options

| Option | Description |
|--------|-------------|
| `WithOpenAI(apiKey)` | Configure OpenAI provider |
| `WithAnthropic(apiKey)` | Configure Anthropic provider |
| `WithGemini(apiKey)` | Configure Gemini provider |
| `WithDefaultProvider(name)` | Set default provider |
| `WithMiddleware(...)` | Add HTTP middleware |
| `WithProviderMiddleware(...)` | Add type-safe provider middleware |
| `WithDebugLogging(logger)` | Enable debug logging |
| `WithModelValidation(enabled)` | Enable model validation |

## Builder Methods

All request builders support these common methods:

| Method | Description |
|--------|-------------|
| `Model(name)` | Set model name |
| `Provider(name)` | Override provider |
| `BaseURL(url)` | Set custom base URL |
| `APIKey(key)` | Set API key |
| `Temperature(n)` | Set temperature (0-2) |
| `MaxTokens(n)` | Set max tokens |
| `TopP(n)` | Set top-p sampling |
| `Validate()` | Validate configuration |
| `MustValidate()` | Validate or panic |

## See Also

- [Quick Start Guide](../getting-started.md)
- [Examples](../examples/)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/garyblankenship/wormhole)

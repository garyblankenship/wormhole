# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

All commands are defined in the Makefile:

- `make all` - Full build pipeline: format, lint, test, and build
- `make test` - Run all tests with coverage
- `make test-coverage` - Generate HTML coverage report
- `make lint` - Run golangci-lint (install with: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)
- `make fmt` - Format all Go code
- `make build` - Build all packages
- `make example` - Run the main example in cmd/example/main.go
- `make clean` - Clean artifacts and go clean

For individual tests:
- `go test ./pkg/prism/...` - Test core prism package
- `go test ./pkg/providers/openai/...` - Test specific provider
- `go test -run TestSpecificFunction ./...` - Run specific test

## Architecture Overview

Prism Go is a unified LLM provider SDK with a Laravel-inspired builder pattern architecture:

### Core Components

**Main Client (`pkg/prism/prism.go`)**
- `Prism` struct: Main client with provider registry and configuration
- Builder factories: `.Text()`, `.Structured()`, `.Embeddings()`, `.Image()`, `.Audio()`
- Provider management with lazy initialization and caching

**Provider System (`pkg/providers/`)**
- All providers implement the unified `types.Provider` interface
- Each provider has its own package: `openai/`, `anthropic/`, `gemini/`, `groq/`, `mistral/`, `ollama/`
- OpenAI-compatible providers in `openai_compatible/` support LMStudio, vLLM, generic APIs
- Transform layer converts between Prism types and provider-specific APIs

**Type System (`pkg/types/`)**
- Unified request/response types across all providers
- Provider interface definitions with capability-based sub-interfaces
- Message types, tool definitions, and schema structures

**Builder Pattern**
- Fluent API with method chaining: `.Model().Prompt().Temperature().Generate()`
- Each modality has its own builder (TextRequestBuilder, StructuredRequestBuilder, etc.)
- Provider selection via `.Using("provider")` or falls back to default

### Key Patterns

**Provider Registration**
```go
// Via config
p := prism.New(prism.Config{
    DefaultProvider: "openai",
    Providers: map[string]types.ProviderConfig{...}
})

// Via fluent methods
p.WithOpenAI("key").WithAnthropic("key").WithGemini("key")
```

**Request Building**
```go
response, err := p.Text().
    Using("anthropic").        // Optional provider override
    Model("claude-3-opus").    // Required model
    Messages(messages...).     // Messages or Prompt()
    MaxTokens(100).           // Optional parameters
    Temperature(0.7).
    Generate(ctx)             // Execute request
```

**Provider Capabilities**
- Not all providers support all modalities (see README.md compatibility matrix)
- Providers implement subset interfaces (TextProvider, EmbeddingsProvider, etc.)
- Request builders handle provider capability validation

### Testing Infrastructure

**Mock Provider (`pkg/testing/mock_provider.go`)**
- Implements full Provider interface for testing
- Configurable responses, streaming chunks, errors
- Use `NewMockProvider().WithTextResponse().WithError()` pattern

**Example Structure**
- `examples/` directory contains working examples for each provider
- `cmd/example/main.go` - Main demonstration
- Each example shows provider-specific configuration and usage patterns

### Code Conventions

- Use errors.New(), errors.Wrap(), not fmt.Errorf for error handling
- Context.Context required for all provider operations
- Channel-based streaming with proper cleanup
- Structured logging preferred over fmt.Print for examples
- Provider-specific transform.go files handle API translation

### Development Workflow

1. Run `make fmt lint` before committing
2. Add tests for new providers/features
3. Update examples/ when adding new capabilities
4. Provider implementations must handle context cancellation
5. All public APIs should include comprehensive godoc comments
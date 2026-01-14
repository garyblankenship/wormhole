# Wormhole SDK Architecture Flow

## Overview
Wormhole SDK is a unified LLM provider abstraction layer written in Go. It provides a consistent interface for interacting with multiple LLM providers (OpenAI, Anthropic, Gemini, Ollama, OpenRouter) while maintaining provider-specific capabilities and optimizations.

## Core Architecture Components

### 1. Wormhole Client (`pkg/wormhole/wormhole.go`)
**Primary entry point**: The `Wormhole` struct manages provider lifecycle, caching, and request routing.

**Key responsibilities**:
- Provider factory registration and instantiation
- Provider caching with reference counting
- Request builder creation (Text, Structured, Embeddings, Image, Audio, Batch)
- Tool registry management
- Model discovery service coordination
- Configuration validation and defaults

**Concurrency patterns**:
- `sync.RWMutex` for provider map access
- Reference counting for cached providers
- `sync.Once` for idempotent close operations
- Stale provider cleanup with LRU-like logic

### 2. Provider Abstraction Layer (`pkg/types/provider.go`)
**Interface hierarchy**:
- `Provider` interface: Unified interface with default "not implemented" implementations
- `BaseProvider` struct: Embedded by concrete providers for common functionality
- Provider-specific implementations in `pkg/providers/`

**Request/Response types**:
- Strongly typed request/response structures for each operation type
- Common message types (`Message`, `Conversation`) for chat interfaces
- Error standardization with `WormholeError` type

### 3. Provider Implementations (`pkg/providers/`)
**Base provider** (`pkg/providers/base.go`):
- HTTP client management with TLS configuration
- Retry logic integration via `RetryableHTTPClient`
- Common error handling and response parsing
- API key validation and masking

**Concrete providers**:
- OpenAI (`openai/`): OpenAI API compatibility
- Anthropic (`anthropic/`): Claude API support
- Gemini (`gemini/`): Google Gemini API
- Ollama (`ollama/`): Local Ollama server
- OpenRouter (`openrouter/`): OpenRouter proxy service

### 4. Middleware System (`pkg/middleware/`)
**Two middleware systems**:
1. **Type-safe middleware** (`types.ProviderMiddlewareChain`): Method-specific middleware with compile-time safety
2. **Generic middleware** (`middleware.Chain`): Legacy system using `any` types (deprecated)

**Available middleware**:
- Metrics: Request timing and success/failure tracking
- Logging: Request/response logging with structured output
- Timeout: Context timeout enforcement
- Circuit breaker: Failure detection and automatic disablement
- Rate limiting: Request rate control
- Load balancing: Provider selection and failover
- Caching: Response caching with TTL

### 5. Request Builder Pattern (`pkg/wormhole/*_builder.go`)
**Builder types**:
- `TextRequestBuilder`: Text generation with conversation support
- `StructuredRequestBuilder`: Structured output generation
- `EmbeddingsRequestBuilder`: Vector embeddings
- `ImageRequestBuilder`: Image generation
- `AudioRequestBuilder`: Audio transcription/synthesis
- `BatchBuilder`: Concurrent batch execution

**Builder features**:
- Fluent API with method chaining
- Immutable cloning for configuration reuse
- Provider override support
- Base URL customization for OpenAI-compatible APIs

### 6. Tool Execution System (`pkg/wormhole/tool_*.go`)
**Components**:
- `ToolRegistry`: Tool registration and lookup
- `ToolExecutor`: Tool execution with safety controls
- `ToolSafetyConfig`: Configurable safety limits

**Safety features**:
- Concurrency limiting
- Circuit breaker for error detection
- Argument schema validation
- Retry logic for transient failures

### 7. Model Discovery Service (`pkg/discovery/`)
**Dynamic model catalog**:
- Background model fetching from provider APIs
- Caching with TTL and stale-while-revalidate
- Parallel model fetching across providers
- Offline mode support

**Fetchers**:
- Provider-specific fetchers in `pkg/discovery/fetchers/`
- Support for OpenAI, Anthropic, Ollama, OpenRouter
- Configurable refresh intervals

## Data Flow

### Standard Request Flow
```
User Code → Request Builder → Wormhole Client → Provider Cache → Provider Instance → HTTP Client → LLM API
```

### Provider Resolution Flow
```
1. Request specifies provider (or uses default)
2. Check provider cache (read lock)
3. If cached: increment ref count, return provider
4. If not cached: acquire write lock, create provider via factory
5. Cache provider, set ref count = 1
6. Return provider
```

### Middleware Application Flow
```
1. Request builder creates typed request
2. Wormhole gets provider
3. Apply type-safe middleware chain (if configured)
4. Call provider method through middleware wrappers
5. Middleware processes request/response
6. Return to caller
```

### Error Handling Flow
```
1. Provider HTTP layer catches errors
2. Map HTTP status codes to Wormhole error codes
3. Wrap with provider context
4. Middleware can add additional context
5. Return structured WormholeError to caller
```

## Concurrency Patterns

### Provider Caching
- Double-checked locking pattern for thread-safe provider creation
- Reference counting for shared provider instances
- Stale provider cleanup based on last-used timestamp

### Batch Execution
- Semaphore-based concurrency limiting
- WaitGroup for request completion tracking
- Context cancellation propagation
- Result ordering preservation

### Discovery Service
- Background goroutine with ticker for periodic refresh
- Parallel model fetching with error channel
- Graceful shutdown with wait group
- Cache staleness detection

### Tool Execution
- Concurrency limiter for parallel tool calls
- Circuit breaker for error rate detection
- Retry executor with exponential backoff

## Memory Management

### Allocation Patterns
- Request builders reuse request objects via pools
- Response parsing minimizes allocations
- Streaming responses use channels rather than buffers
- Tool execution validates schemas without deep copying

### Caching Strategy
- Provider instances cached until stale/unused
- Model discovery cache with TTL
- Response caching via middleware (optional)
- Tool registry uses map for O(1) lookups

## Performance Considerations

### Hot Paths
1. **Provider resolution**: Double-checked locking with RWMutex
2. **HTTP request execution**: Retry logic with exponential backoff
3. **Response parsing**: JSON unmarshaling with type safety
4. **Middleware chain**: Minimal overhead for common operations

### Optimization Opportunities
- Provider pooling for high-concurrency scenarios
- Request/response object pooling
- Connection pooling at HTTP client level
- Lazy initialization of expensive resources

## Security Considerations

### API Key Protection
- Key format validation on configuration
- Key masking in error messages and logs
- TLS configuration with secure defaults
- Provider-specific authentication headers

### Input Validation
- Tool argument schema validation
- Request parameter bounds checking
- Content length limits for large inputs
- Structured output schema enforcement

### Safety Controls
- Maximum tool execution iterations
- Timeout enforcement at multiple levels
- Concurrency limits for parallel operations
- Circuit breakers for error containment

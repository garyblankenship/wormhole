package wormhole

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Wormhole is the main client for interacting with LLM providers
type Wormhole struct {
	providerFactories  map[string]types.ProviderFactory // Factory functions for creating providers
	providers          map[string]*cachedProvider       // Cached provider instances with ref counting
	providersMutex     sync.RWMutex
	config             Config
	providerMiddleware *types.ProviderMiddlewareChain // Type-safe middleware chain
	toolRegistry       *ToolRegistry                  // Registry of available tools for function calling
	discoveryService   *discovery.DiscoveryService    // Dynamic model discovery service

	// Cache metrics
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
	cacheEvictions atomic.Int64

	// Adaptive concurrency control
	adaptiveLimiter *EnhancedAdaptiveLimiter

	// Shutdown management
	shutdownOnce   sync.Once
	shutdownChan   chan struct{}  // Signal for graceful shutdown
	activeRequests sync.WaitGroup // Track in-flight requests
	shuttingDown   atomic.Bool    // Atomic flag for shutdown state

	// Idempotency cache
	idempotencyMu    sync.Mutex
	idempotencyCache map[string]*idempotencyEntry
}

// IdempotencyConfig holds configuration for idempotent request handling
type IdempotencyConfig struct {
	// Key is the unique identifier for the operation
	Key string
	// TTL is the time-to-live for cached responses
	TTL time.Duration
}

// Config holds the configuration for Wormhole
type Config struct {
	DefaultProvider     string
	Providers           map[string]types.ProviderConfig
	CustomFactories     map[string]types.ProviderFactory
	ProviderMiddlewares []types.ProviderMiddleware // Type-safe middleware
	Middleware          []middleware.Middleware    // DEPRECATED: use ProviderMiddlewares instead
	DebugLogging        bool
	Logger              types.Logger
	DefaultTimeout      time.Duration
	DefaultRetries      int
	DefaultRetryDelay   time.Duration
	ModelValidation     bool                      // Whether to validate models against registry (default: true)
	DiscoveryConfig     discovery.DiscoveryConfig // Dynamic model discovery configuration
	EnableDiscovery     bool                      // Whether to enable dynamic model discovery (default: true)
	Idempotency         *IdempotencyConfig        // Idempotency configuration for duplicate prevention
}

// New creates a new Wormhole instance using functional options
func New(opts ...Option) *Wormhole {
	// CRITICAL: Register built-in models FIRST before any model validation
	// No model pre-registration - providers handle model validation at request time

	// Start with a default config
	config := Config{
		Providers:       make(map[string]types.ProviderConfig),
		CustomFactories: make(map[string]types.ProviderFactory),
		ModelValidation: true,                      // Enable model validation by default
		EnableDiscovery: true,                      // Enable model discovery by default
		DiscoveryConfig: discovery.DefaultConfig(), // Use default discovery configuration
	}

	// Apply all provided options
	for _, opt := range opts {
		opt(&config)
	}

	// Create client with final, immutable config
	p := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers:         make(map[string]*cachedProvider),
		config:            config,
		toolRegistry:      NewToolRegistry(),
		shutdownChan:      make(chan struct{}),
		idempotencyCache:  make(map[string]*idempotencyEntry),
	}

	// Initialize model discovery service if enabled
	if config.EnableDiscovery {
		p.initializeDiscoveryService()
	}

	// Pre-register all built-in providers
	p.registerBuiltinProviders()

	// Register custom factories from config
	for name, factory := range config.CustomFactories {
		p.providerFactories[name] = factory
	}

	// Validate configuration and log warnings
	if config.DebugLogging && config.Logger != nil {
		warnings := validateConfig(&config)
		for _, warning := range warnings {
			config.Logger.Warn("Config warning", "warning", warning)
		}
	}

	// Initialize type-safe provider middleware chain
	var providerMiddlewares []types.ProviderMiddleware

	// Add debug logging if enabled
	if config.DebugLogging && config.Logger != nil {
		providerMiddlewares = append(providerMiddlewares, middleware.NewDebugTypedLoggingMiddleware(config.Logger))
	}

	// Add user-provided provider middlewares
	providerMiddlewares = append(providerMiddlewares, config.ProviderMiddlewares...)

	if len(providerMiddlewares) > 0 {
		p.providerMiddleware = types.NewProviderChain(providerMiddlewares...)
	}

	// Legacy middleware support (deprecated)
	// Note: Legacy middleware is automatically converted to type-safe middleware
	// via WithMiddleware() option. The middlewareChain is no longer created
	// as all middleware execution happens through providerMiddleware.

	return p
}

// Text creates a new text generation request builder
func (p *Wormhole) Text() *TextRequestBuilder {
	return &TextRequestBuilder{
		CommonBuilder: newCommonBuilder(p),
		request:       getTextRequest(),
	}
}

// Structured creates a new structured output request builder
func (p *Wormhole) Structured() *StructuredRequestBuilder {
	return &StructuredRequestBuilder{
		CommonBuilder: newCommonBuilder(p),
		request:       getStructuredRequest(),
	}
}

// Embeddings creates a new embeddings request builder
func (p *Wormhole) Embeddings() *EmbeddingsRequestBuilder {
	return &EmbeddingsRequestBuilder{
		CommonBuilder: newCommonBuilder(p),
		request:       getEmbeddingsRequest(),
	}
}

// Image creates a new image generation request builder
func (p *Wormhole) Image() *ImageRequestBuilder {
	return &ImageRequestBuilder{
		CommonBuilder: newCommonBuilder(p),
		request:       getImageRequest(),
	}
}

// Audio creates a new audio request builder
func (p *Wormhole) Audio() *AudioRequestBuilder {
	return &AudioRequestBuilder{
		wormhole: p,
	}
}

// Batch creates a new batch request builder for concurrent execution.
//
// Example:
//
//	results := client.Batch().
//	    Add(client.Text().Model("gpt-4o").Prompt("Q1")).
//	    Add(client.Text().Model("gpt-4o").Prompt("Q2")).
//	    Concurrency(5).
//	    Execute(ctx)
func (p *Wormhole) Batch() *BatchBuilder {
	return &BatchBuilder{
		wormhole:    p,
		concurrency: 10, // Default concurrency
	}
}

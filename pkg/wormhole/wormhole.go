package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/discovery/fetchers"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/providers/anthropic"
	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/providers/ollama"
	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Provider name constants
const (
	providerOpenAI    = "openai"
	providerAnthropic = "anthropic"
)

// Wormhole is the main client for interacting with LLM providers
type Wormhole struct {
	providerFactories  map[string]types.ProviderFactory // Factory functions for creating providers
	providers          map[string]types.Provider        // Cached provider instances
	providersMutex     sync.RWMutex
	config             Config
	providerMiddleware *types.ProviderMiddlewareChain // Type-safe middleware chain
	middlewareChain    *middleware.Chain              // DEPRECATED: use providerMiddleware instead
	toolRegistry       *ToolRegistry                  // Registry of available tools for function calling
	discoveryService   *discovery.DiscoveryService    // Dynamic model discovery service
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
		providers:         make(map[string]types.Provider),
		config:            config,
		toolRegistry:      NewToolRegistry(),
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
	var middlewares []middleware.Middleware
	middlewares = append(middlewares, config.Middleware...)
	if len(middlewares) > 0 {
		p.middlewareChain = middleware.NewChain(middlewares...)
	}

	return p
}

// registerBuiltinProviders pre-registers all built-in provider factories
func (p *Wormhole) registerBuiltinProviders() {
	p.providerFactories["openai"] = func(c types.ProviderConfig) (types.Provider, error) {
		return openai.New(c), nil
	}
	p.providerFactories["anthropic"] = func(c types.ProviderConfig) (types.Provider, error) {
		return anthropic.New(c), nil
	}
	p.providerFactories["gemini"] = func(c types.ProviderConfig) (types.Provider, error) {
		return gemini.New(c.APIKey, c), nil
	}
	p.providerFactories["ollama"] = func(c types.ProviderConfig) (types.Provider, error) {
		return ollama.New(c), nil
	}
	// NOTE: groq and mistral now use the generic openai provider via WithOpenAICompatible()
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

// Provider returns a specific provider instance
func (p *Wormhole) Provider(name string) (types.Provider, error) {
	// First, try to read with read lock
	p.providersMutex.RLock()
	if provider, exists := p.providers[name]; exists {
		p.providersMutex.RUnlock()
		return provider, nil
	}
	p.providersMutex.RUnlock()

	// Provider doesn't exist, need to create it with write lock
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if provider, exists := p.providers[name]; exists {
		return provider, nil
	}

	// Get the factory for the requested provider
	factory, exists := p.providerFactories[name]
	if !exists {
		// Check if it's a custom provider added via With... methods
		if _, configExists := p.config.Providers[name]; configExists {
			// Assume it's an OpenAI-compatible provider for backward compatibility
			factory = func(c types.ProviderConfig) (types.Provider, error) {
				return openai.New(c), nil
			}
		} else {
			return nil, fmt.Errorf("unknown or unregistered provider: %s", name)
		}
	}

	// Get provider config
	config, exists := p.config.Providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not configured", name)
	}

	// Apply DefaultTimeout if provider config doesn't have explicit timeout
	// Special case: if DefaultTimeout is 0 (unlimited), only apply to configs without explicit timeout
	if config.Timeout == 0 && p.config.DefaultTimeout != 0 {
		config.Timeout = int(p.config.DefaultTimeout.Seconds())
	} else if config.Timeout == 0 && p.config.DefaultTimeout == 0 {
		// Unlimited timeout requested - only apply when provider config is also 0
		config.Timeout = 0
	}

	// Use the factory to create the provider instance
	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %s: %w", name, err)
	}

	// Cache the provider
	p.providers[name] = provider
	return provider, nil
}

// getProvider returns the provider to use for a request
func (p *Wormhole) getProvider(override string) (types.Provider, error) {
	providerName := override
	if providerName == "" {
		providerName = p.config.DefaultProvider
	}
	if providerName == "" {
		return nil, fmt.Errorf("no provider specified and no default provider configured")
	}
	return p.Provider(providerName)
}

// createOpenAICompatibleProvider creates a temporary OpenAI provider with custom config
func (p *Wormhole) createOpenAICompatibleProvider(config types.ProviderConfig) (types.Provider, error) {
	// Create temporary OpenAI provider with custom BaseURL
	openAIProvider := openai.New(config)
	return openAIProvider, nil
}

// ==================== Tool Registration API ====================

// RegisterTool registers a new tool that can be called by LLMs.
// Tools are registered globally at the client level and are available to all requests.
//
// Parameters:
//   - name: The unique name of the tool (used by the LLM to call it)
//   - description: A clear description of what the tool does (helps the LLM decide when to use it)
//   - schema: The JSON schema for the tool's input parameters
//   - handler: The function that executes when the LLM calls this tool
//
// Example:
//
//	client.RegisterTool(
//	    "get_weather",
//	    "Get the current weather for a given city",
//	    types.ObjectSchema{
//	        Type: "object",
//	        Properties: map[string]types.Schema{
//	            "city": types.StringSchema{Type: "string"},
//	            "unit": types.StringSchema{Type: "string", Enum: []string{"celsius", "fahrenheit"}},
//	        },
//	        Required: []string{"city"},
//	    },
//	    func(ctx context.Context, args map[string]any) (any, error) {
//	        city := args["city"].(string)
//	        // ... fetch weather data ...
//	        return map[string]any{"temp": 72, "condition": "sunny"}, nil
//	    },
//	)
func (p *Wormhole) RegisterTool(name string, description string, schema types.Schema, handler types.ToolHandler) {
	// Convert schema to map[string]any for Tool.InputSchema
	var schemaMap map[string]any

	// If schema is already a map, use it directly
	if m, ok := schema.(map[string]any); ok {
		schemaMap = m
	} else {
		// Otherwise, marshal and unmarshal to convert to map
		// This handles our Schema types (ObjectSchema, StringSchema, etc.)
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			// Fallback to empty map
			schemaMap = make(map[string]any)
		} else {
			if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
				schemaMap = make(map[string]any)
			}
		}
	}

	tool := types.Tool{
		Type:        "function",
		Name:        name,
		Description: description,
		InputSchema: schemaMap,
		Function: &types.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  schemaMap,
		},
	}

	definition := types.NewToolDefinition(tool, handler)
	p.toolRegistry.Register(name, definition)
}

// UnregisterTool removes a tool from the registry.
// Returns an error if the tool doesn't exist.
func (p *Wormhole) UnregisterTool(name string) error {
	return p.toolRegistry.Unregister(name)
}

// ListTools returns all registered tools (without their handlers).
// This is useful for inspecting which tools are available.
func (p *Wormhole) ListTools() []types.Tool {
	return p.toolRegistry.List()
}

// HasTool checks if a tool with the given name is registered.
func (p *Wormhole) HasTool(name string) bool {
	return p.toolRegistry.Has(name)
}

// ToolCount returns the number of registered tools.
func (p *Wormhole) ToolCount() int {
	return p.toolRegistry.Count()
}

// ClearTools removes all registered tools.
func (p *Wormhole) ClearTools() {
	p.toolRegistry.Clear()
}

// ==================== Model Discovery API ====================

// initializeDiscoveryService creates and configures the model discovery service
func (p *Wormhole) initializeDiscoveryService() {
	var modelFetchers []discovery.ModelFetcher

	// Create fetchers for configured providers
	for providerName, providerConfig := range p.config.Providers {
		switch providerName {
		case providerOpenAI:
			if providerConfig.APIKey != "" {
				modelFetchers = append(modelFetchers, fetchers.NewOpenAIFetcher(providerConfig.APIKey))
			}
		case providerAnthropic:
			if providerConfig.APIKey != "" {
				modelFetchers = append(modelFetchers, fetchers.NewAnthropicFetcher(providerConfig.APIKey))
			}
		case "ollama":
			baseURL := providerConfig.BaseURL
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}
			modelFetchers = append(modelFetchers, fetchers.NewOllamaFetcher(baseURL))
		}
	}

	// Always include OpenRouter (public endpoint, no auth required)
	modelFetchers = append(modelFetchers, fetchers.NewOpenRouterFetcher())

	// Create discovery service with fetchers
	p.discoveryService = discovery.NewDiscoveryService(p.config.DiscoveryConfig, modelFetchers...)

	// Start background refresh if not in offline mode
	if !p.config.DiscoveryConfig.OfflineMode && p.config.DiscoveryConfig.RefreshInterval > 0 {
		p.discoveryService.StartBackgroundRefresh(context.Background())
	}
}

// ListAvailableModels returns all available models for a provider from the discovery cache.
// If the cache is empty, it will fetch models from the provider's API.
//
// Parameters:
//   - provider: The provider name (e.g., "openai", "anthropic", "openrouter")
//
// Returns:
//   - A slice of ModelInfo containing all available models
//   - An error if the provider is not configured or discovery fails
//
// Example:
//
//	models, err := client.ListAvailableModels("openai")
//	if err != nil {
//	    return err
//	}
//	for _, model := range models {
//	    fmt.Printf("Model: %s (%s) - Capabilities: %v\n", model.Name, model.ID, model.Capabilities)
//	}
func (p *Wormhole) ListAvailableModels(provider string) ([]*types.ModelInfo, error) {
	if p.discoveryService == nil {
		return nil, fmt.Errorf("model discovery is not enabled")
	}
	return p.discoveryService.GetModels(context.Background(), provider)
}

// RefreshModels manually triggers a refresh of all provider model catalogs.
// This bypasses the cache and fetches fresh data from all configured providers.
//
// Returns:
//   - An error if any provider fails to refresh (partial failures are collected)
//
// Example:
//
//	if err := client.RefreshModels(); err != nil {
//	    log.Printf("Some providers failed to refresh: %v", err)
//	}
func (p *Wormhole) RefreshModels() error {
	if p.discoveryService == nil {
		return fmt.Errorf("model discovery is not enabled")
	}
	return p.discoveryService.RefreshModels(context.Background())
}

// ClearModelCache clears all cached model data.
// This will force fresh fetches on the next model lookup.
//
// Example:
//
//	client.ClearModelCache()
func (p *Wormhole) ClearModelCache() {
	if p.discoveryService != nil {
		p.discoveryService.ClearCache()
	}
}

// StopModelDiscovery stops the background model refresh goroutine.
// Call this when shutting down the client to clean up resources.
//
// Example:
//
//	defer client.StopModelDiscovery()
func (p *Wormhole) StopModelDiscovery() {
	if p.discoveryService != nil {
		p.discoveryService.Stop()
	}
}

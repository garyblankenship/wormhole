package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"os"
	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/discovery/fetchers"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/providers/anthropic"
	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/providers/ollama"
	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// validateAPIKey performs basic format validation for provider API keys
func validateAPIKey(provider, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("empty API key for provider %s", provider)
	}

	// Skip validation for test/mock keys (common in testing)
	if strings.HasPrefix(apiKey, "test-") || strings.HasPrefix(apiKey, "mock-") || strings.HasPrefix(apiKey, "dummy-") {
		return nil
	}

	// Provider-specific validation
	switch provider {
	case "openai":
		if !strings.HasPrefix(apiKey, "sk-") && !strings.HasPrefix(apiKey, "org-") {
			return fmt.Errorf("invalid OpenAI API key format, expected 'sk-' or 'org-' prefix")
		}
	case "anthropic":
		if !strings.HasPrefix(apiKey, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format, expected 'sk-ant-' prefix")
		}
	case "gemini":
		// Google AI Studio keys don't have fixed format, just check length
		if len(apiKey) < 10 {
			return fmt.Errorf("API key for Google AI Studio is too short (minimum 10 characters)")
		}
	case "openrouter":
		if !strings.HasPrefix(apiKey, "sk-or-") {
			return fmt.Errorf("invalid OpenRouter API key format, expected 'sk-or-' prefix")
		}
		// For other providers like ollama, groq, mistral, etc., just basic validation
	}
	return nil
}

// Provider name constants
const (
	providerOpenAI    = "openai"
	providerAnthropic = "anthropic"
)

// cachedProvider wraps a provider with reference counting and last-used tracking
type cachedProvider struct {
	provider types.Provider
	lastUsed time.Time
	refCount int
	mu       sync.RWMutex
}

// Wormhole is the main client for interacting with LLM providers
type Wormhole struct {
	providerFactories  map[string]types.ProviderFactory // Factory functions for creating providers
	providers          map[string]*cachedProvider       // Cached provider instances with ref counting
	providersMutex     sync.RWMutex
	closeOnce          sync.Once // Ensures Close() is idempotent
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
		providers:         make(map[string]*cachedProvider),
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
		// Check environment variable for Ollama base URL if not set in config
		if c.BaseURL == "" {
			if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
				c.BaseURL = envURL
			}
		}
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

// Provider returns a specific provider instance
func (p *Wormhole) Provider(name string) (types.Provider, error) {
	// First, try to read with read lock
	p.providersMutex.RLock()
	if cp, exists := p.providers[name]; exists {
		// Increment reference count and update last used time
		cp.mu.Lock()
		cp.refCount++
		cp.lastUsed = time.Now()
		cp.mu.Unlock()
		p.providersMutex.RUnlock()
		return cp.provider, nil
	}
	p.providersMutex.RUnlock()

	// Provider doesn't exist, need to create it with write lock
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if cp, exists := p.providers[name]; exists {
		// Increment reference count and update last used time
		cp.mu.Lock()
		cp.refCount++
		cp.lastUsed = time.Now()
		cp.mu.Unlock()
		return cp.provider, nil
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
			return nil, types.ErrProviderNotFound.
				WithProvider(name).
				WithDetails(p.formatProviderHint(name))
		}
	}

	// Get provider config
	config, exists := p.config.Providers[name]
	if !exists {
		return nil, types.ErrProviderNotFound.
			WithProvider(name).
			WithDetails(p.formatProviderHint(name))
	}

	// Validate API key format before creating provider
	if config.APIKey != "" {
		if err := validateAPIKey(name, config.APIKey); err != nil {
			return nil, fmt.Errorf("invalid API key for provider %s: %w", name, err)
		}
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

	// Create cached provider wrapper
	cp := &cachedProvider{
		provider: provider,
		lastUsed: time.Now(),
		refCount: 1,
	}

	// Cache the provider
	p.providers[name] = cp
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

// formatProviderHint returns a helpful error message with configured providers listed
func (p *Wormhole) formatProviderHint(requested string) string {
	configured := p.getConfiguredProviders()
	if len(configured) == 0 {
		return fmt.Sprintf("provider '%s' not configured. No providers are configured - use wormhole.WithOpenAI(), wormhole.WithAnthropic(), etc.", requested)
	}
	return fmt.Sprintf("provider '%s' not configured. Available providers: %s. Use wormhole.With%s() to configure it.",
		requested,
		formatList(configured),
		capitalize(requested),
	)
}

// getConfiguredProviders returns a sorted list of configured provider names
func (p *Wormhole) getConfiguredProviders() []string {
	providers := make([]string, 0, len(p.config.Providers))
	for name := range p.config.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

// formatList formats a slice as a comma-separated list
func formatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	return fmt.Sprintf("%s", items)
}

// capitalize returns a string with the first letter capitalized
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:] // Simple ASCII uppercase for first char
}

// validateConfig checks the configuration for common mistakes and returns warnings.
// These are non-fatal issues that developers should be aware of.
func validateConfig(c *Config) []string {
	var warnings []string

	// Check if default provider is set but not configured
	if c.DefaultProvider != "" {
		if _, exists := c.Providers[c.DefaultProvider]; !exists {
			warnings = append(warnings, fmt.Sprintf(
				"DefaultProvider '%s' is set but not configured. Use wormhole.With%s() to configure it.",
				c.DefaultProvider, capitalize(c.DefaultProvider),
			))
		}
	}

	// Check for providers configured without API keys (excluding local providers)
	localProviders := map[string]bool{"ollama": true, "lmstudio": true, "vllm": true}
	for name, cfg := range c.Providers {
		if !localProviders[name] && cfg.APIKey == "" {
			warnings = append(warnings, fmt.Sprintf(
				"Provider '%s' is configured but has no API key. Requests will likely fail.",
				name,
			))
		}
	}

	// Check if no default provider is set with multiple providers configured
	if c.DefaultProvider == "" && len(c.Providers) > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"No DefaultProvider set but %d providers configured. Use WithDefaultProvider() or specify .Using() on each request.",
			len(c.Providers),
		))
	}

	return warnings
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
				if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
					baseURL = envURL
				} else {
					baseURL = "http://localhost:11434"
				}
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
		if err := p.discoveryService.Stop(); err != nil && p.config.Logger != nil {
			p.config.Logger.Warn("error stopping discovery service", "error", err)
		}
	}
}

// ==================== Provider Capability Helpers ====================

// Capability represents a provider/model capability
type Capability string

const (
	CapabilityText          Capability = "text"
	CapabilityStructured    Capability = "structured"
	CapabilityEmbeddings    Capability = "embeddings"
	CapabilityImages        Capability = "images"
	CapabilityAudio         Capability = "audio"
	CapabilityToolCalling   Capability = "tool_calling"
	CapabilityStreaming     Capability = "streaming"
	CapabilityVision        Capability = "vision"
	CapabilityCodeExecution Capability = "code_execution"
)

// ProviderCapabilities returns the capabilities supported by a provider.
// This is a static lookup based on known provider features.
//
// Example:
//
//	caps := client.ProviderCapabilities("openai")
//	if caps.Has(wormhole.CapabilityToolCalling) {
//	    // Use tool calling
//	}
func (p *Wormhole) ProviderCapabilities(provider string) *Capabilities {
	caps := &Capabilities{
		provider: provider,
		caps:     make(map[Capability]bool),
	}

	switch provider {
	case "openai":
		caps.caps[CapabilityText] = true
		caps.caps[CapabilityStructured] = true
		caps.caps[CapabilityEmbeddings] = true
		caps.caps[CapabilityImages] = true
		caps.caps[CapabilityAudio] = true
		caps.caps[CapabilityToolCalling] = true
		caps.caps[CapabilityStreaming] = true
		caps.caps[CapabilityVision] = true
	case "anthropic":
		caps.caps[CapabilityText] = true
		caps.caps[CapabilityStructured] = true
		caps.caps[CapabilityToolCalling] = true
		caps.caps[CapabilityStreaming] = true
		caps.caps[CapabilityVision] = true
		caps.caps[CapabilityCodeExecution] = true
	case "gemini":
		caps.caps[CapabilityText] = true
		caps.caps[CapabilityStructured] = true
		caps.caps[CapabilityEmbeddings] = true
		caps.caps[CapabilityImages] = true
		caps.caps[CapabilityToolCalling] = true
		caps.caps[CapabilityStreaming] = true
		caps.caps[CapabilityVision] = true
		caps.caps[CapabilityCodeExecution] = true
	case "ollama":
		caps.caps[CapabilityText] = true
		caps.caps[CapabilityEmbeddings] = true
		caps.caps[CapabilityStreaming] = true
	case "openrouter":
		// OpenRouter proxies to multiple providers, so it has broad capabilities
		caps.caps[CapabilityText] = true
		caps.caps[CapabilityStructured] = true
		caps.caps[CapabilityToolCalling] = true
		caps.caps[CapabilityStreaming] = true
		caps.caps[CapabilityVision] = true
	}

	return caps
}

// Capabilities holds the capabilities of a provider
type Capabilities struct {
	provider string
	caps     map[Capability]bool
}

// Has returns true if the capability is supported
func (c *Capabilities) Has(cap Capability) bool {
	if c == nil || c.caps == nil {
		return false
	}
	return c.caps[cap]
}

// All returns all supported capabilities
func (c *Capabilities) All() []Capability {
	if c == nil || c.caps == nil {
		return nil
	}
	var result []Capability
	for cap, supported := range c.caps {
		if supported {
			result = append(result, cap)
		}
	}
	return result
}

// SupportsText returns true if the provider supports text generation
func (c *Capabilities) SupportsText() bool {
	return c.Has(CapabilityText)
}

// SupportsStructured returns true if the provider supports structured output
func (c *Capabilities) SupportsStructured() bool {
	return c.Has(CapabilityStructured)
}

// SupportsEmbeddings returns true if the provider supports embeddings
func (c *Capabilities) SupportsEmbeddings() bool {
	return c.Has(CapabilityEmbeddings)
}

// SupportsToolCalling returns true if the provider supports function/tool calling
func (c *Capabilities) SupportsToolCalling() bool {
	return c.Has(CapabilityToolCalling)
}

// SupportsStreaming returns true if the provider supports streaming responses
func (c *Capabilities) SupportsStreaming() bool {
	return c.Has(CapabilityStreaming)
}

// SupportsVision returns true if the provider supports image/vision inputs
func (c *Capabilities) SupportsVision() bool {
	return c.Has(CapabilityVision)
}

// SupportsImages returns true if the provider supports image generation
func (c *Capabilities) SupportsImages() bool {
	return c.Has(CapabilityImages)
}

// SupportsAudio returns true if the provider supports audio generation/transcription
func (c *Capabilities) SupportsAudio() bool {
	return c.Has(CapabilityAudio)
}

// Close implements io.Closer interface for Wormhole
// It cleans up all cached providers and stops the discovery service
func (p *Wormhole) Close() error {
	var errs []error
	p.closeOnce.Do(func() {
		// Cleanup providers
		p.providersMutex.Lock()
		defer p.providersMutex.Unlock()
		for name, cp := range p.providers {
			if err := cp.provider.Close(); err != nil {
				errs = append(errs, fmt.Errorf("provider %s: %w", name, err))
			}
			delete(p.providers, name)
		}

		// Stop discovery service
		if p.discoveryService != nil {
			if err := p.discoveryService.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("discovery service: %w", err))
			}
		}

		// Cleanup tool registry resources if needed
		// (currently tool registry has no resources to clean up)
	})

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}
	return nil
}

// CleanupStaleProviders cleans up providers that haven't been used for a while
func (p *Wormhole) CleanupStaleProviders(maxAge time.Duration, maxCount int) {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	now := time.Now()
	staleKeys := []string{}
	for name, cp := range p.providers {
		cp.mu.RLock()
		if cp.refCount == 0 && now.Sub(cp.lastUsed) > maxAge {
			staleKeys = append(staleKeys, name)
		}
		cp.mu.RUnlock()
	}

	// Remove stale providers
	for _, name := range staleKeys {
		if cp, ok := p.providers[name]; ok {
			if err := cp.provider.Close(); err != nil && p.config.Logger != nil {
				p.config.Logger.Warn("error closing stale provider", "provider", name, "error", err)
			}
			delete(p.providers, name)
		}
	}

	// Enforce max count
	if len(p.providers) > maxCount {
		// Remove oldest unused providers
		// TODO: Implement LRU cleanup logic
	}
}

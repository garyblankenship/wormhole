package wormhole

import (
	"fmt"
	"sync"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/providers/anthropic"
	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/providers/groq"
	"github.com/garyblankenship/wormhole/pkg/providers/mistral"
	"github.com/garyblankenship/wormhole/pkg/providers/ollama"
	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/providers/openai_compatible"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Wormhole is the main client for interacting with LLM providers
type Wormhole struct {
	providerFactories map[string]types.ProviderFactory // Factory functions for creating providers
	providers         map[string]types.Provider        // Cached provider instances
	providersMutex    sync.RWMutex
	config            Config
	middlewareChain   *middleware.Chain
}

// Config holds the configuration for Wormhole
type Config struct {
	DefaultProvider string
	Providers       map[string]types.ProviderConfig
	Middleware      []middleware.Middleware
	DebugLogging    bool
	Logger          types.Logger
}

// New creates a new Wormhole instance
func New(config Config) *Wormhole {
	p := &Wormhole{
		providerFactories: make(map[string]types.ProviderFactory),
		providers:         make(map[string]types.Provider),
		config:            config,
	}

	// Pre-register all built-in providers
	p.registerBuiltinProviders()

	// Initialize middleware chain
	var middlewares []middleware.Middleware

	// Add debug logging if enabled
	if config.DebugLogging && config.Logger != nil {
		middlewares = append(middlewares, middleware.DebugLoggingMiddleware(config.Logger))
	}

	// Add user-provided middleware
	middlewares = append(middlewares, config.Middleware...)

	if len(middlewares) > 0 {
		p.middlewareChain = middleware.NewChain(middlewares...)
	}

	return p
}

// Use adds middleware to the Wormhole instance
func (p *Wormhole) Use(mw ...middleware.Middleware) *Wormhole {
	if p.middlewareChain == nil {
		p.middlewareChain = middleware.NewChain()
	}
	for _, m := range mw {
		p.middlewareChain.Add(m)
	}
	return p
}

// RegisterProvider allows registering a custom provider factory.
// This enables extending Wormhole with new providers without modifying core code.
func (p *Wormhole) RegisterProvider(name string, factory types.ProviderFactory) {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()
	p.providerFactories[name] = factory
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
	p.providerFactories["groq"] = func(c types.ProviderConfig) (types.Provider, error) {
		return groq.New(c.APIKey, c), nil
	}
	p.providerFactories["mistral"] = func(c types.ProviderConfig) (types.Provider, error) {
		return mistral.New(c), nil
	}
	p.providerFactories["ollama"] = func(c types.ProviderConfig) (types.Provider, error) {
		return ollama.New(c), nil
	}
}

// Text creates a new text generation request builder
func (p *Wormhole) Text() *TextRequestBuilder {
	return &TextRequestBuilder{
		wormhole: p,
		request: &types.TextRequest{
			Messages: []types.Message{},
		},
	}
}

// Structured creates a new structured output request builder
func (p *Wormhole) Structured() *StructuredRequestBuilder {
	return &StructuredRequestBuilder{
		wormhole: p,
		request: &types.StructuredRequest{
			Messages: []types.Message{},
		},
	}
}

// Embeddings creates a new embeddings request builder
func (p *Wormhole) Embeddings() *EmbeddingsRequestBuilder {
	return &EmbeddingsRequestBuilder{
		wormhole: p,
		request: &types.EmbeddingsRequest{
			Input: []string{},
		},
	}
}

// Image creates a new image generation request builder
func (p *Wormhole) Image() *ImageRequestBuilder {
	return &ImageRequestBuilder{
		wormhole: p,
		request:  &types.ImageRequest{},
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
		// Check if it's an openai_compatible provider added via With... methods
		if _, configExists := p.config.Providers[name]; configExists {
			// Assume it's an openai_compatible provider for backward compatibility
			factory = func(c types.ProviderConfig) (types.Provider, error) {
				return openai_compatible.New(name, c), nil
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

	// Use the factory to create the provider instance
	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %s: %w", name, err)
	}

	// Cache the provider
	p.providers[name] = provider
	return provider, nil
}

// WithLMStudio adds LMStudio provider support
func (p *Wormhole) WithLMStudio(config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Register factory for LMStudio
	p.providerFactories["lmstudio"] = func(c types.ProviderConfig) (types.Provider, error) {
		return openai_compatible.NewLMStudio(c), nil
	}

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["lmstudio"] = config

	return p
}

// WithVLLM adds vLLM provider support
func (p *Wormhole) WithVLLM(config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Register factory for vLLM
	p.providerFactories["vllm"] = func(c types.ProviderConfig) (types.Provider, error) {
		return openai_compatible.NewVLLM(c), nil
	}

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["vllm"] = config

	return p
}

// WithOllamaOpenAI adds Ollama OpenAI-compatible API support
func (p *Wormhole) WithOllamaOpenAI(config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Register factory for Ollama OpenAI
	p.providerFactories["ollama-openai"] = func(c types.ProviderConfig) (types.Provider, error) {
		return openai_compatible.NewOllamaOpenAI(c), nil
	}

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["ollama-openai"] = config

	return p
}

// WithOpenAICompatible adds a generic OpenAI-compatible provider
func (p *Wormhole) WithOpenAICompatible(name string, baseURL string, config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Set the baseURL in config if provided
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	// Register factory for the generic provider
	p.providerFactories[name] = func(c types.ProviderConfig) (types.Provider, error) {
		return openai_compatible.NewGeneric(name, c.BaseURL, c), nil
	}

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers[name] = config

	return p
}

// WithGemini adds Gemini provider support
func (p *Wormhole) WithGemini(apiKey string, config ...types.ProviderConfig) *Wormhole {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	cfg.APIKey = apiKey

	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["gemini"] = cfg

	return p
}

// WithGroq adds Groq provider support
func (p *Wormhole) WithGroq(apiKey string, config ...types.ProviderConfig) *Wormhole {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	cfg.APIKey = apiKey

	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["groq"] = cfg

	return p
}

// WithMistral adds Mistral provider support
func (p *Wormhole) WithMistral(config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["mistral"] = config

	return p
}

// WithOllama adds Ollama provider support
func (p *Wormhole) WithOllama(config types.ProviderConfig) *Wormhole {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	// Ensure config is stored
	if p.config.Providers == nil {
		p.config.Providers = make(map[string]types.ProviderConfig)
	}
	p.config.Providers["ollama"] = config

	return p
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

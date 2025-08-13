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
	providers       map[string]types.Provider
	providersMutex  sync.RWMutex
	config          Config
	middlewareChain *middleware.Chain
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
		providers: make(map[string]types.Provider),
		config:    config,
	}

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

	// Get provider config
	config, exists := p.config.Providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not configured", name)
	}

	// Create provider instance
	var provider types.Provider
	var err error

	switch name {
	case "openai":
		provider = openai.New(config)
	case "anthropic":
		provider = anthropic.New(config)
	case "gemini":
		provider = gemini.New(config.APIKey, config)
	case "groq":
		provider = groq.New(config.APIKey, config)
	case "mistral":
		provider = mistral.New(config)
	case "ollama":
		provider = ollama.New(config)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	if err != nil {
		return nil, err
	}

	// Cache the provider
	p.providers[name] = provider
	return provider, nil
}

// WithLMStudio adds LMStudio provider support
func (p *Wormhole) WithLMStudio(config types.ProviderConfig) *Wormhole {
	provider := openai_compatible.NewLMStudio(config)
	p.providersMutex.Lock()
	p.providers["lmstudio"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithVLLM adds vLLM provider support
func (p *Wormhole) WithVLLM(config types.ProviderConfig) *Wormhole {
	provider := openai_compatible.NewVLLM(config)
	p.providersMutex.Lock()
	p.providers["vllm"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithOllamaOpenAI adds Ollama OpenAI-compatible API support
func (p *Wormhole) WithOllamaOpenAI(config types.ProviderConfig) *Wormhole {
	provider := openai_compatible.NewOllamaOpenAI(config)
	p.providersMutex.Lock()
	p.providers["ollama-openai"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithOpenAICompatible adds a generic OpenAI-compatible provider
func (p *Wormhole) WithOpenAICompatible(name string, baseURL string, config types.ProviderConfig) *Wormhole {
	provider := openai_compatible.NewGeneric(name, baseURL, config)
	p.providersMutex.Lock()
	p.providers[name] = provider
	p.providersMutex.Unlock()
	return p
}

// WithGemini adds Gemini provider support
func (p *Wormhole) WithGemini(apiKey string, config ...types.ProviderConfig) *Wormhole {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	provider := gemini.New(apiKey, cfg)
	p.providersMutex.Lock()
	p.providers["gemini"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithGroq adds Groq provider support
func (p *Wormhole) WithGroq(apiKey string, config ...types.ProviderConfig) *Wormhole {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	provider := groq.New(apiKey, cfg)
	p.providersMutex.Lock()
	p.providers["groq"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithMistral adds Mistral provider support
func (p *Wormhole) WithMistral(config types.ProviderConfig) *Wormhole {
	provider := mistral.New(config)
	p.providersMutex.Lock()
	p.providers["mistral"] = provider
	p.providersMutex.Unlock()
	return p
}

// WithOllama adds Ollama provider support
func (p *Wormhole) WithOllama(config types.ProviderConfig) *Wormhole {
	provider := ollama.New(config)
	p.providersMutex.Lock()
	p.providers["ollama"] = provider
	p.providersMutex.Unlock()
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

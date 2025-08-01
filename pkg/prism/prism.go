package prism

import (
	"fmt"

	"github.com/prism-php/prism-go/pkg/providers/anthropic"
	"github.com/prism-php/prism-go/pkg/providers/gemini"
	"github.com/prism-php/prism-go/pkg/providers/groq"
	"github.com/prism-php/prism-go/pkg/providers/mistral"
	"github.com/prism-php/prism-go/pkg/providers/ollama"
	"github.com/prism-php/prism-go/pkg/providers/openai"
	"github.com/prism-php/prism-go/pkg/providers/openai_compatible"
	"github.com/prism-php/prism-go/pkg/types"
)

// Prism is the main client for interacting with LLM providers
type Prism struct {
	providers map[string]types.Provider
	config    Config
}

// Config holds the configuration for Prism
type Config struct {
	DefaultProvider string
	Providers       map[string]types.ProviderConfig
}

// New creates a new Prism instance
func New(config Config) *Prism {
	return &Prism{
		providers: make(map[string]types.Provider),
		config:    config,
	}
}

// Text creates a new text generation request builder
func (p *Prism) Text() *TextRequestBuilder {
	return &TextRequestBuilder{
		prism: p,
		request: &types.TextRequest{
			Messages: []types.Message{},
		},
	}
}

// Structured creates a new structured output request builder
func (p *Prism) Structured() *StructuredRequestBuilder {
	return &StructuredRequestBuilder{
		prism: p,
		request: &types.StructuredRequest{
			Messages: []types.Message{},
		},
	}
}

// Embeddings creates a new embeddings request builder
func (p *Prism) Embeddings() *EmbeddingsRequestBuilder {
	return &EmbeddingsRequestBuilder{
		prism: p,
		request: &types.EmbeddingsRequest{
			Input: []string{},
		},
	}
}

// Image creates a new image generation request builder
func (p *Prism) Image() *ImageRequestBuilder {
	return &ImageRequestBuilder{
		prism:   p,
		request: &types.ImageRequest{},
	}
}

// Audio creates a new audio request builder
func (p *Prism) Audio() *AudioRequestBuilder {
	return &AudioRequestBuilder{
		prism: p,
	}
}

// Provider returns a specific provider instance
func (p *Prism) Provider(name string) (types.Provider, error) {
	// Check if provider is already initialized
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
func (p *Prism) WithLMStudio(config types.ProviderConfig) *Prism {
	provider := openai_compatible.NewLMStudio(config)
	p.providers["lmstudio"] = provider
	return p
}

// WithVLLM adds vLLM provider support  
func (p *Prism) WithVLLM(config types.ProviderConfig) *Prism {
	provider := openai_compatible.NewVLLM(config)
	p.providers["vllm"] = provider
	return p
}

// WithOllamaOpenAI adds Ollama OpenAI-compatible API support
func (p *Prism) WithOllamaOpenAI(config types.ProviderConfig) *Prism {
	provider := openai_compatible.NewOllamaOpenAI(config)
	p.providers["ollama-openai"] = provider
	return p
}

// WithOpenAICompatible adds a generic OpenAI-compatible provider
func (p *Prism) WithOpenAICompatible(name string, baseURL string, config types.ProviderConfig) *Prism {
	provider := openai_compatible.NewGeneric(name, baseURL, config)
	p.providers[name] = provider
	return p
}

// WithGemini adds Gemini provider support
func (p *Prism) WithGemini(apiKey string, config ...types.ProviderConfig) *Prism {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	provider := gemini.New(apiKey, cfg)
	p.providers["gemini"] = provider
	return p
}

// WithGroq adds Groq provider support
func (p *Prism) WithGroq(apiKey string, config ...types.ProviderConfig) *Prism {
	var cfg types.ProviderConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	provider := groq.New(apiKey, cfg)
	p.providers["groq"] = provider
	return p
}

// WithMistral adds Mistral provider support
func (p *Prism) WithMistral(config types.ProviderConfig) *Prism {
	provider := mistral.New(config)
	p.providers["mistral"] = provider
	return p
}

// WithOllama adds Ollama provider support
func (p *Prism) WithOllama(config types.ProviderConfig) *Prism {
	provider := ollama.New(config)
	p.providers["ollama"] = provider
	return p
}

// getProvider returns the provider to use for a request
func (p *Prism) getProvider(override string) (types.Provider, error) {
	providerName := override
	if providerName == "" {
		providerName = p.config.DefaultProvider
	}
	if providerName == "" {
		return nil, fmt.Errorf("no provider specified and no default provider configured")
	}
	return p.Provider(providerName)
}

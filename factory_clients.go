package wormhole

import (
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

// SimpleFactory provides Laravel-inspired factory methods for common use cases
type SimpleFactory struct{}

// NewSimpleFactory creates a new SimpleFactory instance
func NewSimpleFactory() *SimpleFactory {
	return &SimpleFactory{}
}

// OpenAI creates a Wormhole client configured for OpenAI
func (f *SimpleFactory) OpenAI(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerOpenAI)

	return New(
		WithDefaultProvider("openai"),
		WithOpenAI(key),
	)
}

// Anthropic creates a Wormhole client configured for Anthropic
func (f *SimpleFactory) Anthropic(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerAnthropic)

	return New(
		WithDefaultProvider("anthropic"),
		WithAnthropic(key),
	)
}

// Gemini creates a Wormhole client configured for Google Gemini
func (f *SimpleFactory) Gemini(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerGemini)

	return New(
		WithDefaultProvider("gemini"),
		WithGemini(key),
	)
}

// Ollama creates a Wormhole client configured for Ollama
func (f *SimpleFactory) Ollama(baseURL ...string) (*Wormhole, error) {
	url, ok := f.getRequiredProfileBaseURL(baseURL, providerOllama)
	if !ok {
		return nil, fmt.Errorf("Ollama base URL is required: provide via parameter or %s environment variable", primaryBaseURLEnv(providerOllama))
	}

	return New(
		WithDefaultProvider("ollama"),
		WithOllama(types.ProviderConfig{
			BaseURL:       url,
			DynamicModels: true, // Users can load any model in Ollama
		}),
	), nil
}

// Groq creates a Wormhole client configured for Groq
func (f *SimpleFactory) Groq(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, "groq")

	return New(
		WithDefaultProvider("groq"),
		WithGroq(key),
	)
}

// Mistral creates a Wormhole client configured for Mistral
func (f *SimpleFactory) Mistral(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, "mistral")

	return New(
		WithDefaultProvider("mistral"),
		WithMistral(types.ProviderConfig{APIKey: key}),
	)
}

// LMStudio creates a Wormhole client configured for LMStudio
func (f *SimpleFactory) LMStudio(baseURL ...string) (*Wormhole, error) {
	url, ok := f.getRequiredProfileBaseURL(baseURL, "lmstudio")
	if !ok {
		return nil, fmt.Errorf("LMStudio base URL is required: provide via parameter or %s environment variable", primaryBaseURLEnv("lmstudio"))
	}

	return New(
		WithDefaultProvider("lmstudio"),
		WithLMStudio(types.ProviderConfig{
			BaseURL:       url,
			DynamicModels: true, // Users can load any model in LMStudio
		}),
	), nil
}

// LocalOpenAI creates a no-auth OpenAI-compatible local client. The base URL
// should include the compatible API root, usually http://host:port/v1.
func (f *SimpleFactory) LocalOpenAI(baseURL string, config ...types.ProviderConfig) (*Wormhole, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("local OpenAI-compatible base URL is required")
	}
	return New(WithLocalOpenAI(baseURL, config...)), nil
}

// OpenRouter creates a Wormhole client configured for OpenRouter (multi-provider gateway)
func (f *SimpleFactory) OpenRouter(apiKey ...string) (*Wormhole, error) {
	key := f.getProfileAPIKey(apiKey, providerOpenRouter)
	if key == "" {
		return nil, fmt.Errorf("OpenRouter API key is required: provide via parameter or %s environment variable", primaryAPIKeyEnv(providerOpenRouter))
	}

	return New(
		WithDefaultProvider("openrouter"),
		WithProfiledOpenAICompatible("openrouter", types.ProviderConfig{
			APIKey:        key,
			DynamicModels: true, // Enable all 200+ OpenRouter models without registry validation
		}),
	), nil
}

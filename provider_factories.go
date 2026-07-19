package wormhole

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/garyblankenship/wormhole/v2/providers/anthropic"
	"github.com/garyblankenship/wormhole/v2/providers/gemini"
	"github.com/garyblankenship/wormhole/v2/providers/ollama"
	"github.com/garyblankenship/wormhole/v2/providers/openai"
	"github.com/garyblankenship/wormhole/v2/types"
)

func validateAPIKey(provider, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("empty API key for provider %s", provider)
	}

	if strings.HasPrefix(apiKey, "test-") || strings.HasPrefix(apiKey, "mock-") || strings.HasPrefix(apiKey, "dummy-") {
		return nil
	}

	switch provider {
	case providerOpenAI:
		if !strings.HasPrefix(apiKey, "sk-") && !strings.HasPrefix(apiKey, "org-") {
			return fmt.Errorf("invalid OpenAI API key format, expected 'sk-' or 'org-' prefix")
		}
	case providerAnthropic:
		if !strings.HasPrefix(apiKey, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format, expected 'sk-ant-' prefix")
		}
	case providerGemini:
		if len(apiKey) < 10 {
			return fmt.Errorf("API key for Google AI Studio is too short (minimum 10 characters)")
		}
	case providerOpenRouter:
		if !strings.HasPrefix(apiKey, "sk-or-") {
			return fmt.Errorf("invalid OpenRouter API key format, expected 'sk-or-' prefix")
		}
	}
	return nil
}

func shouldValidateAPIKey(provider string, config types.ProviderConfig) bool {
	if config.EffectiveAPIKey() == "" || config.NoAuth {
		return false
	}
	if provider != providerOpenAI || config.BaseURL == "" {
		return true
	}

	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return true
	}
	return strings.EqualFold(parsed.Hostname(), "api.openai.com")
}

func openAIFactory() types.ProviderFactory {
	return func(c types.ProviderConfig) (types.Provider, error) {
		return openai.New(c), nil
	}
}

func anthropicFactory() types.ProviderFactory {
	return func(c types.ProviderConfig) (types.Provider, error) {
		return anthropic.New(c), nil
	}
}

func geminiFactory() types.ProviderFactory {
	return func(c types.ProviderConfig) (types.Provider, error) {
		return gemini.New(c.APIKey, c), nil
	}
}

func ollamaFactory() types.ProviderFactory {
	return func(c types.ProviderConfig) (types.Provider, error) {
		if c.BaseURL == "" {
			if profile, ok := providerProfile(providerOllama); ok {
				c.BaseURL = configuredBaseURL(profile)
			}
		}
		return ollama.New(c)
	}
}

func namedOpenAICompatibleFactory(name string) types.ProviderFactory {
	return func(c types.ProviderConfig) (types.Provider, error) {
		return openai.NewWithName(name, c), nil
	}
}

const (
	providerOpenAI     = "openai"
	providerAnthropic  = "anthropic"
	providerGemini     = "gemini"
	providerOpenRouter = "openrouter"
	providerOllama     = "ollama"
)

type cachedProvider struct {
	provider types.Provider
	lastUsed int64
	refCount int32
}

// ProviderHandle wraps a provider with automatic reference counting.
// Callers MUST call Close() when done with the provider to prevent memory leaks.
type ProviderHandle struct {
	types.Provider
	wormhole *Wormhole
	name     string
	released atomic.Bool
}

// Close decrements the reference count for this provider handle.
func (h *ProviderHandle) Close() error {
	if h.released.CompareAndSwap(false, true) {
		h.wormhole.releaseProvider(h.name)
	}
	return nil
}

func (p *Wormhole) registerBuiltinProviders() {
	p.providerFactories[providerOpenAI] = openAIFactory()
	p.providerFactories[providerAnthropic] = anthropicFactory()
	p.providerFactories[providerGemini] = geminiFactory()
	p.providerFactories[providerOllama] = ollamaFactory()
}

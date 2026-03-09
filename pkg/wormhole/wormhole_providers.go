package wormhole

import (
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/providers/anthropic"
	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/providers/ollama"
	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/types"
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
			if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
				c.BaseURL = envURL
			}
		}
		return ollama.New(c)
	}
}

func openAICompatibleFactory() types.ProviderFactory {
	return openAIFactory()
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

// Provider returns a specific provider instance.
func (p *Wormhole) Provider(name string) (types.Provider, error) {
	if p.shuttingDown.Load() {
		return nil, fmt.Errorf("client is shutting down")
	}
	return p.getOrCreateCachedProvider(name, false)
}

func (p *Wormhole) getProvider(override string) (types.Provider, error) {
	providerName, err := p.resolveProviderName(override)
	if err != nil {
		return nil, err
	}
	return p.Provider(providerName)
}

func (p *Wormhole) releaseProvider(name string) {
	p.providersMutex.RLock()
	cp, exists := p.providers[name]
	p.providersMutex.RUnlock()

	if exists && atomic.AddInt32(&cp.refCount, -1) < 0 {
		atomic.StoreInt32(&cp.refCount, 0)
	}
}

// ProviderWithHandle returns a provider wrapped in a handle that must be closed.
func (p *Wormhole) ProviderWithHandle(name string) (*ProviderHandle, error) {
	providerName, err := p.resolveProviderName(name)
	if err != nil {
		return nil, err
	}

	provider, err := p.getOrCreateCachedProvider(providerName, true)
	if err != nil {
		return nil, err
	}

	return &ProviderHandle{
		Provider: provider,
		wormhole: p,
		name:     providerName,
	}, nil
}

func (p *Wormhole) leaseProvider(override string) (types.Provider, func(), error) {
	handle, err := p.ProviderWithHandle(override)
	if err != nil {
		return nil, nil, err
	}
	return handle.Provider, func() { _ = handle.Close() }, nil
}

func (p *Wormhole) resolveProviderName(override string) (string, error) {
	providerName := override
	if providerName == "" {
		providerName = p.config.DefaultProvider
	}
	if providerName == "" && len(p.config.Providers) == 1 {
		for name := range p.config.Providers {
			providerName = name
		}
	}
	if providerName == "" {
		return "", fmt.Errorf("no provider specified and no default provider configured")
	}
	return providerName, nil
}

func (p *Wormhole) providerFactoryFor(name string) (types.ProviderFactory, error) {
	if factory, exists := p.providerFactories[name]; exists {
		return factory, nil
	}

	if _, configExists := p.config.Providers[name]; configExists {
		return openAIFactory(), nil
	}

	return nil, types.ErrProviderNotFound.WithProvider(name).WithDetails(p.formatProviderHint(name))
}

func (p *Wormhole) configuredProviderConfig(name string) (types.ProviderConfig, error) {
	config, exists := p.config.Providers[name]
	if !exists {
		return types.ProviderConfig{}, types.ErrProviderNotFound.WithProvider(name).WithDetails(p.formatProviderHint(name))
	}
	return cloneProviderConfig(config), nil
}

func cloneProviderConfig(config types.ProviderConfig) types.ProviderConfig {
	cloned := config
	if len(config.Headers) > 0 {
		cloned.Headers = make(map[string]string, len(config.Headers))
		maps.Copy(cloned.Headers, config.Headers)
	}
	if len(config.Params) > 0 {
		cloned.Params = make(map[string]any, len(config.Params))
		maps.Copy(cloned.Params, config.Params)
	}
	if config.MaxRetries != nil {
		maxRetries := *config.MaxRetries
		cloned.MaxRetries = &maxRetries
	}
	if config.RetryDelay != nil {
		retryDelay := *config.RetryDelay
		cloned.RetryDelay = &retryDelay
	}
	if config.RetryMaxDelay != nil {
		retryMaxDelay := *config.RetryMaxDelay
		cloned.RetryMaxDelay = &retryMaxDelay
	}
	return cloned
}

func (p *Wormhole) applyDefaultTimeout(config types.ProviderConfig) types.ProviderConfig {
	if config.Timeout == 0 && p.config.DefaultTimeout != 0 {
		config.Timeout = int(p.config.DefaultTimeout.Seconds())
	}
	return config
}

func (p *Wormhole) createProviderWithConfig(name string, config types.ProviderConfig) (types.Provider, error) {
	factory, err := p.providerFactoryFor(name)
	if err != nil {
		return nil, err
	}

	if config.APIKey != "" {
		if err := validateAPIKey(name, config.APIKey); err != nil {
			return nil, fmt.Errorf("invalid API key for provider %s: %w", name, err)
		}
	}

	provider, err := factory(p.applyDefaultTimeout(config))
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %s: %w", name, err)
	}

	return provider, nil
}

func (p *Wormhole) getOrCreateCachedProvider(name string, acquireRef bool) (types.Provider, error) {
	p.providersMutex.RLock()
	if cp, exists := p.providers[name]; exists {
		if acquireRef {
			atomic.AddInt32(&cp.refCount, 1)
		}
		atomic.StoreInt64(&cp.lastUsed, time.Now().UnixNano())
		p.cacheHits.Add(1)
		p.providersMutex.RUnlock()
		return cp.provider, nil
	}
	p.providersMutex.RUnlock()

	config, err := p.configuredProviderConfig(name)
	if err != nil {
		return nil, err
	}

	provider, err := p.createProviderWithConfig(name, config)
	if err != nil {
		return nil, err
	}

	refCount := int32(0)
	if acquireRef {
		refCount = 1
	}

	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()
	if cp, exists := p.providers[name]; exists {
		if acquireRef {
			atomic.AddInt32(&cp.refCount, 1)
		}
		atomic.StoreInt64(&cp.lastUsed, time.Now().UnixNano())
		p.cacheHits.Add(1)
		if err := provider.Close(); err != nil && p.config.Logger != nil {
			p.config.Logger.Warn("error closing duplicate provider", "provider", name, "error", err)
		}
		return cp.provider, nil
	}

	p.providers[name] = &cachedProvider{
		provider: provider,
		lastUsed: time.Now().UnixNano(),
		refCount: refCount,
	}
	p.cacheMisses.Add(1)
	return provider, nil
}

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

func (p *Wormhole) getConfiguredProviders() []string {
	providers := make([]string, 0, len(p.config.Providers))
	for name := range p.config.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

func formatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items, ", ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func validateConfig(c *Config) []string {
	var warnings []string

	if c.DefaultProvider != "" {
		if _, exists := c.Providers[c.DefaultProvider]; !exists {
			warnings = append(warnings, fmt.Sprintf(
				"DefaultProvider '%s' is set but not configured. Use wormhole.With%s() to configure it.",
				c.DefaultProvider, capitalize(c.DefaultProvider),
			))
		}
	}

	localProviders := map[string]bool{"ollama": true, "lmstudio": true, "vllm": true}
	for name, cfg := range c.Providers {
		if !localProviders[name] && cfg.APIKey == "" {
			warnings = append(warnings, fmt.Sprintf(
				"Provider '%s' is configured but has no API key. Requests will likely fail.",
				name,
			))
		}
	}

	if c.DefaultProvider == "" && len(c.Providers) > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"No DefaultProvider set but %d providers configured. Use WithDefaultProvider() or specify .Using() on each request.",
			len(c.Providers),
		))
	}

	return warnings
}

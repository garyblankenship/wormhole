package wormhole

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

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

	for name, cfg := range c.Providers {
		profile, knownProfile := providerProfile(name)
		if cfg.NoAuth {
			continue
		}
		if (!knownProfile || !profile.Local) && cfg.EffectiveAPIKey() == "" {
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

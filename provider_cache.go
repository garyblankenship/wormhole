package wormhole

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// providerInit is owned by the cache, so canceling one waiter never cancels
// construction needed by another caller. done publishes the immutable result.
type providerInit struct {
	done chan struct{}
	err  error
}

func (p *Wormhole) getOrCreateCachedProviderContext(ctx context.Context, name string, acquireRef, pin bool) (types.Provider, error) {
	created := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.providersMutex.RLock()
		if cp, exists := p.providers[name]; exists && !pin {
			if acquireRef {
				atomic.AddInt32(&cp.refCount, 1)
			}
			atomic.StoreInt64(&cp.lastUsed, time.Now().UnixNano())
			if !created {
				p.cacheHits.Add(1)
			}
			p.providersMutex.RUnlock()
			return cp.provider, nil
		}
		p.providersMutex.RUnlock()

		p.providersMutex.Lock()
		if cp, exists := p.providers[name]; exists {
			if acquireRef {
				atomic.AddInt32(&cp.refCount, 1)
			}
			if pin {
				cp.pinned = true
			}
			atomic.StoreInt64(&cp.lastUsed, time.Now().UnixNano())
			if !created {
				p.cacheHits.Add(1)
			}
			p.providersMutex.Unlock()
			return cp.provider, nil
		}
		init := p.providerInits[name]
		if init == nil {
			if !p.beginProviderAcquisition() {
				p.providersMutex.Unlock()
				return nil, fmt.Errorf("client is shutting down")
			}
			created = true
			init = &providerInit{done: make(chan struct{})}
			if p.providerInits == nil {
				p.providerInits = make(map[string]*providerInit)
			}
			p.providerInits[name] = init
			go p.initializeCachedProvider(name, init)
		}
		p.providersMutex.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-init.done:
			if init.err != nil {
				return nil, init.err
			}
		}
	}
}

func (p *Wormhole) initializeCachedProvider(name string, init *providerInit) {
	defer p.providerAcquisitionWg.Done()
	config, err := p.configuredProviderConfig(name)
	var provider types.Provider
	if err == nil {
		provider, err = p.createProviderWithConfig(name, config)
	}
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()
	if err == nil {
		p.providers[name] = &cachedProvider{provider: provider, lastUsed: time.Now().UnixNano()}
		p.cacheMisses.Add(1)
	}
	init.err = err
	delete(p.providerInits, name)
	close(init.done)
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

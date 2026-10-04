package wormhole

import (
	"fmt"
	"sync/atomic"

	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Wormhole) releaseProvider(name string) {
	p.providersMutex.RLock()
	defer p.providersMutex.RUnlock()
	cp, exists := p.providers[name]

	if exists {
		for count := atomic.LoadInt32(&cp.refCount); count > 0; count = atomic.LoadInt32(&cp.refCount) {
			if atomic.CompareAndSwapInt32(&cp.refCount, count, count-1) {
				break
			}
		}
	}
}

// ProviderWithHandle returns a cache-eviction lease that must be closed. The
// lease does not extend provider lifetime across Wormhole.Shutdown.
func (p *Wormhole) ProviderWithHandle(name string) (*ProviderHandle, error) {
	if !p.beginProviderAcquisition() {
		return nil, fmt.Errorf("client is shutting down")
	}
	defer p.providerAcquisitionWg.Done()

	providerName, err := p.resolveProviderName(name)
	if err != nil {
		return nil, err
	}

	provider, err := p.getOrCreateCachedProvider(providerName, true, false)
	if err != nil {
		return nil, err
	}
	if p.finishProviderAcquisition() {
		p.releaseProvider(providerName)
		return nil, fmt.Errorf("client is shutting down")
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

package wormhole

import (
	"context"
	"fmt"
	"os"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/discovery/fetchers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func (p *Wormhole) initializeDiscoveryService() {
	var modelFetchers []discovery.ModelFetcher

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
		case providerOllama:
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

	modelFetchers = append(modelFetchers, fetchers.NewOpenRouterFetcher())
	p.discoveryService = discovery.NewDiscoveryService(p.config.DiscoveryConfig, modelFetchers...)

	if !p.config.DiscoveryConfig.OfflineMode && p.config.DiscoveryConfig.RefreshInterval > 0 {
		p.discoveryService.StartBackgroundRefresh(context.Background())
	}
}

// ListAvailableModels returns all available models for a provider from the discovery cache.
func (p *Wormhole) ListAvailableModels(provider string) ([]*types.ModelInfo, error) {
	if p.discoveryService == nil {
		return nil, fmt.Errorf("model discovery is not enabled")
	}
	return p.discoveryService.GetModels(context.Background(), provider)
}

// RefreshModels manually triggers a refresh of all provider model catalogs.
func (p *Wormhole) RefreshModels() error {
	if p.discoveryService == nil {
		return fmt.Errorf("model discovery is not enabled")
	}
	return p.discoveryService.RefreshModels(context.Background())
}

// ClearModelCache clears all cached model data.
func (p *Wormhole) ClearModelCache() {
	if p.discoveryService != nil {
		p.discoveryService.ClearCache()
	}
}

// StopModelDiscovery stops the background model refresh goroutine.
func (p *Wormhole) StopModelDiscovery() {
	if p.discoveryService != nil {
		if err := p.discoveryService.Stop(); err != nil && p.config.Logger != nil {
			p.config.Logger.Warn("error stopping discovery service", "error", err)
		}
	}
}

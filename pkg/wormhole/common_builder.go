package wormhole

import "github.com/garyblankenship/wormhole/pkg/types"

// CommonBuilder contains shared fields and methods for all request builders
type CommonBuilder struct {
	wormhole *Wormhole
	provider string
	baseURL  string
}

// newCommonBuilder creates a new CommonBuilder with the given wormhole instance
func newCommonBuilder(wormhole *Wormhole) CommonBuilder {
	return CommonBuilder{
		wormhole: wormhole,
		provider: wormhole.config.DefaultProvider,
	}
}

// getWormhole returns the wormhole instance
func (cb *CommonBuilder) getWormhole() *Wormhole {
	return cb.wormhole
}

// getProvider returns the current provider
func (cb *CommonBuilder) getProvider() string {
	return cb.provider
}

// setProvider sets the provider to use
func (cb *CommonBuilder) setProvider(provider string) {
	cb.provider = provider
}

// getBaseURL returns the current base URL
func (cb *CommonBuilder) getBaseURL() string {
	return cb.baseURL
}

// setBaseURL sets a custom base URL for OpenAI-compatible APIs
func (cb *CommonBuilder) setBaseURL(url string) {
	cb.baseURL = url
}

// getProviderWithBaseURL gets the provider, creating a temporary one with custom baseURL if specified
func (cb *CommonBuilder) getProviderWithBaseURL() (types.Provider, error) {
	// If no custom baseURL, use normal provider
	if cb.getBaseURL() == "" {
		return cb.getWormhole().getProvider(cb.getProvider())
	}

	// Create a temporary OpenAI-compatible provider with custom baseURL
	providerName := cb.getProvider()
	if providerName == "" {
		providerName = cb.getWormhole().config.DefaultProvider
	}

	// Get existing provider config for API key
	var apiKey string
	if providerConfig, exists := cb.getWormhole().config.Providers[providerName]; exists {
		apiKey = providerConfig.APIKey
	}

	// Create temporary provider with custom baseURL
	tempConfig := types.ProviderConfig{
		APIKey:  apiKey,
		BaseURL: cb.getBaseURL(),
	}

	return cb.getWormhole().createOpenAICompatibleProvider(tempConfig)
}

package wormhole

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
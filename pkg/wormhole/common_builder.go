package wormhole

import (
	"maps"

	"github.com/garyblankenship/wormhole/pkg/types"
)

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

// getProviderWithBaseURL gets a provider lease for the duration of a request.
// When BaseURL is overridden, a temporary provider is created with the full
// configured provider settings preserved and only BaseURL changed.
func (cb *CommonBuilder) getProviderWithBaseURL() (types.Provider, func(), error) {
	if cb.getBaseURL() == "" {
		return cb.getWormhole().leaseProvider(cb.getProvider())
	}

	providerName, err := cb.getWormhole().resolveProviderName(cb.getProvider())
	if err != nil {
		return nil, nil, err
	}

	config, err := cb.getWormhole().configuredProviderConfig(providerName)
	if err != nil {
		return nil, nil, err
	}
	config.BaseURL = cb.getBaseURL()

	provider, err := cb.getWormhole().createProviderWithConfig(providerName, config)
	if err != nil {
		return nil, nil, err
	}

	return provider, func() { _ = provider.Close() }, nil
}

func (cb *CommonBuilder) idempotencyScope(operation string) string {
	providerName := cb.getProvider()
	if providerName == "" {
		providerName = cb.getWormhole().config.DefaultProvider
	}
	return operation + ":" + providerName + ":" + cb.getBaseURL()
}

func cloneBaseRequestFields(dst, src *types.BaseRequest) {
	if src.Temperature != nil {
		temp := *src.Temperature
		dst.Temperature = &temp
	}
	if src.TopP != nil {
		topP := *src.TopP
		dst.TopP = &topP
	}
	if src.MaxTokens != nil {
		maxTokens := *src.MaxTokens
		dst.MaxTokens = &maxTokens
	}
	if src.PresencePenalty != nil {
		presencePenalty := *src.PresencePenalty
		dst.PresencePenalty = &presencePenalty
	}
	if src.FrequencyPenalty != nil {
		frequencyPenalty := *src.FrequencyPenalty
		dst.FrequencyPenalty = &frequencyPenalty
	}
	if src.Seed != nil {
		seed := *src.Seed
		dst.Seed = &seed
	}
	if len(src.Stop) > 0 {
		dst.Stop = make([]string, len(src.Stop))
		copy(dst.Stop, src.Stop)
	}
	dst.ProviderOptions = cloneProviderOptions(src.ProviderOptions)
}

// cloneProviderOptions returns a shallow copy of the provider options map.
// Returns nil if the source is empty.
func cloneProviderOptions(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	maps.Copy(dst, src)
	return dst
}

func prepareExecutionMessages(systemPrompt string, messages []types.Message) []types.Message {
	if systemPrompt == "" {
		return messages
	}
	result := make([]types.Message, 0, 1+len(messages))
	result = append(result, types.NewSystemMessage(systemPrompt))
	result = append(result, messages...)
	return result
}

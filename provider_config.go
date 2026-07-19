package wormhole

import (
	"fmt"
	"maps"
	"time"

	whconfig "github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

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
		for key, value := range config.Params {
			cloned.Params[key] = types.CloneValue(value)
		}
	}
	cloned.APIKeys = append([]string(nil), config.APIKeys...)
	cloned.DefaultProviderOptions = types.CloneMap(config.DefaultProviderOptions)
	if config.ProviderOptionsByModel != nil {
		cloned.ProviderOptionsByModel = make(map[string]map[string]any, len(config.ProviderOptionsByModel))
		for model, options := range config.ProviderOptionsByModel {
			cloned.ProviderOptionsByModel[model] = types.CloneMap(options)
		}
	}
	if len(config.RequestPolicy.MaxTokensParamRules) > 0 {
		cloned.RequestPolicy.MaxTokensParamRules = make([]types.MaxTokensParamRule, len(config.RequestPolicy.MaxTokensParamRules))
		copy(cloned.RequestPolicy.MaxTokensParamRules, config.RequestPolicy.MaxTokensParamRules)
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
	if config.HTTPTimeout != nil {
		httpTimeout := *config.HTTPTimeout
		cloned.HTTPTimeout = &httpTimeout
	}
	return cloned
}

func (p *Wormhole) applyDefaultTimeout(config types.ProviderConfig) types.ProviderConfig {
	if config.HTTPTimeout != nil || config.Timeout != 0 {
		return config
	}
	timeout := whconfig.GetDefaultHTTPTimeout()
	if p.config.DefaultTimeoutSet {
		timeout = p.config.DefaultTimeout
	}
	config.HTTPTimeout = &timeout
	if timeout > 0 && timeout%time.Second == 0 {
		config.Timeout = int(timeout.Seconds())
	}
	return config
}

func (p *Wormhole) applyDefaultRetries(config types.ProviderConfig) types.ProviderConfig {
	if config.MaxRetries == nil && p.config.DefaultRetriesSet {
		maxRetries := p.config.DefaultRetries
		config.MaxRetries = &maxRetries
	}
	if config.RetryDelay == nil && p.config.DefaultRetryDelaySet {
		retryDelay := p.config.DefaultRetryDelay
		config.RetryDelay = &retryDelay
	}
	return config
}

func (p *Wormhole) createProviderWithConfig(name string, config types.ProviderConfig) (types.Provider, error) {
	factory, err := p.providerFactoryFor(name)
	if err != nil {
		return nil, err
	}

	if config.APIKey == "" {
		config.APIKey = config.EffectiveAPIKey()
	}
	if shouldValidateAPIKey(name, config) {
		if err := validateAPIKey(name, config.EffectiveAPIKey()); err != nil {
			return nil, fmt.Errorf("invalid API key for provider %s: %w", name, err)
		}
	}

	config = p.applyDefaultTimeout(config)
	config = p.applyDefaultRetries(config)
	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %s: %w", name, err)
	}

	return provider, nil
}

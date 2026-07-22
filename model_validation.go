package wormhole

import (
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

var textModelCapabilities = []types.ModelCapability{
	types.CapabilityText,
	types.CapabilityChat,
}

// validateModelAttempt applies the opt-in registry policy immediately before
// an operation attempt. Empty registries and dynamic-provider catalogs remain
// permissive so provider-native model IDs keep working by default.
func (p *Wormhole) validateModelAttempt(providerName, modelID string, anyOf, required []types.ModelCapability) error {
	if !p.config.ModelValidation || p.modelRegistry == nil || p.modelRegistry.Count() == 0 {
		return nil
	}

	resolvedProvider, err := p.resolveProviderName(providerName)
	if err != nil {
		return err
	}
	providerConfig, err := p.configuredProviderConfig(resolvedProvider)
	if err != nil {
		return err
	}
	if providerConfig.DynamicModels {
		return nil
	}

	model, ok := p.modelRegistry.Get(modelID)
	if !ok {
		return types.ErrModelNotFound.WithModel(modelID)
	}
	if model.Provider != "" && model.Provider != resolvedProvider {
		return types.ErrModelNotFound.
			WithModel(modelID).
			WithProvider(resolvedProvider).
			WithDetails(fmt.Sprintf("model is registered for provider %q", model.Provider))
	}

	if err := p.modelRegistry.ValidateModel(modelID, required); err != nil {
		return err
	}
	if len(anyOf) == 0 {
		return nil
	}

	for _, available := range model.Capabilities {
		for _, acceptable := range anyOf {
			if available == acceptable {
				return nil
			}
		}
	}
	return types.ErrModelNotSupported.
		WithModel(modelID).
		WithDetails(fmt.Sprintf("missing one of capabilities: %v", anyOf))
}

func textRequiredCapabilities(request *types.TextRequest, toolsEnabled, streaming bool) []types.ModelCapability {
	required := make([]types.ModelCapability, 0, 3)
	if streaming {
		required = append(required, types.CapabilityStream)
	}
	if toolsEnabled || len(request.Tools) > 0 {
		required = append(required, types.CapabilityFunctions)
	}
	if textRequestHasMedia(request) {
		required = append(required, types.CapabilityVision)
	}
	return required
}

func textRequestHasMedia(request *types.TextRequest) bool {
	if request == nil {
		return false
	}
	for _, message := range request.Messages {
		if user, ok := message.(*types.UserMessage); ok && len(user.Media) > 0 {
			return true
		}
	}
	return false
}

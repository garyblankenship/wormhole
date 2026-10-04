package types

import "fmt"

// GetForProvider selects provider-specific metadata before unscoped metadata.
// Global Get retains its existing last-registration-wins behavior.
func (r *ModelRegistry) GetForProvider(provider, modelID string) (*ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, scope := range []string{provider, ""} {
		for _, model := range r.byProvider[scope] {
			if model.ID == modelID {
				return CloneModelInfo(model), true
			}
		}
	}
	return nil, false
}

// ValidateModelForProvider validates the selected provider's model entry.
func (r *ModelRegistry) ValidateModelForProvider(provider, modelID string, required []ModelCapability) error {
	model, exists := r.GetForProvider(provider, modelID)
	if !exists {
		return ErrModelNotFound.WithModel(modelID).WithProvider(provider)
	}
	return validateModelInfo(model, required)
}

func validateModelInfo(model *ModelInfo, requiredCapabilities []ModelCapability) error {
	modelID := model.ID

	if model.Deprecated {
		return NewWormholeError(ErrorCodeModel, "model is deprecated", false).
			WithModel(modelID).
			WithDetails("consider using a newer model")
	}

	// Check capabilities
	for _, required := range requiredCapabilities {
		found := false
		for _, cap := range model.Capabilities {
			if cap == required {
				found = true
				break
			}
		}
		if !found {
			return ErrModelNotSupported.
				WithModel(modelID).
				WithDetails(fmt.Sprintf("missing capability: %s", required))
		}
	}

	return nil
}

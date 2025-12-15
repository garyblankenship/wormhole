package types

import (
	"fmt"
	"strings"
)

// ModelInfo contains metadata about a model
type ModelInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Provider      string            `json:"provider"`
	Description   string            `json:"description,omitempty"`
	ContextLength int               `json:"context_length,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Cost          *ModelCost        `json:"cost,omitempty"`
	Capabilities  []ModelCapability `json:"capabilities"`
	Constraints   map[string]any    `json:"constraints,omitempty"`
	Deprecated    bool              `json:"deprecated,omitempty"`
}

// ModelCost represents the cost of using a model
type ModelCost struct {
	InputTokens  float64 `json:"input_tokens"`  // Cost per 1K input tokens
	OutputTokens float64 `json:"output_tokens"` // Cost per 1K output tokens
	Currency     string  `json:"currency"`      // USD, EUR, etc.
}

// ModelCapability represents what a model can do
type ModelCapability string

const (
	CapabilityText       ModelCapability = "text"
	CapabilityChat       ModelCapability = "chat"
	CapabilityStructured ModelCapability = "structured"
	CapabilityEmbeddings ModelCapability = "embeddings"
	CapabilityImages     ModelCapability = "images"
	CapabilityAudio      ModelCapability = "audio"
	CapabilityVision     ModelCapability = "vision"
	CapabilityFunctions  ModelCapability = "functions"
	CapabilityStream     ModelCapability = "stream"
)

// ModelRegistry manages available models across providers
type ModelRegistry struct {
	models     map[string]*ModelInfo
	byProvider map[string][]*ModelInfo
}

// NewModelRegistry creates a new empty model registry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:     make(map[string]*ModelInfo),
		byProvider: make(map[string][]*ModelInfo),
	}
}

// Register adds a model to the registry
func (r *ModelRegistry) Register(model *ModelInfo) {
	r.models[model.ID] = model
	r.byProvider[model.Provider] = append(r.byProvider[model.Provider], model)
}

// Get retrieves a model by ID
func (r *ModelRegistry) Get(modelID string) (*ModelInfo, bool) {
	model, exists := r.models[modelID]
	return model, exists
}

// GetByProvider returns all models for a provider
func (r *ModelRegistry) GetByProvider(provider string) []*ModelInfo {
	return r.byProvider[provider]
}

// GetByCapability returns all models with a specific capability
func (r *ModelRegistry) GetByCapability(capability ModelCapability) []*ModelInfo {
	var results []*ModelInfo
	for _, model := range r.models {
		for _, cap := range model.Capabilities {
			if cap == capability {
				results = append(results, model)
				break
			}
		}
	}
	return results
}

// List returns all registered models
func (r *ModelRegistry) List() []*ModelInfo {
	models := make([]*ModelInfo, 0, len(r.models))
	for _, model := range r.models {
		models = append(models, model)
	}
	return models
}

// Search finds models matching a query
func (r *ModelRegistry) Search(query string) []*ModelInfo {
	query = strings.ToLower(query)
	var results []*ModelInfo

	for _, model := range r.models {
		if strings.Contains(strings.ToLower(model.ID), query) ||
			strings.Contains(strings.ToLower(model.Name), query) ||
			strings.Contains(strings.ToLower(model.Description), query) {
			results = append(results, model)
		}
	}

	return results
}

// ValidateModel checks if a model is available and supports required capabilities
func (r *ModelRegistry) ValidateModel(modelID string, requiredCapabilities []ModelCapability) error {
	model, exists := r.Get(modelID)
	if !exists {
		return ErrModelNotFound.WithModel(modelID)
	}

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

// EstimateCost calculates the estimated cost for a request
func (r *ModelRegistry) EstimateCost(modelID string, inputTokens, outputTokens int) (float64, error) {
	model, exists := r.Get(modelID)
	if !exists {
		return 0, ErrModelNotFound.WithModel(modelID)
	}

	if model.Cost == nil {
		return 0, nil // No cost information available
	}

	inputCost := (float64(inputTokens) / 1000.0) * model.Cost.InputTokens
	outputCost := (float64(outputTokens) / 1000.0) * model.Cost.OutputTokens

	return inputCost + outputCost, nil
}

// GetConstraints returns model-specific constraints
func (r *ModelRegistry) GetConstraints(modelID string) (map[string]any, error) {
	model, exists := r.Get(modelID)
	if !exists {
		return nil, ErrModelNotFound.WithModel(modelID)
	}

	return model.Constraints, nil
}

// LoadModelsFromConfig loads models from external configuration
func (r *ModelRegistry) LoadModelsFromConfig(models []*ModelInfo) {
	for _, model := range models {
		r.Register(model)
	}
}

// Global model registry instance - Use LoadModelsFromConfig() to populate
var DefaultModelRegistry = NewModelRegistry()

// Helper functions for model operations

// ListAvailableModels returns models available for a provider
func ListAvailableModels(provider string) []*ModelInfo {
	return DefaultModelRegistry.GetByProvider(provider)
}

// ValidateModelForCapability checks if a model supports a capability
func ValidateModelForCapability(modelID string, capability ModelCapability) error {
	return DefaultModelRegistry.ValidateModel(modelID, []ModelCapability{capability})
}

// GetModelConstraints returns constraints for a model
func GetModelConstraints(modelID string) (map[string]any, error) {
	return DefaultModelRegistry.GetConstraints(modelID)
}

// EstimateModelCost calculates cost for input/output tokens
func EstimateModelCost(modelID string, inputTokens, outputTokens int) (float64, error) {
	return DefaultModelRegistry.EstimateCost(modelID, inputTokens, outputTokens)
}

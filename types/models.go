package types

import (
	"fmt"
	"strings"
	"sync"
)

// ModelInfo contains metadata about a model
type ModelInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Provider      string            `json:"provider"`
	Created       int64             `json:"created,omitempty"`
	OwnedBy       string            `json:"owned_by,omitempty"`
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
	CapabilityRerank     ModelCapability = "rerank"
)

// ModelRegistry manages available models across providers.
// It is safe for concurrent use: the global DefaultModelRegistry is mutated at
// wormhole.New(WithModels(...)) time and read by validation helpers.
type ModelRegistry struct {
	mu         sync.RWMutex
	models     map[string]*ModelInfo
	byProvider map[string][]*ModelInfo
}

// Count returns the number of registered models.
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.models)
}

// NewModelRegistry creates a new empty model registry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:     make(map[string]*ModelInfo),
		byProvider: make(map[string][]*ModelInfo),
	}
}

// Register adds a model to the registry (or updates an existing one by ID).
func (r *ModelRegistry) Register(model *ModelInfo) {
	if model == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	model = CloneModelInfo(model)
	r.models[model.ID] = model

	// Replace an existing entry for this ID in the provider's slice, else append —
	// so repeated registration of the same model list does not accumulate duplicates.
	list := r.byProvider[model.Provider]
	for i, m := range list {
		if m.ID == model.ID {
			list[i] = model
			return
		}
	}
	r.byProvider[model.Provider] = append(list, model)
}

// Get retrieves a model by ID
func (r *ModelRegistry) Get(modelID string) (*ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	model, exists := r.models[modelID]
	return CloneModelInfo(model), exists
}

// GetByProvider returns all models for a provider (a copy, safe to retain).
func (r *ModelRegistry) GetByProvider(provider string) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.byProvider[provider]
	out := make([]*ModelInfo, len(list))
	for i := range list {
		out[i] = CloneModelInfo(list[i])
	}
	return out
}

// GetByCapability returns all models with a specific capability
func (r *ModelRegistry) GetByCapability(capability ModelCapability) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var results []*ModelInfo
	for _, model := range r.models {
		for _, cap := range model.Capabilities {
			if cap == capability {
				results = append(results, CloneModelInfo(model))
				break
			}
		}
	}
	return results
}

// List returns all registered models
func (r *ModelRegistry) List() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]*ModelInfo, 0, len(r.models))
	for _, model := range r.models {
		models = append(models, CloneModelInfo(model))
	}
	return models
}

// Search finds models matching a query
func (r *ModelRegistry) Search(query string) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	query = strings.ToLower(query)
	var results []*ModelInfo

	for _, model := range r.models {
		if strings.Contains(strings.ToLower(model.ID), query) ||
			strings.Contains(strings.ToLower(model.Name), query) ||
			strings.Contains(strings.ToLower(model.Description), query) {
			results = append(results, CloneModelInfo(model))
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

// DefaultModelRegistry is the global model registry instance. It starts EMPTY
// (opt-in): populate it with LoadModelsFromConfig, or pass models via
// wormhole.WithModels(...) to wormhole.New. Until populated, model-validation
// helpers have no models to validate against.
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

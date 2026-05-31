package wormhole

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ModelSort controls the deterministic order used by SelectModels.
type ModelSort string

const (
	ModelSortDefault ModelSort = ""
	ModelSortCost    ModelSort = "cost"
	ModelSortContext ModelSort = "context"
	ModelSortName    ModelSort = "name"
)

// ModelQuery describes the model constraints an application needs.
type ModelQuery struct {
	Providers         []string
	Capabilities      []types.ModelCapability
	NameContains      string
	MinContextLength  int
	MinMaxTokens      int
	MaxInputCost      float64
	MaxOutputCost     float64
	IncludeDeprecated bool
	PreferProviders   []string
	SortBy            ModelSort
	Limit             int
}

// SelectModel returns the first model matching query according to SelectModels ordering.
func (p *Wormhole) SelectModel(ctx context.Context, query ModelQuery) (*types.ModelInfo, error) {
	query.Limit = 1
	models, err := p.SelectModels(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, types.ErrModelNotFound.WithDetails("no discovered model matched query")
	}
	return models[0], nil
}

// SelectModels returns discovered models matching query.
func (p *Wormhole) SelectModels(ctx context.Context, query ModelQuery) ([]*types.ModelInfo, error) {
	if p.discoveryService == nil {
		return nil, fmt.Errorf("model discovery is not enabled")
	}

	providers := query.Providers
	if len(providers) == 0 {
		providers = p.discoveryService.Providers()
	}
	if len(providers) == 0 {
		return nil, types.ErrModelNotFound.WithDetails("no model discovery providers configured")
	}

	var matches []*types.ModelInfo
	var errs []string
	for _, provider := range providers {
		models, err := p.discoveryService.GetModels(ctx, provider)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", provider, err))
			continue
		}
		for _, model := range models {
			if matchesModelQuery(model, query) {
				matches = append(matches, cloneModelInfo(model))
			}
		}
	}

	sortModels(matches, query)
	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	if len(matches) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("model selection failed: %s", strings.Join(errs, "; "))
	}
	return matches, nil
}

func matchesModelQuery(model *types.ModelInfo, query ModelQuery) bool {
	if model == nil {
		return false
	}
	if model.Deprecated && !query.IncludeDeprecated {
		return false
	}
	if query.NameContains != "" {
		needle := strings.ToLower(query.NameContains)
		if !strings.Contains(strings.ToLower(model.ID), needle) &&
			!strings.Contains(strings.ToLower(model.Name), needle) &&
			!strings.Contains(strings.ToLower(model.Description), needle) {
			return false
		}
	}
	if query.MinContextLength > 0 && model.ContextLength < query.MinContextLength {
		return false
	}
	if query.MinMaxTokens > 0 && model.MaxTokens < query.MinMaxTokens {
		return false
	}
	if query.MaxInputCost > 0 && (model.Cost == nil || model.Cost.InputTokens > query.MaxInputCost) {
		return false
	}
	if query.MaxOutputCost > 0 && (model.Cost == nil || model.Cost.OutputTokens > query.MaxOutputCost) {
		return false
	}
	return hasCapabilities(model, query.Capabilities)
}

func hasCapabilities(model *types.ModelInfo, required []types.ModelCapability) bool {
	if len(required) == 0 {
		return true
	}
	available := make(map[types.ModelCapability]struct{}, len(model.Capabilities))
	for _, capability := range model.Capabilities {
		available[capability] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := available[capability]; !ok {
			return false
		}
	}
	return true
}

func sortModels(models []*types.ModelInfo, query ModelQuery) {
	providerRank := make(map[string]int, len(query.PreferProviders))
	for i, provider := range query.PreferProviders {
		providerRank[provider] = i
	}

	sort.SliceStable(models, func(i, j int) bool {
		left, right := models[i], models[j]
		if rankLess(left.Provider, right.Provider, providerRank) {
			return true
		}
		if rankLess(right.Provider, left.Provider, providerRank) {
			return false
		}
		switch query.SortBy {
		case ModelSortCost:
			if modelCostScore(left) != modelCostScore(right) {
				return modelCostScore(left) < modelCostScore(right)
			}
		case ModelSortContext:
			if left.ContextLength != right.ContextLength {
				return left.ContextLength > right.ContextLength
			}
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		return left.ID < right.ID
	})
}

func rankLess(left, right string, ranks map[string]int) bool {
	leftRank, leftOK := ranks[left]
	rightRank, rightOK := ranks[right]
	switch {
	case leftOK && rightOK:
		return leftRank < rightRank
	case leftOK:
		return true
	default:
		return false
	}
}

func modelCostScore(model *types.ModelInfo) float64 {
	if model == nil || model.Cost == nil {
		return 1 << 60
	}
	return model.Cost.InputTokens + model.Cost.OutputTokens
}

func cloneModelInfo(model *types.ModelInfo) *types.ModelInfo {
	cloned := *model
	if len(model.Capabilities) > 0 {
		cloned.Capabilities = append([]types.ModelCapability(nil), model.Capabilities...)
	}
	if len(model.Constraints) > 0 {
		cloned.Constraints = make(map[string]any, len(model.Constraints))
		for key, value := range model.Constraints {
			cloned.Constraints[key] = value
		}
	}
	if model.Cost != nil {
		cost := *model.Cost
		cloned.Cost = &cost
	}
	return &cloned
}

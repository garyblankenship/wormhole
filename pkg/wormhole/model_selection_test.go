package wormhole

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/types"
)

type staticModelFetcher struct {
	name   string
	models []*types.ModelInfo
}

func (f staticModelFetcher) Name() string { return f.name }

func (f staticModelFetcher) FetchModels(context.Context) ([]*types.ModelInfo, error) {
	return f.models, nil
}

func TestSelectModelsFiltersAndSorts(t *testing.T) {
	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{}, staticModelFetcher{
		name: "testai",
		models: []*types.ModelInfo{
			{
				ID:            "expensive",
				Name:          "Expensive",
				Provider:      "testai",
				ContextLength: 128000,
				Capabilities:  []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
				Cost:          &types.ModelCost{InputTokens: 5, OutputTokens: 10},
			},
			{
				ID:            "cheap",
				Name:          "Cheap",
				Provider:      "testai",
				ContextLength: 64000,
				Capabilities:  []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
				Cost:          &types.ModelCost{InputTokens: 1, OutputTokens: 2},
			},
			{
				ID:           "embedding",
				Name:         "Embedding",
				Provider:     "testai",
				Capabilities: []types.ModelCapability{types.CapabilityEmbeddings},
			},
		},
	})

	models, err := client.SelectModels(context.Background(), ModelQuery{
		Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
		SortBy:       ModelSortCost,
		Limit:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "cheap" {
		t.Fatalf("selected models = %#v", models)
	}
}

func TestSelectModelHonorsPreferredProvider(t *testing.T) {
	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{},
		staticModelFetcher{name: "test-openai", models: []*types.ModelInfo{{
			ID:           "openai-model",
			Provider:     "test-openai",
			Capabilities: []types.ModelCapability{types.CapabilityText},
		}}},
		staticModelFetcher{name: "test-anthropic", models: []*types.ModelInfo{{
			ID:           "anthropic-model",
			Provider:     "test-anthropic",
			Capabilities: []types.ModelCapability{types.CapabilityText},
		}}},
	)

	model, err := client.SelectModel(context.Background(), ModelQuery{
		Capabilities:    []types.ModelCapability{types.CapabilityText},
		PreferProviders: []string{"test-anthropic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.ID != "anthropic-model" {
		t.Fatalf("selected model = %s", model.ID)
	}
}

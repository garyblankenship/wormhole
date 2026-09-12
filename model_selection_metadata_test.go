package wormhole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/discovery/fetchers"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestModelSelectionMetadataFromFetchedFixtures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/anthropic/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"id":           "claude-fixture",
					"display_name": "Claude Fixture",
				}},
			})
		case "/openrouter/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"id":             "openai/context-fixture",
					"name":           "Context Fixture",
					"context_length": 400000,
					"architecture":   map[string]any{"modality": "text->text"},
					"moderation":     map[string]any{"illicit": false},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	anthropicModels, err := fetchers.NewAnthropicFetcherWithConfig(types.ProviderConfig{
		APIKey:  "anthropic-key",
		BaseURL: server.URL + "/anthropic",
	}).FetchModels(ctx)
	if err != nil {
		t.Fatalf("fetch Anthropic fixture: %v", err)
	}
	openRouterModels, err := fetchers.NewOpenRouterFetcherWithConfig(types.ProviderConfig{
		BaseURL: server.URL + "/openrouter",
	}).FetchModels(ctx)
	if err != nil {
		t.Fatalf("fetch OpenRouter fixture: %v", err)
	}

	if len(anthropicModels) != 1 || len(openRouterModels) != 1 {
		t.Fatalf("fetched models = Anthropic %#v, OpenRouter %#v", anthropicModels, openRouterModels)
	}
	if anthropicModels[0].ContextLength != 0 || anthropicModels[0].MaxTokens != 0 {
		t.Fatalf("Anthropic unknown limits = context %d, output %d", anthropicModels[0].ContextLength, anthropicModels[0].MaxTokens)
	}
	if openRouterModels[0].ContextLength != 400000 || openRouterModels[0].MaxTokens != 0 {
		t.Fatalf("OpenRouter limits = context %d, output %d", openRouterModels[0].ContextLength, openRouterModels[0].MaxTokens)
	}

	client := New(WithDiscovery(false))
	client.discoveryService = discovery.NewDiscoveryService(discovery.DiscoveryConfig{
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	},
		staticModelFetcher{name: "anthropic", models: anthropicModels},
		staticModelFetcher{name: "openrouter", models: openRouterModels},
	)
	t.Cleanup(func() { _ = client.discoveryService.Stop() })
	// Replace built-in fallback catalogs with the fetched fixture metadata.
	if err := client.discoveryService.RefreshModels(ctx); err != nil {
		t.Fatalf("refresh fixture catalogs: %v", err)
	}

	streaming, err := client.SelectModels(ctx, ModelQuery{
		Providers:    []string{"anthropic"},
		Capabilities: []types.ModelCapability{types.CapabilityStream},
	})
	if err != nil {
		t.Fatalf("select streaming Anthropic model: %v", err)
	}
	if len(streaming) != 1 || streaming[0].ID != "claude-fixture" {
		t.Fatalf("streaming selection = %#v", streaming)
	}

	knownContext, err := client.SelectModels(ctx, ModelQuery{
		Providers:        []string{"openrouter"},
		MinContextLength: 128000,
	})
	if err != nil {
		t.Fatalf("select known OpenRouter context: %v", err)
	}
	if len(knownContext) != 1 || knownContext[0].ID != "openai/context-fixture" {
		t.Fatalf("context selection = %#v", knownContext)
	}

	unknownContext, err := client.SelectModels(ctx, ModelQuery{
		Providers:        []string{"anthropic"},
		MinContextLength: 1,
	})
	if err != nil {
		t.Fatalf("select unknown Anthropic context: %v", err)
	}
	if len(unknownContext) != 0 {
		t.Fatalf("unknown Anthropic context matched = %#v", unknownContext)
	}

	unknownOutput, err := client.SelectModels(ctx, ModelQuery{
		Providers:    []string{"openrouter", "anthropic"},
		MinMaxTokens: 1,
	})
	if err != nil {
		t.Fatalf("select known output limit: %v", err)
	}
	if len(unknownOutput) != 0 {
		t.Fatalf("unknown output limit matched = %#v", unknownOutput)
	}
}

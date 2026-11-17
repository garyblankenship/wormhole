package wormhole

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAgentScopedToolSchemaMatchesDispatch(t *testing.T) {
	t.Parallel()
	provider := &mockToolProvider{responses: []*types.TextResponse{
		{ToolCalls: []types.ToolCall{{ID: "lookup", Name: "shared", Arguments: map[string]any{"query": "value"}}}},
		{Text: "done"},
	}}
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	var globalCalls atomic.Int32
	client.RegisterTool("shared", "global", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
		globalCalls.Add(1)
		return "global", nil
	})
	builder := client.Agent().Model("mock-model").AddTool("shared", "scoped", map[string]any{
		"type":       "object",
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"required":   []string{"query"},
	}, func(_ context.Context, args map[string]any) (any, error) { return args["query"], nil })
	result, err := builder.Run(context.Background(), "lookup")
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range provider.requests {
		if len(request.Tools) != 1 || request.Tools[0].Description != "scoped" {
			t.Fatalf("advertised tools = %#v", request.Tools)
		}
		properties, ok := request.Tools[0].InputSchema["properties"].(map[string]any)
		if !ok || properties["query"] == nil {
			t.Fatalf("advertised schema = %#v", request.Tools[0].InputSchema)
		}
	}
	if globalCalls.Load() != 0 || len(result.Steps[0].ToolResults) != 1 || result.Steps[0].ToolResults[0].Result != "value" {
		t.Fatalf("global calls = %d, results = %#v", globalCalls.Load(), result.Steps[0].ToolResults)
	}
	if got := client.toolRegistry.Get("shared").Tool.Description; got != "global" {
		t.Fatalf("global description after run = %q", got)
	}
}

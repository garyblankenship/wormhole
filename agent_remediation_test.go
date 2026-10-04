package wormhole

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type remediationAgentMiddleware struct {
	types.ProviderMiddleware
	wraps atomic.Int32
	calls atomic.Int32
}

func (m *remediationAgentMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	m.wraps.Add(1)
	return func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
		m.calls.Add(1)
		return next(ctx, req)
	}
}

func TestRemediationC211AgentMiddlewareOnce(t *testing.T) {
	t.Parallel()
	mw := &remediationAgentMiddleware{}
	provider := &mockToolProvider{responses: []*types.TextResponse{
		{ToolCalls: []types.ToolCall{{ID: "one", Name: "tool"}}},
		{ToolCalls: []types.ToolCall{{ID: "two", Name: "tool"}}},
		{Text: "done"},
	}}
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("mock", types.ProviderConfig{}), WithDiscovery(false), WithProviderMiddleware(mw))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	result, err := client.Agent().Model("test").AddTool("tool", "tool", nil, func(context.Context, map[string]any) (any, error) { return true, nil }).Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if mw.wraps.Load() != 1 || mw.calls.Load() != 3 || result.TotalSteps != 3 {
		t.Fatalf("wraps=%d calls=%d result=%#v", mw.wraps.Load(), mw.calls.Load(), result)
	}
}

func TestRemediationC212AgentInvalidStepsBeforeResources(t *testing.T) {
	t.Parallel()
	for _, max := range []int{0, -1} {
		t.Run(map[int]string{0: "zero", -1: "negative"}[max], func(t *testing.T) {
			t.Parallel()
			var providerCreations, toolCalls atomic.Int32
			client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
				providerCreations.Add(1)
				return &mockToolProvider{}, nil
			}), WithProviderConfig("mock", types.ProviderConfig{}), WithDiscovery(false))
			t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
			agent := client.Agent().Model("test").MaxSteps(max).AddTool("tool", "tool", nil, func(context.Context, map[string]any) (any, error) { toolCalls.Add(1); return nil, nil })
			result, err := agent.Run(context.Background(), "go")
			if err == nil || !strings.Contains(err.Error(), "max steps must be positive") || result != nil || providerCreations.Load() != 0 || toolCalls.Load() != 0 {
				t.Fatalf("result=%#v err=%v providers=%d tools=%d", result, err, providerCreations.Load(), toolCalls.Load())
			}
		})
	}
}

func TestRemediationC2I03AgentStepLimitRetainsHistory(t *testing.T) {
	t.Parallel()
	provider := &mockToolProvider{responses: []*types.TextResponse{{Text: "working", ToolCalls: []types.ToolCall{{ID: "one", Name: "tool"}}}}}
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("mock", types.ProviderConfig{}), WithDiscovery(false))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	result, err := client.Agent().Model("test").MaxSteps(1).AddTool("tool", "tool", nil, func(context.Context, map[string]any) (any, error) { return "finished tool", nil }).Run(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "max steps (1)") {
		t.Fatalf("err=%v", err)
	}
	if result == nil || result.TotalSteps != 1 || len(result.Steps) != 1 || result.Response.Text != "working" || result.Steps[0].Done || result.Steps[0].ToolResults[0].Result != "finished tool" {
		t.Fatalf("result=%#v", result)
	}
}

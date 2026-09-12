package wormholetest

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type operationConformanceProvider struct {
	*types.BaseProvider
	capabilities []types.ModelCapability
	text         func(context.Context, types.TextRequest) (*types.TextResponse, error)
	rerank       func(context.Context, types.RerankRequest) (*types.RerankResponse, error)
}

func newOperationConformanceProvider(capabilities ...types.ModelCapability) *operationConformanceProvider {
	return &operationConformanceProvider{
		BaseProvider: types.NewBaseProvider("operations"),
		capabilities: capabilities,
	}
}

func (p *operationConformanceProvider) SupportedCapabilities() []types.ModelCapability {
	return append([]types.ModelCapability(nil), p.capabilities...)
}

func (p *operationConformanceProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if p.text == nil {
		return p.BaseProvider.Text(ctx, request)
	}
	return p.text(ctx, request)
}

func (p *operationConformanceProvider) Rerank(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
	if p.rerank == nil {
		return p.BaseProvider.Rerank(ctx, request)
	}
	return p.rerank(ctx, request)
}

func TestRunProviderConformanceOperations(t *testing.T) {
	provider := newOperationConformanceProvider(types.CapabilityFunctions, types.CapabilityRerank)
	provider.text = func(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
		if request.Model != "tool-model" {
			t.Errorf("tool model = %q, want tool-model", request.Model)
		}
		if len(request.Tools) != 1 || request.Tools[0].Name != conformanceToolName {
			t.Errorf("tools = %#v", request.Tools)
		}
		if request.ToolChoice == nil || request.ToolChoice.Type != types.ToolChoiceTypeSpecific || request.ToolChoice.ToolName != conformanceToolName {
			t.Errorf("tool choice = %#v", request.ToolChoice)
		}
		return &types.TextResponse{ToolCalls: []types.ToolCall{{
			ID:        "call-1",
			Name:      conformanceToolName,
			Arguments: map[string]any{"message": "hello"},
		}}}, nil
	}
	provider.rerank = func(_ context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
		if request.Model != "rerank-model" {
			t.Errorf("rerank model = %q, want rerank-model", request.Model)
		}
		if len(request.Documents) != 3 {
			t.Errorf("documents = %#v", request.Documents)
		}
		return &types.RerankResponse{Results: []types.RerankResult{{Index: 1, RelevanceScore: 0.9}}}, nil
	}

	RunProviderConformance(t, ProviderConformanceConfig{
		Provider:         provider,
		ToolModel:        "tool-model",
		RerankModel:      "rerank-model",
		CheckToolCalling: true,
		CheckRerank:      true,
	})
}

func TestCheckToolCallingMalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		call types.ToolCall
		want string
	}{
		{name: "missing call", want: "no tool call"},
		{name: "wrong function", call: types.ToolCall{ID: "call-1", Name: "other", Arguments: map[string]any{"message": "hello"}}, want: "want"},
		{name: "missing call ID", call: types.ToolCall{Name: conformanceToolName, Arguments: map[string]any{"message": "hello"}}, want: "without a call ID"},
		{name: "malformed JSON arguments", call: types.ToolCall{ID: "call-1", Function: &types.ToolCallFunction{Name: conformanceToolName, Arguments: "{"}}, want: "malformed"},
		{name: "missing message", call: types.ToolCall{ID: "call-1", Name: conformanceToolName, Arguments: map[string]any{}}, want: "message argument"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newOperationConformanceProvider()
			provider.text = func(context.Context, types.TextRequest) (*types.TextResponse, error) {
				if test.call.ID == "" && test.call.Name == "" && test.call.Function == nil && test.call.Arguments == nil {
					return &types.TextResponse{}, nil
				}
				return &types.TextResponse{ToolCalls: []types.ToolCall{test.call}}, nil
			}
			err := checkToolCalling(provider, "tool-model", time.Second)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("checkToolCalling error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCheckRerankMalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		response *types.RerankResponse
		want     string
	}{
		{name: "nil response", want: "no results"},
		{name: "empty results", response: &types.RerankResponse{}, want: "no results"},
		{name: "out of range", response: &types.RerankResponse{Results: []types.RerankResult{{Index: 3, RelevanceScore: 1}}}, want: "out-of-range"},
		{name: "duplicate index", response: &types.RerankResponse{Results: []types.RerankResult{{Index: 1, RelevanceScore: 1}, {Index: 1, RelevanceScore: 0.5}}}, want: "duplicate"},
		{name: "non finite score", response: &types.RerankResponse{Results: []types.RerankResult{{Index: 1, RelevanceScore: math.Inf(1)}}}, want: "non-finite"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newOperationConformanceProvider()
			provider.rerank = func(context.Context, types.RerankRequest) (*types.RerankResponse, error) {
				return test.response, nil
			}
			err := checkRerank(provider, "rerank-model", time.Second)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("checkRerank error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRequireConformanceCapability(t *testing.T) {
	if err := requireConformanceCapability(map[types.ModelCapability]bool{}, types.CapabilityFunctions, "tool calling"); err == nil || !strings.Contains(err.Error(), "does not advertise") {
		t.Fatalf("missing capability error = %v", err)
	}
	if err := requireConformanceCapability(map[types.ModelCapability]bool{types.CapabilityFunctions: true}, types.CapabilityFunctions, "tool calling"); err != nil {
		t.Fatalf("advertised capability error = %v", err)
	}
}

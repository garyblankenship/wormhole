package wormholetest

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

const conformanceToolName = "wormhole_conformance_echo"

func requireConformanceCapability(caps map[types.ModelCapability]bool, capability types.ModelCapability, operation string) error {
	if !caps[capability] {
		return fmt.Errorf("provider does not advertise %s capability required for opted-in %s conformance check", capability, operation)
	}
	return nil
}

func checkToolCalling(provider types.Provider, model string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	response, err := provider.Text(ctx, types.TextRequest{
		BaseRequest: types.BaseRequest{Model: model},
		Messages:    []types.Message{types.NewUserMessage("call the requested function")},
		Tools: []types.Tool{*types.NewTool(conformanceToolName, "Echo a message", map[string]any{
			"type":       "object",
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
			"required":   []string{"message"},
		})},
		ToolChoice: &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: conformanceToolName},
	})
	if err != nil {
		return fmt.Errorf("Text returned error for advertised tool calling capability: %w", err)
	}
	if response == nil || len(response.ToolCalls) == 0 {
		return fmt.Errorf("Text returned no tool call for forced function %q", conformanceToolName)
	}

	call, err := types.NormalizeToolCall(response.ToolCalls[0])
	if err != nil {
		return fmt.Errorf("Text returned malformed tool call: %w", err)
	}
	if call.Name != conformanceToolName {
		return fmt.Errorf("Text returned tool call %q, want %q", call.Name, conformanceToolName)
	}
	if call.ID == "" {
		return fmt.Errorf("Text returned tool call %q without a call ID", conformanceToolName)
	}
	if _, ok := call.Arguments["message"].(string); !ok {
		return fmt.Errorf("Text returned tool call %q without string message argument", conformanceToolName)
	}
	return nil
}

func checkRerank(provider types.Provider, model string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	documents := []string{"alpha", "beta", "gamma"}
	response, err := provider.Rerank(ctx, types.RerankRequest{
		Model:     model,
		Query:     "Which document mentions beta?",
		Documents: documents,
	})
	if err != nil {
		return fmt.Errorf("Rerank returned error for advertised capability: %w", err)
	}
	if response == nil || len(response.Results) == 0 {
		return fmt.Errorf("Rerank returned no results")
	}

	seen := make(map[int]struct{}, len(response.Results))
	for _, result := range response.Results {
		if result.Index < 0 || result.Index >= len(documents) {
			return fmt.Errorf("Rerank returned out-of-range index %d for %d documents", result.Index, len(documents))
		}
		if _, ok := seen[result.Index]; ok {
			return fmt.Errorf("Rerank returned duplicate index %d", result.Index)
		}
		if math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) {
			return fmt.Errorf("Rerank returned non-finite relevance score for index %d", result.Index)
		}
		seen[result.Index] = struct{}{}
	}
	return nil
}

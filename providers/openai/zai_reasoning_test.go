package openai

import (
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestZAIReasoningLowAndDisabledWire(t *testing.T) {
	t.Parallel()
	provider := NewWithName("zai", types.NewProviderConfig("fixture"))
	low := &types.TextRequest{BaseRequest: types.BaseRequest{
		Model: "caller-selected-glm", Reasoning: &types.Reasoning{Effort: types.ReasoningEffortLow},
	}}
	payload := provider.buildChatPayload(low, nil)
	assertZAIReasoningWire(t, payload, "enabled", "low")
	if payload["model"] != "caller-selected-glm" {
		t.Fatalf("model = %#v", payload["model"])
	}

	no := false
	for _, reasoning := range []*types.Reasoning{
		{Effort: types.ReasoningEffortNone},
		{Effort: types.ReasoningEffortLow, Enabled: &no},
	} {
		low.Reasoning = reasoning
		assertZAIReasoningWire(t, provider.buildChatPayload(low, nil), "disabled", "")
	}
}

func TestZAIReasoningOptionPrecedence(t *testing.T) {
	t.Parallel()
	cfg := types.NewProviderConfig("fixture").
		WithDefaultProviderOptions(map[string]any{"thinking": map[string]any{"type": "disabled"}}).
		WithProviderOptionsForModel("glm", map[string]any{"thinking": map[string]any{"type": "enabled"}, "reasoning_effort": "medium"})
	provider := NewWithName("zai", cfg)
	request := &types.TextRequest{BaseRequest: types.BaseRequest{
		Model: "glm", Reasoning: &types.Reasoning{Effort: types.ReasoningEffortLow},
	}}
	assertZAIReasoningWire(t, provider.buildChatPayload(request, nil), "enabled", "medium")
	request.ProviderOptions = map[string]any{"thinking": map[string]any{"type": "disabled"}, "reasoning_effort": "high"}
	assertZAIReasoningWire(t, provider.buildChatPayload(request, nil), "disabled", "high")
	delete(request.ProviderOptions, "reasoning_effort")
	// A higher-priority explicit disable must not leave a generated low effort.
	request.Model = "other"
	assertZAIReasoningWire(t, provider.buildChatPayload(request, nil), "disabled", "")
	request.ProviderOptions = map[string]any{"reasoning_effort": "high"}
	assertZAIReasoningWire(t, provider.buildChatPayload(request, nil), "enabled", "high")
	if cfg.DefaultProviderOptions["thinking"].(map[string]any)["type"] != "disabled" {
		t.Fatal("mutated provider defaults")
	}
}

func TestZAIReasoningDoesNotChangeOtherOpenAICompatibleWire(t *testing.T) {
	t.Parallel()
	request := &types.TextRequest{BaseRequest: types.BaseRequest{
		Model: "model", Reasoning: &types.Reasoning{Effort: types.ReasoningEffortLow},
	}}
	for _, name := range []string{"openrouter", "openai"} {
		provider := NewWithName(name, types.NewProviderConfig("fixture"))
		payload := provider.buildChatPayload(request, nil)
		if _, present := payload["thinking"]; present {
			t.Fatalf("%s gained thinking: %#v", name, payload)
		}
		if name == "openai" {
			if payload["reasoning_effort"] != "low" || payload["reasoning"] != nil {
				t.Fatalf("native wire = %#v", payload)
			}
		} else if nested, ok := payload["reasoning"].(map[string]any); !ok || nested["effort"] != "low" {
			t.Fatalf("compatible wire = %#v", payload)
		}
	}
}

func assertZAIReasoningWire(t *testing.T, payload map[string]any, thinkingType, effort string) {
	t.Helper()
	thinking, ok := payload["thinking"].(map[string]any)
	if !ok || thinking["type"] != thinkingType {
		t.Fatalf("thinking = %#v, want %s", payload["thinking"], thinkingType)
	}
	if effort == "" {
		if _, ok := payload["reasoning_effort"]; ok {
			t.Fatalf("unexpected reasoning_effort: %#v", payload)
		}
	} else if payload["reasoning_effort"] != effort {
		t.Fatalf("reasoning_effort = %#v, want %s", payload["reasoning_effort"], effort)
	}
	if _, ok := payload["reasoning"]; ok {
		t.Fatalf("unexpected generic reasoning: %#v", payload["reasoning"])
	}
}

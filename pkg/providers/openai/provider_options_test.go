package openai

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderOptionsMergedIntoChatPayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"service_tier": "default", "store": false}).
		WithProviderOptionsForModel("gpt-test", map[string]any{"service_tier": "model"}))

	payload := provider.buildChatPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "gpt-test",
			ProviderOptions: map[string]any{"service_tier": "request"},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})

	if payload["service_tier"] != "request" {
		t.Fatalf("service_tier = %v, want request", payload["service_tier"])
	}
	if payload["store"] != false {
		t.Fatalf("store = %v, want false", payload["store"])
	}
}

func TestProviderOptionsMergedIntoResponsesPayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"parallel_tool_calls": false}).
		WithProviderOptionsForModel("gpt-test", map[string]any{"reasoning": map[string]any{"effort": "low"}}))

	payload := provider.buildResponsesPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "gpt-test",
			ProviderOptions: map[string]any{"parallel_tool_calls": true},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})

	if payload["parallel_tool_calls"] != true {
		t.Fatalf("parallel_tool_calls = %v, want true", payload["parallel_tool_calls"])
	}
	if payload["reasoning"] == nil {
		t.Fatal("reasoning option missing")
	}
}

func TestTypedReasoningMergedIntoPayloads(t *testing.T) {
	t.Parallel()
	enabled := true
	provider := New(types.NewProviderConfig("key"))

	chatPayload := provider.buildChatPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-test",
			Reasoning: &types.Reasoning{
				Effort:    types.ReasoningEffortLow,
				MaxTokens: 256,
				Enabled:   &enabled,
			},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})

	reasoning, ok := chatPayload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("chat reasoning = %#v, want map", chatPayload["reasoning"])
	}
	if reasoning["effort"] != "low" || reasoning["max_tokens"] != 256 || reasoning["enabled"] != true {
		t.Fatalf("chat reasoning = %#v", reasoning)
	}

	responsesPayload := provider.buildResponsesPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gpt-test",
			Reasoning: &types.Reasoning{Effort: types.ReasoningEffortHigh},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	reasoning, ok = responsesPayload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("responses reasoning = %#v", responsesPayload["reasoning"])
	}
}

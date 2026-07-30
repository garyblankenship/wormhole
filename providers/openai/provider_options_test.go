package openai

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderOptionsMergedIntoChatPayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"service_tier": "default", "store": false}).
		WithProviderOptionsForModel("gpt-test", map[string]any{"service_tier": "model"}))

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "gpt-test",
			ProviderOptions: map[string]any{"service_tier": "request"},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	}
	payload := provider.buildChatPayload(request, request.Messages)

	if payload["service_tier"] != "request" {
		t.Fatalf("service_tier = %v, want request", payload["service_tier"])
	}
	if payload["store"] != false {
		t.Fatalf("store = %v, want false", payload["store"])
	}
}

func TestTypedSamplingControlsReachOpenAIPayloads(t *testing.T) {
	t.Parallel()
	frequency := float32(0.4)
	presence := float32(-0.3)
	seed := 42
	parallel := false
	provider := New(types.NewProviderConfig("key"))
	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:             "gpt-test",
			FrequencyPenalty:  &frequency,
			PresencePenalty:   &presence,
			Seed:              &seed,
			ParallelToolCalls: &parallel,
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	}

	chat := provider.buildChatPayload(request, request.Messages)
	if chat["frequency_penalty"] != frequency || chat["presence_penalty"] != presence || chat["seed"] != seed || chat["parallel_tool_calls"] != parallel {
		t.Fatalf("chat sampling controls = %#v", chat)
	}

	responses := provider.buildResponsesPayload(request, request.Messages)
	if responses["parallel_tool_calls"] != parallel {
		t.Fatalf("responses parallel_tool_calls = %v, want false", responses["parallel_tool_calls"])
	}
	if err := provider.validateResponsesSampling(*request); err == nil {
		t.Fatal("Responses API accepted unsupported penalty/seed controls")
	}
}

func TestProviderOptionsMergedIntoResponsesPayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"parallel_tool_calls": false}).
		WithProviderOptionsForModel("gpt-test", map[string]any{"reasoning": map[string]any{"effort": "low"}}))

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "gpt-test",
			ProviderOptions: map[string]any{"parallel_tool_calls": true},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	}
	payload := provider.buildResponsesPayload(request, request.Messages)

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

	chatRequest := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-test",
			Reasoning: &types.Reasoning{
				Effort:    types.ReasoningEffortLow,
				MaxTokens: 256,
				Enabled:   &enabled,
			},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	}
	chatPayload := provider.buildChatPayload(chatRequest, chatRequest.Messages)

	reasoning, ok := chatPayload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("chat reasoning = %#v, want map", chatPayload["reasoning"])
	}
	if reasoning["effort"] != "low" || reasoning["max_tokens"] != 256 || reasoning["enabled"] != true {
		t.Fatalf("chat reasoning = %#v", reasoning)
	}

	responsesRequest := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gpt-test",
			Reasoning: &types.Reasoning{Effort: types.ReasoningEffortHigh},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	}
	responsesPayload := provider.buildResponsesPayload(responsesRequest, responsesRequest.Messages)
	reasoning, ok = responsesPayload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("responses reasoning = %#v", responsesPayload["reasoning"])
	}
}

package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderOptionsMergedIntoMessagePayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"metadata": map[string]any{"source": "default"}, "thinking": false}).
		WithProviderOptionsForModel("claude-test", map[string]any{"thinking": true}))

	payload, err := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "claude-test",
			ProviderOptions: map[string]any{"metadata": map[string]any{"source": "request"}},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}

	if payload["thinking"] != true {
		t.Fatalf("thinking = %v, want true", payload["thinking"])
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["source"] != "request" {
		t.Fatalf("metadata source = %v, want request", metadata["source"])
	}
}

func TestTypedReasoningMergedIntoMessagePayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key"))

	payload, err := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "claude-test",
			Reasoning: &types.Reasoning{MaxTokens: 1024},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}

	thinking, ok := payload["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want map", payload["thinking"])
	}
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != 1024 {
		t.Fatalf("thinking = %#v", thinking)
	}
}

func TestParallelToolCallsMapsToAnthropicToolChoice(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key"))
	parallel := false
	payload, err := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test", ParallelToolCalls: &parallel},
		Messages:    []types.Message{types.NewUserMessage("hi")},
		Tools:       []types.Tool{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}
	choice, ok := payload["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "auto" || choice["disable_parallel_tool_use"] != true {
		t.Fatalf("tool_choice = %#v", payload["tool_choice"])
	}

	frequency := float32(0.1)
	if err := provider.validateSamplingControls(types.TextRequest{BaseRequest: types.BaseRequest{FrequencyPenalty: &frequency}}); err == nil {
		t.Fatal("Anthropic accepted unsupported frequency_penalty")
	}

	none := &types.ToolChoice{Type: types.ToolChoiceTypeNone}
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test", ParallelToolCalls: &parallel},
		Messages:    []types.Message{types.NewUserMessage("hi")},
		Tools:       []types.Tool{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice:  none,
	}
	if err := provider.validateSamplingControls(request); err == nil {
		t.Fatal("Anthropic accepted parallel_tool_calls with tool_choice none")
	}
	payload, err = provider.buildMessagePayload(&request)
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}
	choice = payload["tool_choice"].(map[string]any)
	if choice["type"] != "none" {
		t.Fatalf("tool_choice type = %v, want none", choice["type"])
	}
	if _, ok := choice["disable_parallel_tool_use"]; ok {
		t.Fatalf("tool_choice contains invalid disable_parallel_tool_use: %#v", choice)
	}
}

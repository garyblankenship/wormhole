package providers

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestRequestBuilderPayloads(t *testing.T) {
	t.Parallel()

	builder := NewRequestBuilder()

	payload := builder.BuildTextPayload("model-1", []any{"message"}, "system")
	if payload["model"] != "model-1" || payload["system"] != "system" {
		t.Fatalf("text payload = %#v", payload)
	}

	temp := float32(0.7)
	topP := float32(0.9)
	maxTokens := 128
	builder.AddGenerationParams(payload, &temp, &topP, &maxTokens, []string{"stop"})
	if payload["temperature"] != temp || payload["top_p"] != topP || payload["max_tokens"] != maxTokens {
		t.Fatalf("generation params missing from payload: %#v", payload)
	}
	if got := payload["stop"].([]string); len(got) != 1 || got[0] != "stop" {
		t.Fatalf("stop = %#v, want [stop]", payload["stop"])
	}

	single := builder.BuildEmbeddingsPayload("embed", []string{"one"})
	if single["input"] != "one" {
		t.Fatalf("single embedding input = %#v, want one", single["input"])
	}

	multiple := builder.BuildEmbeddingsPayload("embed", []string{"one", "two"})
	if got := multiple["input"].([]string); len(got) != 2 || got[1] != "two" {
		t.Fatalf("multiple embedding input = %#v, want [one two]", multiple["input"])
	}
}

func TestRequestBuilderTransformsMessagesAndTools(t *testing.T) {
	t.Parallel()

	builder := NewRequestBuilder()
	toolCall := types.ToolCall{
		ID:   "call-1",
		Type: "lookup",
		Arguments: map[string]any{
			"query": "weather",
		},
	}

	tests := []struct {
		name string
		msg  any
		role string
	}{
		{name: "user", msg: types.NewUserMessage("hello"), role: "user"},
		{name: "assistant", msg: &types.AssistantMessage{Content: "hi", ToolCalls: []types.ToolCall{toolCall}}, role: "assistant"},
		{name: "system", msg: types.NewSystemMessage("rules"), role: "system"},
		{name: "tool", msg: types.NewToolResultMessage("call-1", "result"), role: "tool"},
		{name: "fallback", msg: 123, role: "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := builder.TransformMessage(tt.msg)
			if got["role"] != tt.role {
				t.Fatalf("role = %v, want %s", got["role"], tt.role)
			}
		})
	}

	transformed := builder.TransformMessages([]any{types.NewUserMessage("one"), types.NewSystemMessage("two")})
	if len(transformed) != 2 || transformed[0]["role"] != "user" || transformed[1]["role"] != "system" {
		t.Fatalf("TransformMessages = %#v", transformed)
	}

	interfaceMessages := builder.TransformMessagesFromInterface([]types.Message{
		types.NewUserMessage("one"),
		types.NewSystemMessage("two"),
	})
	if len(interfaceMessages) != 2 || interfaceMessages[0]["role"] != "user" || interfaceMessages[1]["role"] != "system" {
		t.Fatalf("TransformMessagesFromInterface = %#v", interfaceMessages)
	}

	tool := types.NewTool("lookup", "Lookup data", map[string]any{"type": "object"})
	toolMap := builder.TransformTool(*tool)
	function := toolMap["function"].(map[string]any)
	if function["name"] != "lookup" || function["parameters"].(map[string]any)["type"] != "object" {
		t.Fatalf("TransformTool = %#v", toolMap)
	}

	tools := builder.TransformTools([]types.Tool{*tool})
	if len(tools) != 1 {
		t.Fatalf("TransformTools len = %d, want 1", len(tools))
	}
}

func TestRequestBuilderTransformToolChoice(t *testing.T) {
	t.Parallel()

	builder := NewRequestBuilder()

	if got := builder.TransformToolChoice(nil); got != nil {
		t.Fatalf("nil tool choice = %#v, want nil", got)
	}
	if got := builder.TransformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeNone}); got != "none" {
		t.Fatalf("none tool choice = %#v, want none", got)
	}
	if got := builder.TransformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAuto}); got != "auto" {
		t.Fatalf("auto tool choice = %#v, want auto", got)
	}
	specific := builder.TransformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"}).(map[string]any)
	function := specific["function"].(map[string]any)
	if specific["type"] != "function" || function["name"] != "lookup" {
		t.Fatalf("specific tool choice = %#v", specific)
	}
	if got := builder.TransformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeSpecific}); got != "auto" {
		t.Fatalf("specific without name = %#v, want auto", got)
	}
}

func TestRequestBuilderValidateModelName(t *testing.T) {
	t.Parallel()

	builder := NewRequestBuilder()
	if _, err := builder.ValidateModelName(""); err == nil {
		t.Fatal("ValidateModelName empty returned nil error")
	}
	got, err := builder.ValidateModelName("gpt-5", "claude-", "gpt-")
	if err != nil {
		t.Fatalf("ValidateModelName returned error: %v", err)
	}
	if got != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", got)
	}
	got, err = builder.ValidateModelName("custom")
	if err != nil {
		t.Fatalf("ValidateModelName without prefixes returned error: %v", err)
	}
	if got != "custom" {
		t.Fatalf("model = %q, want custom", got)
	}
}

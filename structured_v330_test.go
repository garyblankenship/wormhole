package wormhole

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330StructuredControlledToolRoundTrip(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		tools, _ := payload["tools"].([]any)
		if len(tools) != 2 {
			t.Errorf("expected app and result tools, got %#v", tools)
		}
		var message map[string]any
		finish := "tool_calls"
		if requests.Add(1) == 1 {
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "app-1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}}
		} else {
			messages, _ := payload["messages"].([]any)
			last, _ := messages[len(messages)-1].(map[string]any)
			if last["role"] != "tool" {
				t.Errorf("tool result not replayed: %#v", last)
			}
			raw, _ := json.Marshal(map[string]any{"value": "done"})
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "final", "type": "function", "function": map[string]any{"name": structured.ResultToolName, "arguments": string(raw)}}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "response", "model": "fixture", "choices": []any{map[string]any{"message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}})
	}))
	t.Cleanup(upstream.Close)
	cfg := types.NewProviderConfig("fixture")
	cfg.BaseURL = upstream.URL
	client := New(WithOpenAI("fixture", cfg))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	builder := client.Structured().Using("openai").Model("fixture").Prompt("look up value").Schema(map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}}).Mode(types.StructuredModeTools).Tools(*types.NewTool("lookup", "lookup", map[string]any{"type": "object"}))
	var output struct {
		Value string `json:"value"`
	}
	err := builder.GenerateAs(context.Background(), &output)
	var required *types.ToolCallsRequiredError
	if !errors.As(err, &required) || len(required.Response.ToolCalls) != 1 || required.Response.Data != nil {
		t.Fatalf("expected caller-owned tool turn: %v", err)
	}
	if required.Response.Usage.TotalTokens != 5 {
		t.Fatal("tool turn lost usage")
	}
	builder.AddMessage(&types.AssistantMessage{ToolCalls: required.Response.ToolCalls}).AddMessage(types.NewToolResultMessage("app-1", "found"))
	if err := builder.GenerateAs(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Value != "done" || requests.Load() != 2 {
		t.Fatalf("roundtrip output=%#v requests=%d", output, requests.Load())
	}
}

func TestV330StructuredToolsSnapshot(t *testing.T) {
	t.Parallel()
	tool := *types.NewTool("lookup", "lookup", map[string]any{"type": "object"})
	b := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).Tools(tool).ToolChoice(types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"})
	snapshot := cloneStructuredRequest(b.request)
	tool.InputSchema["type"] = "changed"
	b.request.Tools[0].InputSchema["type"] = "builder-change"
	b.request.ToolChoice.ToolName = "other"
	if snapshot.Tools[0].InputSchema["type"] != "object" || snapshot.ToolChoice.ToolName != "lookup" {
		t.Fatal("structured tools snapshot shares mutable fields")
	}
}

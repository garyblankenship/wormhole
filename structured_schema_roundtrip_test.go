package wormhole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// Exercise the public builder through HTTP, not just the adapter with a map.
// The builder snapshots schema bytes, which must reach the wire as an object.
func TestStructuredBuilderSchemaRoundTrip(t *testing.T) {
	t.Parallel()
	for _, mode := range []types.StructuredMode{"", types.StructuredModeTools, types.StructuredModeJSON, types.StructuredModeStrict} {
		name := string(mode)
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			toolsMode := mode == "" || mode == types.StructuredModeTools
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("path = %q", r.URL.Path)
				}
				var request struct {
					Tools []struct {
						Function struct {
							Name       string         `json:"name"`
							Parameters map[string]any `json:"parameters"`
						} `json:"function"`
					} `json:"tools"`
					ToolChoice struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tool_choice"`
					ResponseFormat struct {
						Type       string `json:"type"`
						JSONSchema struct {
							Schema map[string]any `json:"schema"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var wireSchema map[string]any
				toolName := ""
				if toolsMode {
					if len(request.Tools) != 1 {
						t.Errorf("tools = %#v", request.Tools)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					wireSchema = request.Tools[0].Function.Parameters
					toolName = request.Tools[0].Function.Name
					if toolName != "structured_output" || request.ToolChoice.Function.Name != toolName {
						t.Errorf("tool name = %q, choice = %q", toolName, request.ToolChoice.Function.Name)
					}
				} else if mode == types.StructuredModeStrict {
					if request.ResponseFormat.Type != "json_schema" {
						t.Errorf("format = %q", request.ResponseFormat.Type)
					}
					wireSchema = request.ResponseFormat.JSONSchema.Schema
				} else if request.ResponseFormat.Type != "json_object" {
					t.Errorf("format = %q", request.ResponseFormat.Type)
				}
				if toolsMode || mode == types.StructuredModeStrict {
					if wireSchema["type"] != "object" {
						t.Errorf("schema type = %#v", wireSchema["type"])
					}
					props, ok := wireSchema["properties"].(map[string]any)
					if !ok {
						t.Errorf("schema properties = %#v", wireSchema["properties"])
					} else if status, ok := props["status"].(map[string]any); !ok || status["type"] != "string" {
						t.Errorf("status property = %#v", props["status"])
					}
				}
				result, err := json.Marshal(map[string]string{"status": "ok"})
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				message := map[string]any{"role": "assistant", "content": string(result)}
				finish := "stop"
				if toolsMode {
					message["content"] = nil
					message["tool_calls"] = []map[string]any{{
						"id": "call-verdict", "type": "function",
						"function": map[string]any{"name": toolName, "arguments": string(result)},
					}}
					finish = "tool_calls"
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"id": "verdict", "model": "stub-chat",
					"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": finish}},
				}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(srv.Close)
			client := New(WithLocalOpenAI(srv.URL + "/v1"))
			t.Cleanup(func() { _ = client.Close() })
			type verdict struct {
				Status string `json:"status" tool:"required"`
			}
			schema := MustSchemaFromStruct(verdict{})
			builder := client.Structured().Model("stub-chat").Prompt("classify").Schema(schema).Mode(mode)
			// Schema() takes a snapshot; later caller edits must not alter the wire.
			schema["type"] = "array"
			var output verdict
			if err := builder.GenerateAs(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			if output.Status != "ok" {
				t.Fatalf("status = %q, want ok", output.Status)
			}
		})
	}
}

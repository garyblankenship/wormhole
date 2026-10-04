package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-06: normalize strict schemas recursively without changing the caller.
func TestRemediationC406StrictSchemaRecursiveAndImmutable(t *testing.T) {
	t.Parallel()
	nested := map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": []string{"string", "null"}}}, "required": []string{"value"}}
	schema := map[string]any{"type": "object", "properties": map[string]any{"entries": map[string]any{"type": "array", "items": nested}, "choice": map[string]any{"anyOf": []any{nested, map[string]any{"type": "null"}}}}, "required": []string{"entries", "choice"}, "$defs": map[string]any{"Nested": nested}}
	before := types.CloneMap(schema)
	output, err := strictSchemaToMap(schema)
	if err != nil {
		t.Fatal(err)
	}
	if output["additionalProperties"] != false {
		t.Fatal(output)
	}
	props := output["properties"].(map[string]any)
	if props["entries"].(map[string]any)["items"].(map[string]any)["additionalProperties"] != false {
		t.Fatal(props)
	}
	if props["choice"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)["additionalProperties"] != false {
		t.Fatal(props)
	}
	if output["$defs"].(map[string]any)["Nested"].(map[string]any)["additionalProperties"] != false {
		t.Fatal(output)
	}
	if !reflect.DeepEqual(before, schema) {
		t.Fatal("caller schema mutated")
	}
	plain, err := schemaToMap(schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["additionalProperties"]; ok {
		t.Fatal("nonstrict schema normalized")
	}
}

func TestRemediationC406StrictSchemaRejectsBeforeHTTP(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		schema map[string]any
		path   string
	}{
		{"permissive", map[string]any{"type": "object", "additionalProperties": true}, "$.additionalProperties"},
		{"schema extras", map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, "$.additionalProperties"},
		{"optional", map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}, "$.properties.name"},
		{"nested optional", map[string]any{"type": "object", "required": []string{"child"}, "properties": map[string]any{"child": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}}}, "$.properties.child.properties.name"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			})
			_, err := provider.Structured(context.Background(), types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage("go")}, Schema: test.schema, Mode: types.StructuredModeStrict})
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("err=%v", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("HTTP requests=%d", requests.Load())
			}
		})
	}
}

// C4-01: direct OpenAI uses explicit system messages, preserving its existing
// behavior of leaving the separate SystemPrompt field unmapped and untouched.
func TestRemediationC401DirectProviderSystemPrompt(t *testing.T) {
	t.Parallel()
	for _, includeSystem := range []bool{false, true} {
		t.Run(map[bool]string{false: "separate field remains unmapped", true: "explicit system message"}[includeSystem], func(t *testing.T) {
			t.Parallel()
			provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if includeSystem {
					if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[0].Content != "system" {
						t.Errorf("messages=%#v", request.Messages)
					}
				} else if len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != "user" {
					t.Errorf("messages=%#v", request.Messages)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse{ID: "one", Model: "test", Choices: []chatCompletionChoice{{Message: message{Role: "assistant", Content: "done"}, FinishReason: "stop"}}})
			})
			messages := []types.Message{types.NewUserMessage("user")}
			if includeSystem {
				messages = append([]types.Message{types.NewSystemMessage("system")}, messages...)
			}
			request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, SystemPrompt: "separate field", Messages: messages}
			before := types.CloneMessages(request.Messages)
			if _, err := provider.Text(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if request.SystemPrompt != "separate field" || !reflect.DeepEqual(before, request.Messages) {
				t.Fatal("caller request mutated")
			}
		})
	}
}

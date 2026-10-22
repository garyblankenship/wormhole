package ollama

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStructuredDeliversCallerSchema(t *testing.T) {
	t.Parallel()
	for _, system := range []string{"", "existing-system"} {
		t.Run(system, func(t *testing.T) {
			t.Parallel()
			if testing.Short() {
				t.Skip("loopback integration")
			}
			schema := map[string]any{
				"type":       "object",
				"properties": map[string]any{"answer": map[string]any{"type": "integer"}},
				"required":   []string{"answer"},
			}
			schemaJSON, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload chatRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if payload.Format != "json" || len(payload.Messages) == 0 {
					t.Error("JSON mode or messages absent")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				index := len(payload.Messages) - 1
				prefix := "original"
				if system != "" {
					index, prefix = 0, system
				}
				content, ok := payload.Messages[index].Content.(string)
				if !ok || content != prefix+"\n\n"+string(schemaJSON) {
					t.Errorf("caller schema or prompt lost: %#v", payload.Messages)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				value, err := json.Marshal(map[string]any{"answer": 5})
				if err != nil {
					t.Error(err)
					return
				}
				if err := json.NewEncoder(w).Encode(chatResponse{
					Model: "fixture", Message: message{Role: "assistant", Content: string(value)}, Done: true,
				}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			provider, err := New(types.ProviderConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			user := types.NewUserMessage("original")
			request := types.StructuredRequest{
				BaseRequest: types.BaseRequest{Model: "fixture"}, Messages: []types.Message{user},
				SystemPrompt: system, Schema: schema,
			}
			response, err := provider.Structured(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			data, ok := response.Data.(map[string]any)
			if !ok || data["answer"] != float64(5) || !strings.Contains(response.Raw, "answer") {
				t.Fatalf("structured data lost: %#v", response)
			}
			if user.Content != "original" || request.SystemPrompt != system {
				t.Fatal("caller request mutated")
			}
		})
	}
}

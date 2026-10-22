package wormhole

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type omlxWireCase struct {
	name     string
	defaults *bool
	model    *bool
	request  *bool
	want     *bool
}

func TestOMLXThinkingOptionsWire(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("HTTP wire integration")
	}
	enabled, disabled := true, false
	cases := []omlxWireCase{
		{name: "omitted"},
		{name: "request_enabled", request: &enabled, want: &enabled},
		{name: "request_disabled", request: &disabled, want: &disabled},
		{name: "default_disabled", defaults: &disabled, want: &disabled},
		{name: "model_disables_default", defaults: &enabled, model: &disabled, want: &disabled},
		{name: "model_enables_default", defaults: &disabled, model: &enabled, want: &enabled},
		{name: "request_disables_model", defaults: &disabled, model: &enabled, request: &disabled, want: &disabled},
		{name: "request_enables_model", defaults: &enabled, model: &disabled, request: &enabled, want: &enabled},
	}
	for _, surface := range []string{"text", "stream", "structured"} {
		t.Run(surface, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testOMLXThinkingWire(t, surface, tc)
				})
			}
		})
	}
}

func testOMLXThinkingWire(t *testing.T, surface string, tc omlxWireCase) {
	t.Helper()
	wantContent := omlxWireContent(t)
	payloads := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request route = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("local request unexpectedly contains authentication")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case payloads <- payload:
		default:
			t.Error("unexpected additional provider request")
		}
		stream, _ := payload["stream"].(bool)
		writeOMLXWireResponse(t, w, stream, wantContent)
	}))
	t.Cleanup(server.Close)

	cfg := types.ProviderConfig{}.
		WithDefaultProviderOptions(omlxThinkingOptions(tc.defaults)).
		WithProviderOptionsForModel("omlx-test", omlxThinkingOptions(tc.model)).
		WithNoRetries()
	client := New(WithLocalOpenAI(server.URL+"/v1", cfg), WithDiscovery(false))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})

	got := generateOMLXWireContent(t, client, surface, omlxThinkingOptions(tc.request))
	if got != wantContent {
		t.Fatalf("response content = %q, want %q", got, wantContent)
	}
	select {
	case payload := <-payloads:
		assertOMLXWirePayload(t, payload, tc.want)
	default:
		t.Fatal("request never reached the HTTP fixture")
	}
}

func omlxThinkingOptions(enabled *bool) map[string]any {
	if enabled == nil {
		return nil
	}
	return map[string]any{
		"chat_template_kwargs": map[string]any{"enable_thinking": *enabled},
	}
}

func omlxWireContent(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(map[string]bool{"ok": true})
	if err != nil {
		t.Fatalf("encode content fixture: %v", err)
	}
	return string(data)
}

func writeOMLXWireResponse(t *testing.T, w http.ResponseWriter, stream bool, content string) {
	t.Helper()
	choice := map[string]any{"index": 0, "finish_reason": "stop"}
	result := map[string]any{
		"id":      "omlx-wire",
		"created": 1,
		"model":   "omlx-test",
		"choices": []any{choice},
		"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	}
	if stream {
		result["object"] = "chat.completion.chunk"
		choice["delta"] = map[string]string{"content": content}
		data, err := json.Marshal(result)
		if err != nil {
			t.Errorf("encode stream fixture: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data); err != nil {
			t.Errorf("write stream fixture: %v", err)
		}
		return
	}
	result["object"] = "chat.completion"
	choice["message"] = map[string]string{"role": "assistant", "content": content}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		t.Errorf("write response fixture: %v", err)
	}
}

func generateOMLXWireContent(t *testing.T, client *Wormhole, surface string, options map[string]any) string {
	t.Helper()
	if surface == "structured" {
		response, err := client.Structured().Model("omlx-test").Prompt("Return JSON.").
			Schema(map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}}).
			Mode(types.StructuredModeJSON).ProviderOptions(options).Generate(t.Context())
		if err != nil {
			t.Fatalf("structured generate: %v", err)
		}
		if response == nil {
			t.Fatal("nil structured response")
		}
		data, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("encode structured result: %v", err)
		}
		return string(data)
	}

	builder := client.Text().Model("omlx-test").Prompt("Return JSON.").
		ResponseFormat(map[string]any{"type": "json_object"}).ProviderOptions(options)
	if surface == "stream" {
		chunks, err := builder.Stream(t.Context())
		if err != nil {
			t.Fatalf("stream setup: %v", err)
		}
		var content strings.Builder
		for chunk := range chunks {
			if chunk.Error != nil {
				t.Fatalf("stream chunk: %v", chunk.Error)
			}
			content.WriteString(chunk.Content())
		}
		return content.String()
	}
	response, err := builder.Generate(t.Context())
	if err != nil {
		t.Fatalf("text generate: %v", err)
	}
	if response == nil {
		t.Fatal("nil text response")
	}
	return response.Content()
}

func assertOMLXWirePayload(t *testing.T, payload map[string]any, want *bool) {
	t.Helper()
	if payload["model"] != "omlx-test" {
		t.Fatalf("request model = %#v", payload["model"])
	}
	format, ok := payload["response_format"].(map[string]any)
	if !ok || format["type"] != "json_object" {
		t.Fatalf("response_format = %#v, want json_object", payload["response_format"])
	}
	kwargs, present := payload["chat_template_kwargs"]
	if want == nil {
		if present {
			t.Fatalf("omitted thinking options emitted %#v", kwargs)
		}
		return
	}
	fields, ok := kwargs.(map[string]any)
	if !present || !ok {
		t.Fatalf("chat_template_kwargs = %#v, want an object", kwargs)
	}
	enabled, ok := fields["enable_thinking"].(bool)
	if !ok || enabled != *want {
		t.Fatalf("enable_thinking = %#v, want boolean %v", fields["enable_thinking"], *want)
	}
}

package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStructuredOptionsIsolation(t *testing.T) {
	t.Parallel()
	enabled := true
	options := map[string]any{"nested": map[string]any{"value": "original"}}
	b := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).
		ProviderOptions(options).Reasoning(types.Reasoning{Effort: types.ReasoningEffortHigh, Enabled: &enabled}).TopP(0)
	enabled = false
	options["nested"].(map[string]any)["value"] = "changed"
	snapshot := cloneStructuredRequest(b.request)
	b.request.ProviderOptions["nested"].(map[string]any)["value"] = "builder-change"
	*b.request.Reasoning.Enabled = false
	*b.request.TopP = 1
	if snapshot.ProviderOptions["nested"].(map[string]any)["value"] != "original" || !*snapshot.Reasoning.Enabled || *snapshot.TopP != 0 {
		t.Fatalf("caller or builder mutation escaped snapshot: %#v", snapshot)
	}
	b.ProviderOptions(nil)
	if b.request.ProviderOptions != nil {
		t.Fatal("nil options did not clear request overrides")
	}
	empty := cloneStructuredRequest(&types.StructuredRequest{})
	if empty.ProviderOptions != nil || empty.Reasoning != nil || empty.TopP != nil {
		t.Fatal("unset controls acquired defaults")
	}
}

func TestStructuredOptionsValidateTopP(t *testing.T) {
	t.Parallel()
	for _, topP := range []float32{-0.1, 0, 1, 1.1} {
		t.Run(fmt.Sprintf("topP=%v", topP), func(t *testing.T) {
			t.Parallel()
			b := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).Model("stub").Schema(map[string]any{"type": "object"}).TopP(topP)
			if err := b.Validate(); (err != nil) != (topP < 0 || topP > 1) {
				t.Fatalf("TopP=%v validation=%v", topP, err)
			}
		})
	}
}

func TestStructuredOptionsReachProviderPayload(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"openai", "compatible", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			payloads := make(chan map[string]any, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				payloads <- payload
				w.Header().Set("Content-Type", "application/json")
				result, _ := json.Marshal(map[string]any{"value": "ok"})
				if provider == "gemini" {
					_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": string(result)}}}, "finishReason": "STOP"}}})
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "stub", "model": "stub", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": string(result)}, "finish_reason": "stop"}}})
				}
			}))
			t.Cleanup(upstream.Close)
			cfg := types.NewProviderConfig("test-key").WithDefaultProviderOptions(map[string]any{"metadata": map[string]any{"source": "default"}, "store": false}).WithProviderOptionsForModel("stub", map[string]any{"metadata": map[string]any{"source": "model"}})
			cfg.BaseURL = upstream.URL
			option := WithOpenAI("test-key", cfg)
			if provider == "gemini" {
				option = WithGemini("test-key", cfg)
			}
			if provider == "compatible" {
				option = WithOpenAICompatible(provider, upstream.URL, cfg)
			}
			client := New(option, WithDefaultProvider(provider), WithDiscovery(false))
			t.Cleanup(func() { _ = client.Close() })
			b := client.Structured().Model("stub").Prompt("classify").Schema(map[string]any{"type": "object"}).Mode(types.StructuredModeJSON)
			requestOptions := map[string]any{"metadata": map[string]any{"source": "request"}}
			b.TopP(0.75).Reasoning(types.Reasoning{Effort: types.ReasoningEffortLow}).ProviderOptions(requestOptions)
			requestOptions["metadata"].(map[string]any)["source"] = "mutated"
			if _, err := b.Generate(context.Background()); err != nil {
				t.Fatal(err)
			}
			payload := <-payloads
			if payload["metadata"].(map[string]any)["source"] != "request" || payload["store"] != false {
				t.Fatalf("option precedence/isolation lost: %#v", payload)
			}
			if provider == "gemini" {
				gen := payload["generationConfig"].(map[string]any)
				if gen["topP"] != 0.75 || gen["thinkingConfig"].(map[string]any)["thinkingLevel"] != "low" {
					t.Fatalf("generation controls lost: %#v", gen)
				}
			} else {
				if payload["top_p"] != 0.75 {
					t.Fatalf("top_p lost: %#v", payload)
				}
				if provider == "openai" && payload["reasoning_effort"] != "low" {
					t.Fatalf("reasoning effort lost: %#v", payload)
				}
				if provider == "compatible" && payload["reasoning"].(map[string]any)["effort"] != "low" {
					t.Fatalf("compatible reasoning lost: %#v", payload)
				}
			}
			// Per-request provider fields retain precedence over typed controls.
			override := map[string]any{"reasoning_effort": "high", "top_p": 0.25}
			if provider == "compatible" {
				override["reasoning"] = map[string]any{"effort": "high"}
			}
			if provider == "gemini" {
				override = map[string]any{"generationConfig": map[string]any{"topP": 0.25, "thinkingConfig": map[string]any{"thinkingLevel": "high"}}}
			}
			if _, err := b.ProviderOptions(override).Generate(context.Background()); err != nil {
				t.Fatal(err)
			}
			payload = <-payloads
			if provider == "gemini" {
				gen := payload["generationConfig"].(map[string]any)
				if gen["topP"] != 0.25 || gen["thinkingConfig"].(map[string]any)["thinkingLevel"] != "high" {
					t.Fatalf("request override lost: %#v", gen)
				}
			} else {
				if payload["top_p"] != 0.25 {
					t.Fatalf("request sampling override lost: %#v", payload)
				}
				if provider == "openai" && payload["reasoning_effort"] != "high" {
					t.Fatalf("request reasoning override lost: %#v", payload)
				}
				if provider == "compatible" && payload["reasoning"].(map[string]any)["effort"] != "high" {
					t.Fatalf("request reasoning override lost: %#v", payload)
				}
			}
		})
	}
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestChatReasoningEffortProviderMapping(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"openai", "compatible", "gemini"} {
		for _, stream := range []bool{false, true} {
			for _, effort := range []string{"omitted", "none", "low", "medium", "high"} {
				t.Run(fmt.Sprintf("%s/stream=%v/%s", provider, stream, effort), func(t *testing.T) {
					t.Parallel()
					payloads := make(chan map[string]any, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var payload map[string]any
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
							w.WriteHeader(400)
							return
						}
						payloads <- payload
						result := map[string]any{"id": "stub", "model": "stub", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "ok"}, "delta": map[string]any{"content": "ok"}, "finish_reason": "stop"}}}
						if provider == "gemini" {
							result = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "ok"}}}, "finishReason": "STOP"}}}
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							data, err := json.Marshal(result)
							if err != nil {
								t.Error(err)
								return
							}
							_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
							if provider != "gemini" {
								_, _ = io.WriteString(w, "data: [DONE]\n\n")
							}
						} else {
							w.Header().Set("Content-Type", "application/json")
							_ = json.NewEncoder(w).Encode(result)
						}
					}))
					t.Cleanup(upstream.Close)
					cfg := types.NewProviderConfig("test-key")
					cfg.BaseURL = upstream.URL
					option := wormhole.WithOpenAI("test-key", cfg)
					if provider == "compatible" {
						option = wormhole.WithOpenAICompatible(provider, upstream.URL, cfg)
					}
					if provider == "gemini" {
						option = wormhole.WithGemini("test-key", cfg)
					}
					p := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WormholeOpts: []wormhole.Option{option, wormhole.WithDefaultProvider(provider), wormhole.WithDiscovery(false)}})
					t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
					body := map[string]any{"model": provider + "/stub", "messages": []map[string]string{{"role": "user", "content": "think"}}, "stream": stream}
					if effort != "omitted" {
						body["reasoning_effort"] = effort
					}
					data, err := json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
					rec := performRequest(p, http.MethodPost, "/v1/chat/completions", string(data))
					if rec.Code != http.StatusOK {
						t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
					}
					var payload map[string]any
					select {
					case payload = <-payloads:
					default:
						t.Fatal("request never reached provider")
					}
					if provider == "openai" {
						if effort == "omitted" {
							if _, ok := payload["reasoning_effort"]; ok {
								t.Fatal("omitted effort emitted")
							}
						} else if payload["reasoning_effort"] != effort {
							t.Fatalf("effort = %#v", payload)
						}
					} else if provider == "compatible" {
						if effort == "omitted" || effort == "none" {
							if _, ok := payload["reasoning"]; ok {
								t.Fatalf("unexpected reasoning: %#v", payload)
							}
						} else if payload["reasoning"].(map[string]any)["effort"] != effort {
							t.Fatalf("reasoning = %#v", payload)
						}
					} else {
						gen, _ := payload["generationConfig"].(map[string]any)
						if effort == "omitted" || effort == "none" {
							if gen["thinkingConfig"] != nil {
								t.Fatalf("unexpected thinking config: %#v", gen)
							}
						} else if gen["thinkingConfig"].(map[string]any)["thinkingLevel"] != effort {
							t.Fatalf("thinking = %#v", gen)
						}
					}
					if stream && rec.Header().Get("Content-Type") != "text/event-stream" {
						t.Fatalf("missing streamed response: %s", rec.Body.String())
					}
				})
			}
		}
	}
}

func TestChatReasoningEffortRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, value := range []any{"", "max", "HIGH", " low ", 7, true, []string{"low"}} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/stream=%v", value, stream), func(t *testing.T) {
				t.Parallel()
				// No provider configuration: a validation error must precede upstream use.
				p := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WormholeOpts: []wormhole.Option{wormhole.WithDiscovery(false)}})
				t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
				data, err := json.Marshal(map[string]any{"model": "openai/stub", "messages": []map[string]string{{"role": "user", "content": "think"}}, "reasoning_effort": value, "stream": stream})
				if err != nil {
					t.Fatal(err)
				}
				rec := performRequest(p, http.MethodPost, "/v1/chat/completions", string(data))
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
				}
				var envelope struct {
					Error struct {
						Type    string `json:"type"`
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				wantCode := "invalid_json"
				if _, ok := value.(string); ok {
					wantCode = "unsupported_parameter"
				}
				if envelope.Error.Type != "invalid_request_error" || envelope.Error.Code != wantCode || envelope.Error.Message == "" {
					t.Fatalf("unexpected error envelope: %s", rec.Body.String())
				}
			})
		}
	}
}

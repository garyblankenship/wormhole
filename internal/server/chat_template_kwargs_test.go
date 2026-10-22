package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

// kwargsChatUpstream returns a local OpenAI-compatible fixture that records the
// decoded request body and answers with a fixed assistant message, streaming
// via SSE when stream is set. The channel buffers one payload; a second
// provider request fails the test via the default branch of a receive.
func kwargsChatUpstream(t *testing.T, payloads chan map[string]any, stream bool) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		result := map[string]any{
			"id":      "kwargs-stub",
			"object":  "chat.completion",
			"created": 1,
			"model":   "stub",
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		if stream {
			result["object"] = "chat.completion.chunk"
			result["choices"].([]any)[0].(map[string]any)["delta"] = map[string]string{"content": "ok"}
			w.Header().Set("Content-Type", "text/event-stream")
			data, err := json.Marshal(result)
			if err != nil {
				t.Errorf("encode stream fixture: %v", err)
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			return
		}
		result["choices"].([]any)[0].(map[string]any)["message"] = map[string]string{"role": "assistant", "content": "ok"}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Errorf("encode fixture: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

// newKwargsTestProxy builds a proxy from the given options; callers wire the
// providers they need, mirroring the chat-reasoning regression construction.
func newKwargsTestProxy(t *testing.T, opts ...wormhole.Option) *proxy {
	t.Helper()
	p := New(Config{
		WormholeOpts: opts,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	return p
}

// kwargsOpenAIOption points the openai provider at the local fixture.
func kwargsOpenAIOption(upstreamURL string) wormhole.Option {
	return wormhole.WithOpenAI("test-key", types.NewProviderConfig("test-key").WithBaseURL(upstreamURL))
}

func TestProxyChatTemplateKwargsForwarding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		kwargs     map[string]any
		withFormat bool
		prefixed   bool
		want       map[string]any
	}{
		{name: "omitted"},
		{name: "enable_false", kwargs: map[string]any{"enable_thinking": false}, want: map[string]any{"enable_thinking": false}},
		{
			name:       "enable_true_with_json_mode",
			kwargs:     map[string]any{"enable_thinking": true},
			withFormat: true,
			want:       map[string]any{"enable_thinking": true},
		},
		{
			name:     "extra_kwargs_prefixed_route",
			kwargs:   map[string]any{"enable_thinking": true, "model": "hijack", "reasoning_template": "qwen", "max_items": 3},
			prefixed: true,
			want:     map[string]any{"enable_thinking": true, "model": "hijack", "reasoning_template": "qwen", "max_items": float64(3)},
		},
	}
	for _, stream := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/stream=%v", tc.name, stream), func(t *testing.T) {
				t.Parallel()
				payloads := make(chan map[string]any, 1)
				upstream := kwargsChatUpstream(t, payloads, stream)
				p := newKwargsTestProxy(t, kwargsOpenAIOption(upstream.URL), wormhole.WithDefaultProvider("openai"), wormhole.WithDiscovery(false))

				model := "stub"
				if tc.prefixed {
					model = "openai/stub"
				}
				body := map[string]any{
					"model":    model,
					"messages": []map[string]string{{"role": "user", "content": "hi"}},
					"stream":   stream,
				}
				if tc.kwargs != nil {
					body["chat_template_kwargs"] = tc.kwargs
				}
				if tc.withFormat {
					body["response_format"] = map[string]any{"type": "json_object"}
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
				if payload["model"] != "stub" {
					t.Fatalf("upstream model = %#v, want stub", payload["model"])
				}
				messages, ok := payload["messages"].([]any)
				if !ok || len(messages) != 1 {
					t.Fatalf("upstream messages = %#v, want one preserved message", payload["messages"])
				}
				if tc.withFormat {
					format, ok := payload["response_format"].(map[string]any)
					if !ok || format["type"] != "json_object" {
						t.Fatalf("response_format = %#v, want json_object", payload["response_format"])
					}
				} else if _, present := payload["response_format"]; present {
					t.Fatalf("unexpected response_format: %#v", payload["response_format"])
				}
				if tc.want == nil {
					if raw, present := payload["chat_template_kwargs"]; present {
						t.Fatalf("omitted kwargs forwarded %#v", raw)
					}
				} else {
					got, _ := payload["chat_template_kwargs"].(map[string]any)
					if !reflect.DeepEqual(got, tc.want) {
						t.Fatalf("chat_template_kwargs = %#v, want %#v", got, tc.want)
					}
				}
				if stream {
					if rec.Header().Get("Content-Type") != "text/event-stream" {
						t.Fatalf("missing streamed response: %s", rec.Body.String())
					}
					return
				}
				var out ChatCompletionResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if len(out.Choices) != 1 || out.Choices[0].Message == nil || out.Choices[0].Message.Content != "ok" {
					t.Fatalf("unexpected chat response: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestProxyChatTemplateKwargsRejectsInvalidBeforeUpstream(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		kwargs any
	}{
		{name: "explicit_null", kwargs: json.RawMessage("null")},
		{name: "string_scalar", kwargs: "qwen"},
		{name: "number_scalar", kwargs: 42},
		{name: "array", kwargs: []any{1, 2}},
		{name: "enable_thinking_string", kwargs: map[string]any{"enable_thinking": "yes"}},
		{name: "enable_thinking_number", kwargs: map[string]any{"enable_thinking": 1}},
		{name: "enable_thinking_object", kwargs: map[string]any{"enable_thinking": map[string]any{"nested": true}}},
		{name: "enable_thinking_null", kwargs: map[string]any{"enable_thinking": nil}},
	}
	for _, stream := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/stream=%v", tc.name, stream), func(t *testing.T) {
				t.Parallel()
				payloads := make(chan map[string]any, 1)
				upstream := kwargsChatUpstream(t, payloads, stream)
				p := newKwargsTestProxy(t, kwargsOpenAIOption(upstream.URL), wormhole.WithDiscovery(false))

				body := map[string]any{
					"model":                "stub",
					"messages":             []map[string]string{{"role": "user", "content": "hi"}},
					"chat_template_kwargs": tc.kwargs,
					"stream":               stream,
				}
				data, err := json.Marshal(body)
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
				if envelope.Error.Type != "invalid_request_error" || envelope.Error.Code != "invalid_request_error" || envelope.Error.Message == "" {
					t.Fatalf("unexpected error envelope: %s", rec.Body.String())
				}
				select {
				case payload := <-payloads:
					t.Fatalf("invalid kwargs reached the provider: %#v", payload)
				default:
				}
			})
		}
	}
}

func TestProxyChatTemplateKwargsRejectsNativeProviders(t *testing.T) {
	t.Parallel()
	nativeOption := func(provider, upstreamURL string) wormhole.Option {
		cfg := types.NewProviderConfig("test-key")
		cfg.BaseURL = upstreamURL
		switch provider {
		case "anthropic":
			return wormhole.WithAnthropic("test-key", cfg)
		case "gemini":
			return wormhole.WithGemini("test-key", cfg)
		default:
			return wormhole.WithOllama(cfg)
		}
	}
	cases := []struct {
		name         string
		provider     string
		model        string
		setDefaulter bool
	}{
		{name: "anthropic_prefix", provider: "anthropic", model: "anthropic/claude-sonnet-4-5"},
		{name: "gemini_prefix", provider: "gemini", model: "gemini/gemini-2.5-pro"},
		{name: "ollama_prefix", provider: "ollama", model: "ollama/llama3.2"},
		{name: "anthropic_default", provider: "anthropic", model: "claude-sonnet-4-5", setDefaulter: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payloads := make(chan map[string]any, 1)
			upstream := kwargsChatUpstream(t, payloads, false)
			opts := []wormhole.Option{nativeOption(tc.provider, upstream.URL), wormhole.WithDiscovery(false)}
			if tc.setDefaulter {
				opts = append(opts, wormhole.WithDefaultProvider(tc.provider))
			}
			p := newKwargsTestProxy(t, opts...)

			data, err := json.Marshal(map[string]any{
				"model":                tc.model,
				"messages":             []map[string]string{{"role": "user", "content": "hi"}},
				"chat_template_kwargs": map[string]any{"enable_thinking": false},
			})
			if err != nil {
				t.Fatal(err)
			}
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions", string(data))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			var out ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Error.Code != "unsupported_parameter" || out.Error.Message == "" {
				t.Fatalf("unexpected error envelope: %s", rec.Body.String())
			}
			select {
			case payload := <-payloads:
				t.Fatalf("native route reached the provider: %#v", payload)
			default:
			}
		})
	}
}

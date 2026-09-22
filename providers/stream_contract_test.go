package providers_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/providers/anthropic"
	"github.com/garyblankenship/wormhole/v3/providers/gemini"
	"github.com/garyblankenship/wormhole/v3/providers/ollama"
	"github.com/garyblankenship/wormhole/v3/providers/openai"
	"github.com/garyblankenship/wormhole/v3/types"
)

// These fixtures exercise the HTTP-to-public-channel boundary, including the
// accumulation and provider-stamping goroutines that parser-only tests omit.
var streamContracts = []struct {
	name     string
	new      func(*testing.T, types.ProviderConfig) types.Provider
	ndjson   bool
	partial  string
	terminal string
}{
	{
		name:     "openai_chat",
		new:      func(_ *testing.T, c types.ProviderConfig) types.Provider { return openai.New(c) },
		partial:  `{"choices":[{"delta":{"content":"partial"}}]}`,
		terminal: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	},
	{
		name:     "openai_compatible",
		new:      func(_ *testing.T, c types.ProviderConfig) types.Provider { return openai.NewWithName("gateway", c) },
		partial:  `{"choices":[{"delta":{"content":"partial"}}]}`,
		terminal: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	},
	{
		name: "openai_responses",
		new: func(_ *testing.T, c types.ProviderConfig) types.Provider {
			c.UseResponsesAPI = true
			return openai.New(c)
		},
		partial:  `{"type":"response.output_text.delta","delta":"partial"}`,
		terminal: `{"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`,
	},
	{
		name:     "anthropic",
		new:      func(_ *testing.T, c types.ProviderConfig) types.Provider { return anthropic.New(c) },
		partial:  `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
		terminal: `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	},
	{
		name:     "gemini",
		new:      func(_ *testing.T, c types.ProviderConfig) types.Provider { return gemini.New("test-key", c) },
		partial:  `{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`,
		terminal: `{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}`,
	},
	{
		name: "ollama",
		new: func(t *testing.T, c types.ProviderConfig) types.Provider {
			t.Helper()
			provider, err := ollama.New(c)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			return provider
		},
		ndjson:   true,
		partial:  `{"model":"test-model","message":{"role":"assistant","content":"partial"},"done":false}`,
		terminal: `{"model":"test-model","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`,
	},
}

func streamContractRequest() types.TextRequest {
	return types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "test-model"},
		Messages:    []types.Message{types.NewUserMessage("test")},
	}
}

func TestProviderStreamTerminationContract(t *testing.T) {
	t.Parallel()
	for _, adapter := range streamContracts {
		t.Run(adapter.name, func(t *testing.T) {
			t.Parallel()
			frame := func(data string) string {
				if adapter.ndjson {
					return data + "\n"
				}
				return "data: " + data + "\n\n"
			}
			for _, tc := range []struct {
				name      string
				body      string
				wantText  string
				wantError bool
			}{
				{name: "empty", wantError: true},
				{name: "blank", body: "\n\n", wantError: true},
				{name: "unknown_event", body: frame(`{"future_field":true}`), wantError: true},
				{name: "truncated", body: frame(adapter.partial), wantText: "partial", wantError: true},
				{name: "malformed_frame", body: frame(adapter.partial) + frame(`{"broken":`), wantText: "partial", wantError: true},
				{name: "complete", body: frame(adapter.partial) + frame(adapter.terminal), wantText: "partial"},
				{name: "empty_completion", body: frame(adapter.terminal)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, tc.body)
					}))
					t.Cleanup(server.Close)
					provider := adapter.new(t, types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
					t.Cleanup(func() { _ = provider.Close() })
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					stream, err := provider.Stream(ctx, streamContractRequest())
					if err != nil {
						t.Fatalf("Stream: %v", err)
					}
					var text strings.Builder
					errors, finishes := 0, 0
					for chunk := range stream {
						text.WriteString(chunk.Content())
						if chunk.Error != nil {
							errors++
						}
						if chunk.IsDone() {
							finishes++
						}
					}
					if ctx.Err() != nil {
						t.Fatalf("stream failed to close: %v", ctx.Err())
					}
					wantErrors, wantFinishes := 0, 1
					if tc.wantError {
						wantErrors, wantFinishes = 1, 0
					}
					if errors != wantErrors || finishes != wantFinishes || text.String() != tc.wantText {
						t.Fatalf("text=%q errors=%d finishes=%d; want text=%q errors=%d finishes=%d", text.String(), errors, finishes, tc.wantText, wantErrors, wantFinishes)
					}
				})
			}
		})
	}
}

func TestProviderStreamCancellationContract(t *testing.T) {
	t.Parallel()
	for _, adapter := range streamContracts {
		t.Run(adapter.name, func(t *testing.T) {
			t.Parallel()
			for _, afterChunk := range []bool{false, true} {
				name := "waiting_for_first_chunk"
				if afterChunk {
					name = "after_partial_chunk"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					upstreamClosed := make(chan struct{})
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						defer close(upstreamClosed)
						w.Header().Set("Content-Type", "text/event-stream")
						if afterChunk {
							if adapter.ndjson {
								_, _ = io.WriteString(w, adapter.partial+"\n")
							} else {
								_, _ = io.WriteString(w, "data: "+adapter.partial+"\n\n")
							}
						}
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}))
					t.Cleanup(server.Close)
					provider := adapter.new(t, types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
					t.Cleanup(func() { _ = provider.Close() })
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					stream, err := provider.Stream(ctx, streamContractRequest())
					if err != nil {
						t.Fatalf("Stream: %v", err)
					}
					if afterChunk {
						select {
						case chunk, ok := <-stream:
							if !ok || chunk.Error != nil || chunk.Content() != "partial" {
								t.Fatalf("first chunk = %+v, open=%v", chunk, ok)
							}
						case <-time.After(5 * time.Second):
							t.Fatal("no partial chunk")
						}
					}
					cancel()
					deadline := time.NewTimer(5 * time.Second)
					defer deadline.Stop()
					for {
						select {
						case _, ok := <-stream:
							if !ok {
								select {
								case <-upstreamClosed:
									return
								case <-deadline.C:
									t.Fatal("upstream request remained open after cancellation")
								}
							}
						case <-deadline.C:
							t.Fatal("stream remained open after cancellation")
						}
					}
				})
			}
		})
	}
}

func TestProviderStreamMalformedToolArguments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		body      string
		wantError bool
	}{
		{
			name: "openai_responses",
			body: `data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[{"type":"function_call","id":"item1","call_id":"call1","name":"lookup","arguments":"{\"q\":"}]}}` + "\n\n",
		},
		{
			name:      "gemini",
			body:      `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":"invalid object"}}]},"finishReason":"STOP"}]}` + "\n\n",
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			var provider types.Provider
			for _, adapter := range streamContracts {
				if adapter.name == tc.name {
					provider = adapter.new(t, types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
					break
				}
			}
			if provider == nil {
				t.Fatalf("missing adapter %q", tc.name)
			}
			t.Cleanup(func() { _ = provider.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stream, err := provider.Stream(ctx, streamContractRequest())
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			errors, invalidCalls := 0, 0
			for chunk := range stream {
				if chunk.Error != nil {
					errors++
				}
				calls := append([]types.ToolCall(nil), chunk.ToolCalls...)
				if chunk.ToolCall != nil {
					calls = append(calls, *chunk.ToolCall)
				}
				if chunk.Delta != nil {
					calls = append(calls, chunk.Delta.ToolCalls...)
				}
				for _, call := range calls {
					if !call.ArgsInvalid || call.ArgsParseError == "" || call.Arguments != nil {
						t.Errorf("malformed arguments surfaced as executable call: %+v", call)
					}
					invalidCalls++
				}
			}
			if ctx.Err() != nil {
				t.Fatalf("stream failed to close: %v", ctx.Err())
			}
			wantErrors, wantInvalidCalls := 0, 1
			if tc.wantError {
				wantErrors, wantInvalidCalls = 1, 0
			}
			if errors != wantErrors || invalidCalls != wantInvalidCalls {
				t.Fatalf("errors=%d invalid calls=%d; want errors=%d invalid calls=%d", errors, invalidCalls, wantErrors, wantInvalidCalls)
			}
		})
	}
}

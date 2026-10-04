package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

type remediationStreamProvider struct {
	*wmtest.MockProvider
	source  chan types.TextChunk
	started chan context.Context
}

func (p *remediationStreamProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	p.started <- ctx
	return p.source, nil
}

func remediationProxyProvider(p *remediationStreamProvider) *proxy {
	return New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WormholeOpts: []wormhole.Option{
		wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) { return p, nil }),
		wormhole.WithProviderConfig("openai", types.ProviderConfig{}), wormhole.WithDefaultProvider("openai"), wormhole.WithDiscovery(false),
	}})
}

type remediationFailWriter struct {
	*httptest.ResponseRecorder
	remaining int
	writes    int
}

func (w *remediationFailWriter) Write(b []byte) (int, error) {
	w.writes++
	if w.remaining == 0 {
		return 0, errors.New("client gone")
	}
	w.remaining--
	return w.ResponseRecorder.Write(b)
}

func TestRemediationC3_01_C3_02StreamingWriteFailureCancels(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
		for _, allowed := range []int{0, 1} {
			t.Run(endpoint+string(rune('0'+allowed)), func(t *testing.T) {
				t.Parallel()
				provider := &remediationStreamProvider{MockProvider: wmtest.NewMockProvider("openai"), source: make(chan types.TextChunk, 3), started: make(chan context.Context, 1)}
				provider.source <- types.TextChunk{Text: "one"}
				provider.source <- types.TextChunk{Text: "two"}
				provider.source <- types.TextChunk{Text: "three"}
				p := remediationProxyProvider(provider)
				request := remediationStreamRequest(endpoint)
				writer := &remediationFailWriter{ResponseRecorder: httptest.NewRecorder(), remaining: allowed}
				done := make(chan struct{})
				go func() { p.server.Handler.ServeHTTP(writer, request); close(done) }()
				var upstream context.Context
				select {
				case upstream = <-provider.started:
				case <-time.After(time.Second):
					t.Fatal("upstream did not start")
				}
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("handler did not stop after writer failure")
				}
				select {
				case <-upstream.Done():
				case <-time.After(time.Second):
					t.Fatal("upstream was not canceled")
				}
				if writer.writes != allowed+1 {
					t.Fatalf("writes after failure: %d", writer.writes)
				}
				if body := writer.Body.String(); strings.Contains(body, "[DONE]") || strings.Contains(body, "response.completed") {
					t.Fatalf("completion after failure: %s", body)
				}
			})
		}
	}
}

func remediationStreamRequest(endpoint string) *http.Request {
	body := map[string]any{"model": "gpt-test", "stream": true}
	if endpoint == "/v1/responses" {
		body["input"] = "hello"
	} else {
		body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
	}
	data, _ := json.Marshal(body)
	return httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(data)))
}

func TestRemediationC3_01_C3_02CanceledStalledStream(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			provider := &remediationStreamProvider{MockProvider: wmtest.NewMockProvider("openai"), source: make(chan types.TextChunk), started: make(chan context.Context, 1)}
			p := remediationProxyProvider(provider)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				p.server.Handler.ServeHTTP(rec, remediationStreamRequest(endpoint).WithContext(ctx))
				close(done)
			}()
			select {
			case <-provider.started:
			case <-time.After(time.Second):
				t.Fatal("provider did not start")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("stalled stream blocked cancellation")
			}
			if strings.Contains(rec.Body.String(), "[DONE]") || strings.Contains(rec.Body.String(), "response.completed") {
				t.Fatal("completion after cancellation")
			}
		})
	}
}

func TestRemediationC3_I02ChatRoleAndToolIndices(t *testing.T) {
	t.Parallel()
	finish := types.FinishReasonToolCalls
	fragment := func(index int, id, name, args string) types.TextChunk {
		return types.TextChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: index, ID: id, Name: name, Function: &types.ToolCallFunction{Arguments: args}}}}}
	}
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{
		fragment(3, "a", "alpha", "{"), fragment(7, "b", "beta", "["), fragment(3, "", "", "}"), fragment(7, "", "", "]"),
		{FinishReason: &finish, ToolCalls: []types.ToolCall{{Index: 3, ID: "a", Name: "alpha", Function: &types.ToolCallFunction{Arguments: "{}"}}}},
	})
	p := newTestProxy(mock)
	rec := httptest.NewRecorder()
	p.server.Handler.ServeHTTP(rec, remediationStreamRequest("/v1/chat/completions"))
	var roles int
	args := map[int]string{}
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var response ChatCompletionResponse
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &response); err != nil {
			t.Fatal(err)
		}
		for _, choice := range response.Choices {
			if choice.Delta.Role != "" {
				roles++
			}
			for _, call := range choice.Delta.ToolCalls {
				args[*call.Index] += call.Function.Arguments
			}
		}
	}
	if roles != 1 || args[3] != "{}" || args[7] != "[]" {
		t.Fatalf("roles=%d args=%v", roles, args)
	}
}

func TestRemediationC3_I01ResponsesToolsLiveBeforeTerminal(t *testing.T) {
	t.Parallel()
	provider := &remediationStreamProvider{MockProvider: wmtest.NewMockProvider("openai"), source: make(chan types.TextChunk), started: make(chan context.Context, 1)}
	p := remediationProxyProvider(provider)
	writer := &remediationObservingWriter{ResponseRecorder: httptest.NewRecorder(), events: make(chan string, 32)}
	done := make(chan struct{})
	go func() { p.server.Handler.ServeHTTP(writer, remediationStreamRequest("/v1/responses")); close(done) }()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	send := func(chunk types.TextChunk) {
		t.Helper()
		select {
		case provider.source <- chunk:
		case <-time.After(time.Second):
			t.Fatal("blocked provider send")
		}
	}
	send(types.TextChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 5, ID: "a", Name: "alpha", Function: &types.ToolCallFunction{Arguments: "{"}}}}})
	foundAdded, foundDelta := false, false
	deadline := time.After(time.Second)
	for !foundAdded || !foundDelta {
		select {
		case frame := <-writer.events:
			foundAdded = foundAdded || strings.Contains(frame, "response.output_item.added")
			foundDelta = foundDelta || strings.Contains(frame, "response.function_call_arguments.delta")
		case <-deadline:
			t.Fatal("tool opener/delta buffered until terminal")
		}
	}
	send(types.TextChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 9, ID: "b", Name: "beta", Function: &types.ToolCallFunction{Arguments: "[]"}}}}})
	send(types.TextChunk{Text: "text", Refusal: "refusal"})
	send(types.TextChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 5, Function: &types.ToolCallFunction{Arguments: "}"}}}}})
	finish := types.FinishReasonToolCalls
	send(types.TextChunk{FinishReason: &finish, ToolCalls: []types.ToolCall{{Index: 5, ID: "a", Name: "alpha", Function: &types.ToolCallFunction{Arguments: "{}"}}}, Usage: &types.Usage{TotalTokens: 4}})
	close(provider.source)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not complete")
	}
	body := writer.Body.String()
	if strings.Count(body, "event: response.output_item.added") != 3 || strings.Count(body, "event: response.output_item.done") != 3 || strings.Count(body, "event: response.function_call_arguments.delta") != 3 {
		t.Fatalf("incorrect event counts: %s", body)
	}
	for _, want := range []string{`"arguments":"{}"`, `"arguments":"[]"`, `"total_tokens":4`, `"output_index":2`, "response.refusal.done", "response.completed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s: %s", want, body)
		}
	}
}

type remediationObservingWriter struct {
	*httptest.ResponseRecorder
	events chan string
}

func (w *remediationObservingWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(b)
	w.events <- string(b)
	return n, err
}

func TestRemediationC3_04AuthenticationCaseAndExactTokens(t *testing.T) {
	t.Parallel()
	p := &proxy{apiKey: "Token-Case"}
	h := p.auth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		header string
		status int
	}{
		{"bEaReR Token-Case", http.StatusNoContent}, {"Bearer token-case", http.StatusUnauthorized}, {"Bearer Token-Case ", http.StatusUnauthorized}, {"Bearer", http.StatusUnauthorized}, {"Basic Token-Case", http.StatusUnauthorized},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", tc.header)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("header %q status=%d want=%d", tc.header, rec.Code, tc.status)
		}
	}
}

func TestRemediationC3_I03ResponsesSDKCodeLowercase(t *testing.T) {
	t.Parallel()
	err := types.NewWormholeError(types.ErrorCodeTimeout, "private upstream body", false)
	if got := responsesErrorCode(err); got != "timeout_error" {
		t.Fatalf("code=%q", got)
	}
	rec := httptest.NewRecorder()
	sse := responsesSSEWriter{w: rec}
	writeResponsesFailure(&sse, "r", "m", 1, err)
	if strings.Contains(rec.Body.String(), "private upstream body") {
		t.Fatal("upstream body leaked")
	}
}

func TestRemediationC3_R02CleartextWarningOnce(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	p := New(Config{Addr: ":invalid-port", ProxyAPIKey: "fixture-key", Logger: slog.New(slog.NewTextHandler(&logs, nil)), WormholeOpts: []wormhole.Option{wormhole.WithDiscovery(false)}})
	for range 2 {
		if p.Start() == nil {
			t.Fatal("invalid address unexpectedly started")
		}
	}
	if strings.Count(logs.String(), "trusted TLS proxy") != 1 {
		t.Fatalf("warning count: %s", logs.String())
	}
}

func TestRemediationC3_I01CustomToolLiveInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ raw, want string }{
		{`{"input":"abc`, "abc"},
		{`{"input":"abc\`, "abc"},
		{`{"input":"abc\nxyz`, "abc\nxyz"},
		{`{"input":"abc\nxyz"}`, "abc\nxyz"},
	} {
		if got := partialCustomToolInput(tc.raw); got != tc.want {
			t.Fatalf("raw=%q got=%q want=%q", tc.raw, got, tc.want)
		}
	}
}

type remediationShortWriter struct{ *httptest.ResponseRecorder }

func (w remediationShortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestRemediationC3_02ShortWriteStopsEvents(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sse := responsesSSEWriter{w: remediationShortWriter{httptest.NewRecorder()}, ctx: ctx, cancel: cancel}
	sse.write(responsesEvent{Type: "response.created"})
	if !errors.Is(sse.err, io.ErrShortWrite) || ctx.Err() == nil {
		t.Fatalf("error=%v context=%v", sse.err, ctx.Err())
	}
	sequence := sse.sequence
	sse.write(responsesEvent{Type: "response.completed"})
	if sse.sequence != sequence {
		t.Fatal("writer accepted another event after failure")
	}
}

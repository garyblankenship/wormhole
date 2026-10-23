package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type evidenceClient struct {
	body   string
	calls  int
	status int
}

func (c *evidenceClient) Do(*http.Request) (*http.Response, error) {
	c.calls++
	status := c.status
	if status == 0 {
		status = 200
	}
	return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": {"request-1"}}, Body: io.NopCloser(strings.NewReader(c.body))}, nil
}

func TestTextEvidencePreservesOriginalAndNormalizesOnce(t *testing.T) {
	t.Parallel()
	body := `{"id":"one","model":"model","continuation":{"secret":"opaque"},"choices":[{"index":0,"message":{"role":"assistant","content":"` + "```json\\n{\\\"ok\\\":true}\\n```" + `","refusal":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"act","arguments":" { \\\"n\\\": 1 } "}}]},"finish_reason":"tool_calls"},{"index":1,"message":{"role":"assistant","content":"other","refusal":"refused"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":2,"total_tokens":2}}`
	// Marshal a fixture to retain intentional whitespace in argument text.
	var fixture map[string]any
	if err := json.Unmarshal([]byte(body), &fixture); err != nil {
		t.Fatal(err)
	}
	choices := fixture["choices"].([]any)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	message["content"] = "```json\n{\"ok\":true}\n```"
	message["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"] = " { \"n\": 1 } "
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	client := &evidenceClient{body: string(encoded)}
	p := NewWithNameAndHTTPClient("compatible", types.ProviderConfig{NoAuth: true}, client)
	result, err := p.TextWithEvidence(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "model"}})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || !result.Evidence.Complete || string(result.Evidence.RawBody) != string(encoded) {
		t.Fatalf("capture = %+v, calls=%d", result, client.calls)
	}
	if result.Response.Text != `{"ok":true}` || len(result.Evidence.Choices) != 2 {
		t.Fatalf("result=%+v", result)
	}
	first := result.Evidence.Choices[0]
	var original string
	if err := json.Unmarshal(first.Content, &original); err != nil {
		t.Fatal(err)
	}
	if original != message["content"] || first.ToolCalls[0].Function.Arguments != " { \"n\": 1 } " {
		t.Fatalf("choice=%+v", first)
	}
	if string(first.Refusal) != "null" || result.Evidence.Usage.PromptTokens == nil || *result.Evidence.Usage.PromptTokens != 0 || result.Evidence.Provider != "compatible" {
		t.Fatal("lost presence or identity")
	}
	if result.Evidence.ResponseMetadata.RequestID != "request-1" {
		t.Fatal("metadata missing")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Text(context.Background(), types.TextRequest{}); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatal("injected client replaced")
	}
}

func TestTextEvidenceUsagePresenceAndNormalizationFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, usage string
		want        *int64
		wantErr     bool
	}{
		{"absent", "", nil, false}, {"null", `,"usage":null`, nil, false},
		{"null_count", `,"usage":{"prompt_tokens":null}`, nil, false},
		{"zero", `,"usage":{"prompt_tokens":0}`, new(int64), false},
		{"negative", `,"usage":{"prompt_tokens":-1}`, nil, false},
		{"fractional", `,"usage":{"prompt_tokens":1.5}`, nil, true},
		{"string", `,"usage":{"prompt_tokens":"3"}`, nil, true},
		{"overflow", `,"usage":{"prompt_tokens":9223372036854775808}`, nil, true},
		{"wrong_type", `,"usage":[]`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]` + tc.usage + `}`
			client := &evidenceClient{body: body}
			result, err := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client).TextWithEvidence(context.Background(), types.TextRequest{})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if result == nil || !result.Evidence.Complete || string(result.Evidence.RawBody) != body || client.calls != 1 {
				t.Fatal("lost complete body")
			}
			got := result.Evidence.Usage.PromptTokens
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Fatalf("count=%v", got)
			}
		})
	}
}

func TestTextEvidenceRejectsUnsupportedBeforeDispatch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		config  types.ProviderConfig
		request types.TextRequest
	}{
		{"responses", types.ProviderConfig{NoAuth: true, UseResponsesAPI: true}, types.TextRequest{}},
		{"stream", types.ProviderConfig{NoAuth: true}, types.TextRequest{BaseRequest: types.BaseRequest{ProviderOptions: map[string]any{"stream": true}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &evidenceClient{}
			_, err := NewWithHTTPClient(tc.config, client).TextWithEvidence(context.Background(), tc.request)
			if err == nil || client.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, client.calls)
			}
		})
	}
}

func TestTextEvidenceNoRetryAndCompleteMalformedBody(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			client := &evidenceClient{body: `{"broken":`, status: status}
			result, err := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client).TextWithEvidence(context.Background(), types.TextRequest{})
			if err == nil || result == nil || !result.Evidence.Complete || client.calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, client.calls)
			}
		})
	}
}

type failingEvidenceBody struct{}

func (failingEvidenceBody) Read([]byte) (int, error) { return 0, errors.New("connection lost") }
func (failingEvidenceBody) Close() error             { return nil }

type brokenEvidenceClient struct{}

func (brokenEvidenceClient) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: failingEvidenceBody{}}, nil
}

func TestTextEvidenceIncompleteAndOverflowNeverCertified(t *testing.T) {
	t.Parallel()
	t.Run("incomplete", func(t *testing.T) {
		t.Parallel()
		result, err := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, brokenEvidenceClient{}).TextWithEvidence(context.Background(), types.TextRequest{})
		if err == nil || result.Evidence.Complete || result.Evidence.RawBody != nil {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})
	t.Run("overflow", func(t *testing.T) {
		t.Parallel()
		client := &evidenceClient{body: strings.Repeat("x", (32<<20)+1)}
		result, err := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client).TextWithEvidence(context.Background(), types.TextRequest{})
		if err == nil || result.Evidence.Complete || result.Evidence.RawBody != nil || client.calls != 1 {
			t.Fatal("overflow certified")
		}
	})
}

func TestTextEvidenceLegacyFunctionAndDetailedCounts(t *testing.T) {
	t.Parallel()
	body := `{"choices":[{"message":{"role":"assistant","content":"ok","function_call":{"name":"legacy","arguments":" { } "}},"finish_reason":"function_call"}],"usage":{"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":-1}}}`
	client := &evidenceClient{body: body}
	result, err := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client).TextWithEvidence(context.Background(), types.TextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	call := result.Evidence.Choices[0].FunctionCall
	if call == nil || call.Name != "legacy" || call.Arguments != " { } " {
		t.Fatal("legacy function lost")
	}
	usage := result.Evidence.Usage
	if usage.CachedTokens == nil || *usage.CachedTokens != 0 || usage.ReasoningTokens != nil || usage.PromptTokens != nil || usage.TotalTokens != nil {
		t.Fatal("usage synthesized")
	}
}

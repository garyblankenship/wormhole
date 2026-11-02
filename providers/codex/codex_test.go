package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func fixtureToken(exp int64) string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp))) + ".signature"
}

func TestCodexCredentialBoundsAndExpiry(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "auth.json")
	credential := Credential{AccessToken: fixtureToken(time.Now().Add(time.Hour).Unix()), AccountID: "account-1"}
	data, _ := json.Marshal(map[string]any{"tokens": map[string]string{"access_token": credential.AccessToken, "account_id": credential.AccountID}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCredential(path, time.Now())
	if err != nil || got != credential {
		t.Fatalf("credential mismatch: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, 1<<20+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredential(path, time.Now()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized auth: %v", err)
	}
	for _, invalid := range []Credential{{AccessToken: fixtureToken(time.Now().Add(-time.Hour).Unix()), AccountID: "account-1"}, {AccessToken: "secret", AccountID: "account-1"}, {AccessToken: credential.AccessToken}} {
		_, err := ValidateCredential(invalid, time.Now())
		if err == nil || strings.Contains(err.Error(), invalid.AccessToken) {
			t.Fatalf("invalid credential leaked or accepted: %v", err)
		}
	}
}

func TestCodexWireAndCompleted(t *testing.T) {
	t.Parallel()
	credential := Credential{AccessToken: fixtureToken(time.Now().Add(time.Hour).Unix()), AccountID: "account-1"}
	server := httptest.NewServer(codexWireHandler(t, credential))
	t.Cleanup(server.Close)
	p, err := New(types.ProviderConfig{BaseURL: server.URL, DefaultProviderOptions: map[string]any{"text": map[string]any{"verbosity": "low"}}}.WithNoRetries(), credential, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	budget := 32768
	req := types.TextRequest{BaseRequest: types.BaseRequest{Model: "caller-model", MaxTokens: &budget, Reasoning: &types.Reasoning{Effort: types.ReasoningEffortLow}}, SystemPrompt: "rules", Messages: []types.Message{types.NewUserMessage("input")}}
	resp, err := p.Text(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello" || resp.ID != "resp-1" || resp.Usage.CacheReadTokens != 2 || resp.Usage.ReasoningTokens != 1 {
		t.Fatalf("response: %+v", resp)
	}
	chunks, err := p.Stream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	var delta string
	var terminal bool
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		delta += chunk.Text
		terminal = terminal || chunk.FinishReason != nil
	}
	if delta != "hello" || !terminal {
		t.Fatalf("stream delta=%q terminal=%t", delta, terminal)
	}
}

func codexWireHandler(t *testing.T, credential Credential) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/codex/responses" || r.Header.Get("Authorization") != "Bearer "+credential.AccessToken || r.Header.Get("chatgpt-account-id") != credential.AccountID {
			t.Errorf("codex request path or auth mismatch")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		input, ok := payload["input"].([]any)
		if !ok || len(input) != 1 || payload["instructions"] != "rules" || payload["store"] != false || payload["stream"] != true || payload["max_output_tokens"] != float64(32768) {
			t.Errorf("payload: %v", payload)
		}
		if effort := payload["reasoning"].(map[string]any)["effort"]; effort != "low" {
			t.Errorf("effort: %v", effort)
		}
		if includes := payload["include"].([]any); len(includes) != 1 || includes[0] != "reasoning.encrypted_content" {
			t.Errorf("include: %v", includes)
		}
		if text := payload["text"].(map[string]any); text["verbosity"] != "low" {
			t.Errorf("text verbosity: %v", text)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"); err != nil {
			t.Error(err)
		}
		if _, err := fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\",\"model\":\"caller-model\",\"status\":\"completed\",\"output\":[{\"type\":\"reasoning\",\"summary\":[{\"type\":\"summary_text\",\"text\":\"private\"}],\"encrypted_content\":\"opaque\"},{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":9,\"output_tokens\":3,\"total_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":2},\"output_tokens_details\":{\"reasoning_tokens\":1}}}}\n\n"); err != nil {
			t.Error(err)
		}
	}
}

func TestCodexStreamFailures(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]string{
		"truncated":   "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		"malformed":   "data: {bad}\n\n",
		"failed":      "data: {\"type\":\"response.failed\"}\n\n",
		"incomplete":  "data: {\"type\":\"response.incomplete\"}\n\n",
		"unframed":    "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}",
		"tool":        "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"m\",\"status\":\"completed\",\"output\":[{\"type\":\"function_call\"}]}}\n\n",
		"tool-result": "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"m\",\"status\":\"completed\",\"output\":[{\"type\":\"function_call_output\"}]}}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := consume(t.Context(), strings.NewReader(fixture), nil)
			if err == nil {
				t.Fatal("accepted failed or truncated event")
			}
		})
	}
}

func TestCodexCanceledSSE(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := consume(ctx, strings.NewReader("data: {}\n\n"), nil); err != context.Canceled {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestCodexRejectsContractOverrides(t *testing.T) {
	t.Parallel()
	credential := Credential{AccessToken: fixtureToken(time.Now().Add(time.Hour).Unix()), AccountID: "account-1"}
	for _, config := range []types.ProviderConfig{
		{APIKey: "api-key"}, {NoAuth: true}, {Headers: map[string]string{"AUTHORIZATION": "override"}},
		{DefaultProviderOptions: map[string]any{"store": true}}, {ProviderOptionsByModel: map[string]map[string]any{"m": {"stream": false}}},
		{DefaultProviderOptions: map[string]any{"text": map[string]any{"format": "json"}}},
		{DefaultProviderOptions: map[string]any{"text": map[string]any{"verbosity": "verbose"}}},
	} {
		if _, err := New(config, credential, nil); err == nil {
			t.Fatalf("accepted conflicting config: %+v", config)
		}
	}
	p, err := New(types.ProviderConfig{}, credential, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	for _, options := range []map[string]any{{"include": nil}, {"text": map[string]any{"verbosity": "low", "format": "json"}}} {
		if _, err := p.payload(types.TextRequest{BaseRequest: types.BaseRequest{Model: "m", ProviderOptions: options}, Messages: []types.Message{types.NewUserMessage("hi")}}); err == nil {
			t.Fatalf("accepted request override: %v", options)
		}
	}
	for _, messages := range [][]types.Message{
		{types.NewSystemMessage("second"), types.NewUserMessage("hi")},
		{types.NewUserMessage("hi"), types.NewSystemMessage("late")},
	} {
		if _, err := p.payload(types.TextRequest{BaseRequest: types.BaseRequest{Model: "m"}, SystemPrompt: "first", Messages: messages}); err == nil {
			t.Fatalf("accepted conflicting instructions: %v", messages)
		}
	}
	for _, messages := range [][]types.Message{{types.NewSystemMessage("first"), types.NewSystemMessage("second"), types.NewUserMessage("hi")}, {types.NewUserMessage("hi"), types.NewSystemMessage("late")}} {
		if _, err := p.payload(types.TextRequest{BaseRequest: types.BaseRequest{Model: "m"}, Messages: messages}); err == nil {
			t.Fatalf("accepted multiple or misplaced system messages: %v", messages)
		}
	}
	plain, err := p.payload(types.TextRequest{BaseRequest: types.BaseRequest{Model: "m"}, Messages: []types.Message{types.NewUserMessage("hi")}})
	if err != nil || plain["text"] != nil {
		t.Fatalf("unexpected default verbosity: %v, %v", plain, err)
	}
	custom, err := p.payload(types.TextRequest{BaseRequest: types.BaseRequest{Model: "m", ProviderOptions: map[string]any{"text": map[string]any{"verbosity": "high"}}}, Messages: []types.Message{types.NewUserMessage("hi")}})
	if err != nil || custom["text"].(map[string]any)["verbosity"] != "high" {
		t.Fatalf("explicit verbosity: %v, %v", custom, err)
	}
}

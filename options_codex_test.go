package wormhole

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

func TestWithCodexFluentSystemPrompt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "auth.json")
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix()))) + ".signature"
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"tokens":{"access_token":%q,"account_id":"account-1"}}`, token)), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Instructions string           `json:"instructions"`
			Input        []map[string]any `json:"input"`
			Text         map[string]any   `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if payload.Instructions != "rules" || len(payload.Input) != 1 || payload.Input[0]["role"] != "user" || payload.Text["verbosity"] != "low" {
			t.Errorf("fluent Codex payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n"); err != nil {
			t.Error(err)
		}
		if _, err := fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"model\":\"codex-model\",\"status\":\"completed\",\"output\":[{\"type\":\"reasoning\",\"encrypted_content\":\"opaque\"},{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"answer\"}]}]}}\n\n"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := New(WithCodex(path, types.ProviderConfig{BaseURL: server.URL, DefaultProviderOptions: map[string]any{"text": map[string]any{"verbosity": "low"}}}))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	builder := client.Text().Using("codex").Model("codex-model").SystemPrompt("rules").Prompt("question")
	resp, err := builder.Generate(t.Context())
	if err != nil || resp.Text != "answer" {
		t.Fatalf("generate: %+v, %v", resp, err)
	}
	chunks, err := builder.Stream(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for chunk := range chunks {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		text += chunk.Text
	}
	if text != "answer" {
		t.Fatalf("stream text = %q", text)
	}
}

func TestWithCodexExplicitAuthAndNoRetries(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "auth.json")
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix()))) + ".signature"
	content := fmt.Sprintf(`{"tokens":{"access_token":%q,"account_id":"account-1"}}`, token)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	client := New(WithCodex(path, types.ProviderConfig{}))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	cfg, err := client.configuredProviderConfig("codex")
	if err != nil || cfg.MaxRetries == nil || *cfg.MaxRetries != 0 {
		t.Fatalf("retry config: %+v, %v", cfg.MaxRetries, err)
	}
	handle, err := client.ProviderWithHandle("codex")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	if handle.Name() != "codex" {
		t.Fatalf("provider = %q", handle.Name())
	}
	missing := New(WithCodex(filepath.Join(t.TempDir(), "missing"), types.ProviderConfig{}))
	t.Cleanup(func() { _ = missing.Shutdown(context.Background()) })
	if _, err := missing.ProviderWithHandle("codex"); err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("missing auth accepted or leaked token: %v", err)
	}
}

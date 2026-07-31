package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStructuredDoesNotMutateCallerMessages(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var outbound []chatRequest
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var payload chatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		outbound = append(outbound, payload)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(chatResponse{
			Model:   "test",
			Message: message{Role: "assistant", Content: `{"ok":true}`},
			Done:    true,
		})
	})

	user := types.NewUserMessage("original")
	request := types.StructuredRequest{
		BaseRequest: types.BaseRequest{Model: "test"},
		Messages:    []types.Message{user},
		Schema:      map[string]any{"type": "object"},
		Mode:        types.StructuredModeStrict,
	}
	const calls = 8
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := provider.Structured(context.Background(), request); err != nil {
				t.Errorf("Structured: %v", err)
			}
		}()
	}
	wg.Wait()

	if user.Content != "original" {
		t.Fatalf("caller message = %q, want original", user.Content)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(outbound) != calls {
		t.Fatalf("outbound requests = %d, want %d", len(outbound), calls)
	}
	for i, payload := range outbound {
		if len(payload.Messages) != 1 {
			t.Fatalf("request %d messages = %d", i, len(payload.Messages))
		}
		content, ok := payload.Messages[0].Content.(string)
		if !ok {
			t.Fatalf("request %d content type = %T", i, payload.Messages[0].Content)
		}
		if got := strings.Count(content, "Please respond with valid JSON"); got != 1 {
			t.Fatalf("request %d schema instructions = %d, content %q", i, got, content)
		}
	}
}

func TestStructuredErrorDoesNotMutateCallerMessages(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var outbound []chatRequest
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var payload chatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		outbound = append(outbound, payload)
		mu.Unlock()
		http.Error(w, "failed", http.StatusInternalServerError)
	})
	user := types.NewUserMessage("original")
	_, err := provider.Structured(context.Background(), types.StructuredRequest{
		BaseRequest: types.BaseRequest{Model: "test"},
		Messages:    []types.Message{user},
		Schema:      map[string]any{"type": "object"},
		Mode:        types.StructuredModeStrict,
	})
	if err == nil {
		t.Fatal("Structured returned nil error")
	}
	if user.Content != "original" {
		t.Fatalf("caller message = %q, want original", user.Content)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(outbound) == 0 {
		t.Fatal("Structured sent no outbound request")
	}
	for i, payload := range outbound {
		if len(payload.Messages) != 1 {
			t.Fatalf("outbound request %d messages = %d, want 1", i, len(payload.Messages))
		}
		content, ok := payload.Messages[0].Content.(string)
		if !ok || strings.Count(content, "Please respond with valid JSON") != 1 {
			t.Fatalf("outbound request %d schema instruction content = %#v", i, payload.Messages[0].Content)
		}
	}
}

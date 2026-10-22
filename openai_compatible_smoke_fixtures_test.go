package wormhole

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type smokeChatFixture struct {
	ID        string
	Model     string
	Content   string
	Reasoning string
	Usage     map[string]int
}

func (f smokeChatFixture) wire() map[string]any {
	message := map[string]string{"role": "assistant", "content": f.Content}
	if f.Reasoning != "" {
		message["reasoning_content"] = f.Reasoning
	}
	body := map[string]any{
		"id": f.ID, "created": 100,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}},
	}
	if f.Model != "" {
		body["model"] = f.Model
	}
	if f.Usage != nil {
		body["usage"] = f.Usage
	}
	return body
}

func smokeWriteChat(t *testing.T, w http.ResponseWriter, fixture smokeChatFixture) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fixture.wire()); err != nil {
		t.Errorf("encode chat fixture: %v", err)
	}
}

func smokeFixtureJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Errorf("marshal fixture JSON: %v", err)
		return ""
	}
	return string(b)
}

func smokeWriteStream(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, choice := range []map[string]any{
		{"delta": map[string]string{"content": "pong"}},
		{"delta": map[string]string{}, "finish_reason": "stop"},
	} {
		event := map[string]any{"id": "chatcmpl-smoke", "model": "local-model", "choices": []any{choice}}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", smokeFixtureJSON(t, event)); err != nil {
			t.Errorf("write stream fixture: %v", err)
			return
		}
	}
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		t.Errorf("write stream terminator: %v", err)
	}
}

// smokeDomainServer answers chat with encoded semantic content, plus stream
// and embeddings routes, recording every path it served.
func smokeDomainServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		switch r.URL.Path {
		case "/v1/chat/completions":
			if stream, err := jsonDecodeBool(r); err != nil {
				t.Errorf("decode smoke request: %v", err)
				http.Error(w, "invalid fixture request", http.StatusBadRequest)
				return
			} else if stream {
				smokeWriteStream(t, w)
				return
			}
			smokeWriteChat(t, w, smokeChatFixture{
				ID: "chatcmpl-smoke", Model: "local-model",
				Content: smokeFixtureJSON(t, map[string]string{"answer": "pong"}),
				Usage:   map[string]int{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
			})
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			body := map[string]any{
				"object": "list", "model": "local-model",
				"data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2}}},
			}
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encode embeddings fixture: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

// jsonDecodeBool reports whether the request body carries "stream": true.
func jsonDecodeBool(r *http.Request) (bool, error) {
	var body struct {
		Stream bool `json:"stream"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		return false, err
	}
	return body.Stream, nil
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateMistralEmbeddingUsesProfiledProvider(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" {
			t.Errorf("path = %q, want /embeddings", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-mistral-key" {
			t.Errorf("Authorization = %q, want exported credential as bearer token", got)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "mistral-embed" || len(body.Input) != 1 {
			t.Errorf("request = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [{"object": "embedding", "embedding": [0.25, 0.5], "index": 0}],
			"model": "mistral-embed",
			"usage": {"prompt_tokens": 4, "total_tokens": 4}
		}`))
	}))
	t.Cleanup(server.Close)

	response, err := generateMistralEmbedding(context.Background(), "test-mistral-key", server.URL)
	if err != nil {
		t.Fatalf("generate Mistral embedding: %v", err)
	}
	if len(response.Embeddings) != 1 || len(response.Embeddings[0].Embedding) != 2 {
		t.Fatalf("decoded embeddings = %#v", response.Embeddings)
	}
	if got := response.Embeddings[0].Embedding[1]; got != 0.5 {
		t.Fatalf("decoded vector[1] = %v, want 0.5", got)
	}
}

package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330GeminiReasoningAndFiles(t *testing.T) {
	t.Parallel()
	g := New("key", types.NewProviderConfig("key"))
	no, yes := false, true
	config := geminiThinkingConfig(&types.Reasoning{Effort: types.ReasoningEffortNone, Enabled: &yes, IncludeThoughts: &no})
	if config["thinkingBudget"] != 0 || config["includeThoughts"] != false {
		t.Fatalf("config = %#v", config)
	}
	request := types.TextRequest{BaseRequest: types.BaseRequest{Reasoning: &types.Reasoning{Enabled: &no, MaxTokens: 64}}}
	if _, err := g.buildTextPayload(request); err == nil {
		t.Fatal("expected contradictory controls to fail")
	}
	ref := &types.FileReferenceMedia{Provider: "gemini", Kind: "document", URI: "gs://bucket/file", MimeType: "application/pdf"}
	part, err := g.transformMedia(ref)
	if err != nil {
		t.Fatal(err)
	}
	file := part["fileData"].(map[string]any)
	if file["fileUri"] != ref.URI || file["mimeType"] != ref.MimeType {
		t.Fatalf("part = %#v", part)
	}
	ref.MimeType = ""
	if _, err := g.transformMedia(ref); err == nil {
		t.Fatal("expected required MIME validation")
	}
	ref.MimeType = "application/pdf"
	ref.Provider = "other"
	if _, err := g.transformMedia(ref); err == nil {
		t.Fatal("expected ownership validation")
	}
}
func TestV330GeminiMetadataUsageAndCitations(t *testing.T) {
	t.Parallel()
	frame := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{map[string]any{"text": "éhi"}}}, "finishReason": "STOP",
			"groundingMetadata": map[string]any{
				"groundingChunks":   []any{map[string]any{"web": map[string]any{"uri": "https://example.com", "title": "Example"}}},
				"groundingSupports": []any{map[string]any{"segment": map[string]any{"startIndex": 0, "endIndex": 2}, "groundingChunkIndices": []int{0}}},
			},
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 10, "cachedContentTokenCount": 4, "candidatesTokenCount": 3, "thoughtsTokenCount": 2, "totalTokenCount": 15, "extraDetail": 7},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "upstream-id")
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			body, _ := json.Marshal(frame)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
				t.Errorf("write streamed fixture: %v", err)
			}
			return
		}
		_ = json.NewEncoder(w).Encode(frame)
	}))
	t.Cleanup(server.Close)
	g := New("key", types.ProviderConfig{BaseURL: server.URL})
	request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "gemini-test"}, Messages: []types.Message{types.NewUserMessage("hello")}}
	response, err := g.Text(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.ResponseMetadata.RequestID != "upstream-id" {
		t.Fatalf("metadata=%#v", response.ResponseMetadata)
	}
	if response.Usage.CacheReadTokens != 4 || response.Usage.TotalTokens != 15 || response.Usage.CompletionTokens != 5 {
		t.Fatalf("usage=%#v", response.Usage)
	}
	if response.Metadata["provider_usage"].(map[string]any)["extraDetail"] != float64(7) {
		t.Fatal("lost original usage")
	}
	if len(response.Sources) != 1 || len(response.Citations) != 1 || response.Citations[0].OutputRange.End != 2 {
		t.Fatalf("citations=%#v", response.Citations)
	}
	stream, err := g.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	first, terminal := true, false
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		if first && chunk.ResponseMetadata == nil {
			t.Fatal("missing first metadata")
		}
		first = false
		if chunk.IsDone() {
			terminal = true
			if len(chunk.Citations) != 1 || chunk.ResponseMetadata == nil {
				t.Fatalf("terminal=%#v", chunk)
			}
		}
	}
	if !terminal {
		t.Fatal("missing terminal")
	}
}
func TestV330GeminiMalformedOptionalCitation(t *testing.T) {
	t.Parallel()
	body := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "ok"}}}, "citationMetadata": "broken"}}}
	encoded, _ := json.Marshal(body)
	var raw geminiTextResponse
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	response, err := New("key", types.ProviderConfig{}).transformTextResponse(&raw)
	if err != nil || response.Text != "ok" || len(response.Warnings) != 1 {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	if _, ok := response.Metadata["citationMetadata"]; !ok {
		t.Fatal("raw malformed annotation not retained")
	}
}
func TestV330GeminiStreamRepeatedCitations(t *testing.T) {
	t.Parallel()
	frame := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "hello"}}}, "groundingMetadata": map[string]any{"groundingChunks": []any{map[string]any{"web": map[string]any{"uri": "https://example.com"}}}, "groundingSupports": []any{map[string]any{"segment": map[string]any{"startIndex": 0, "endIndex": 5}, "groundingChunkIndices": []int{0}}}}}}}
	first, _ := json.Marshal(frame)
	candidate := frame["candidates"].([]any)[0].(map[string]any)
	candidate["content"] = map[string]any{"parts": []any{}}
	candidate["finishReason"] = "STOP"
	last, _ := json.Marshal(frame)
	body := io.NopCloser(strings.NewReader(fmt.Sprintf("data: %s\n\ndata: %s\n\n", first, last)))
	var terminal types.TextChunk
	for chunk := range New("key", types.ProviderConfig{}).handleStream(context.Background(), body) {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		if chunk.IsDone() {
			terminal = chunk
		}
	}
	if len(terminal.Sources) != 1 || len(terminal.Citations) != 1 {
		t.Fatalf("terminal=%#v", terminal)
	}
}

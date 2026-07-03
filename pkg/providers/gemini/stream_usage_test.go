package gemini

import (
	"context"
	"io"
	"strings"
	"testing"
)

// usageMetadata is top-level on a Gemini stream response; the stream parser
// must surface it as chunk Usage (the non-streaming path already does).
// Regression guard for dropped streamed usage.
func TestParseStreamEventSurfacesTopLevelUsage(t *testing.T) {
	t.Parallel()
	g := &Gemini{}

	data := `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":3,"totalTokenCount":15}}`

	chunks, done, err := g.parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent: %v", err)
	}
	if done {
		t.Fatal("unexpected done=true")
	}

	found := false
	for _, c := range chunks {
		if c.Usage != nil {
			found = true
			if c.Usage.PromptTokens != 12 {
				t.Fatalf("PromptTokens = %d, want 12", c.Usage.PromptTokens)
			}
			if c.Usage.CompletionTokens != 3 {
				t.Fatalf("CompletionTokens = %d, want 3", c.Usage.CompletionTokens)
			}
			if c.Usage.TotalTokens != 15 {
				t.Fatalf("TotalTokens = %d, want 15", c.Usage.TotalTokens)
			}
		}
	}
	if !found {
		t.Fatal("expected a chunk carrying Usage, found none")
	}
}

func TestConvertUsageCacheReadTokens(t *testing.T) {
	t.Parallel()
	u := convertUsage(&usageMetadata{
		PromptTokenCount:        100,
		CandidatesTokenCount:    20,
		TotalTokenCount:         120,
		CachedContentTokenCount: 75,
	})
	if u == nil {
		t.Fatal("expected non-nil usage")
	}
	if u.CacheReadTokens != 75 {
		t.Fatalf("CacheReadTokens = %d, want 75", u.CacheReadTokens)
	}
	if u.PromptTokens != 100 || u.CompletionTokens != 20 || u.TotalTokens != 120 {
		t.Fatalf("token counts wrong: %+v", u)
	}
}

func TestTransformEmbeddingsResponseUsesRequestModel(t *testing.T) {
	t.Parallel()
	g := &Gemini{}

	response := &geminiEmbeddingsResponse{}
	result := g.transformEmbeddingsResponse(response, "req-z")
	if result.Model != "req-z" {
		t.Fatalf("Model = %q, want %q", result.Model, "req-z")
	}
}

func TestHandleStreamReportsPrematureEOF(t *testing.T) {
	t.Parallel()
	g := &Gemini{}

	stream := io.NopCloser(strings.NewReader(`data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"},"finishReason":""}]}` + "\n\n"))

	var chunks []string
	var gotErr error
	for chunk := range g.handleStream(context.Background(), stream) {
		if chunk.Error != nil {
			gotErr = chunk.Error
			continue
		}
		if chunk.Text != "" {
			chunks = append(chunks, chunk.Text)
		}
	}
	if len(chunks) != 1 || chunks[0] != "partial" {
		t.Fatalf("chunks = %#v, want partial text before error", chunks)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "ended before terminal event") {
		t.Fatalf("error = %v, want premature EOF error", gotErr)
	}
}

package gemini

import "testing"

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

package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderOptionsMergedIntoTextPayload(t *testing.T) {
	t.Parallel()
	provider := New("key", types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"safetySettings": []any{"default"}, "cachedContent": "default"}).
		WithProviderOptionsForModel("gemini-test", map[string]any{"cachedContent": "model"}))

	payload, err := provider.buildTextPayload(types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "gemini-test",
			ProviderOptions: map[string]any{"cachedContent": "request"},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildTextPayload returned error: %v", err)
	}

	if payload["cachedContent"] != "request" {
		t.Fatalf("cachedContent = %v, want request", payload["cachedContent"])
	}
	if payload["safetySettings"] == nil {
		t.Fatal("safetySettings option missing")
	}
}

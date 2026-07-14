package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
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

func TestTypedReasoningMergedIntoGenerationConfig(t *testing.T) {
	t.Parallel()
	include := true
	provider := New("key", types.NewProviderConfig("key"))

	payload, err := provider.buildTextPayload(types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-test",
			Reasoning: &types.Reasoning{
				MaxTokens: 512,
				Enabled:   &include,
			},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildTextPayload returned error: %v", err)
	}

	generationConfig := payload["generationConfig"].(map[string]any)
	thinkingConfig := generationConfig["thinkingConfig"].(map[string]any)
	if thinkingConfig["thinkingBudget"] != 512 || thinkingConfig["includeThoughts"] != true {
		t.Fatalf("thinkingConfig = %#v", thinkingConfig)
	}
}

func TestProviderOptionsGenerationConfigMergesIntoTextPayload(t *testing.T) {
	t.Parallel()
	maxTokens := 128
	provider := New("key", types.NewProviderConfig("key"))

	payload, err := provider.buildTextPayload(types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gemini-test",
			MaxTokens: &maxTokens,
			ProviderOptions: map[string]any{
				"generationConfig": map[string]any{
					"responseMimeType": "application/json",
				},
			},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildTextPayload returned error: %v", err)
	}

	generationConfig := payload["generationConfig"].(map[string]any)
	if generationConfig["maxOutputTokens"] != 128 {
		t.Fatalf("maxOutputTokens = %v, want 128", generationConfig["maxOutputTokens"])
	}
	if generationConfig["responseMimeType"] != "application/json" {
		t.Fatalf("responseMimeType = %v, want application/json", generationConfig["responseMimeType"])
	}
}

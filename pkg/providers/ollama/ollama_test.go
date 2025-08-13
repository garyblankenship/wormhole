package ollama

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestNew_RequiresBaseURL(t *testing.T) {
	config := types.ProviderConfig{}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when BaseURL is not provided")
		}
	}()

	New(config)
}

func TestNew_CustomConfig(t *testing.T) {
	customURL := "http://custom.ollama.host:11434"
	config := types.ProviderConfig{
		BaseURL: customURL,
	}
	provider := New(config)

	if provider == nil {
		t.Fatal("Expected provider to be created")
	}

	if provider.GetBaseURL() != customURL {
		t.Errorf("Expected base URL to be %s, got %s", customURL, provider.GetBaseURL())
	}
}

func TestBuildChatPayload(t *testing.T) {
	provider := New(types.ProviderConfig{
		BaseURL: "http://localhost:11434",
	})

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:       "llama2",
			Temperature: func() *float32 { t := float32(0.7); return &t }(),
		},
		Messages: []types.Message{
			&types.UserMessage{Content: "Hello, world!"},
		},
		SystemPrompt: "You are a helpful assistant.",
	}

	payload := provider.buildChatPayload(request)

	if payload.Model != "llama2" {
		t.Errorf("Expected model to be 'llama2', got %s", payload.Model)
	}

	if len(payload.Messages) != 2 { // system + user
		t.Errorf("Expected 2 messages, got %d", len(payload.Messages))
	}

	if payload.Messages[0].Role != "system" {
		t.Errorf("Expected first message role to be 'system', got %s", payload.Messages[0].Role)
	}

	if payload.Messages[1].Role != "user" {
		t.Errorf("Expected second message role to be 'user', got %s", payload.Messages[1].Role)
	}

	if payload.Options == nil {
		t.Fatal("Expected options to be set")
	}

	if payload.Options.Temperature == nil || *payload.Options.Temperature != 0.7 {
		t.Errorf("Expected temperature to be 0.7, got %v", payload.Options.Temperature)
	}
}

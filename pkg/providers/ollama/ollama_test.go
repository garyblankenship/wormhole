package ollama

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestNew_DefaultConfig(t *testing.T) {
	config := types.ProviderConfig{}
	provider := New(config)

	if provider == nil {
		t.Fatal("Expected provider to be created")
	}

	if provider.Name() != "ollama" {
		t.Errorf("Expected provider name to be 'ollama', got %s", provider.Name())
	}

	if provider.GetBaseURL() != defaultBaseURL {
		t.Errorf("Expected base URL to be %s, got %s", defaultBaseURL, provider.GetBaseURL())
	}
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
	provider := New(types.ProviderConfig{})

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

package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderOptionsMergedIntoMessagePayload(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key").
		WithDefaultProviderOptions(map[string]any{"metadata": map[string]any{"source": "default"}, "thinking": false}).
		WithProviderOptionsForModel("claude-test", map[string]any{"thinking": true}))

	payload := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:           "claude-test",
			ProviderOptions: map[string]any{"metadata": map[string]any{"source": "request"}},
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})

	if payload["thinking"] != true {
		t.Fatalf("thinking = %v, want true", payload["thinking"])
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["source"] != "request" {
		t.Fatalf("metadata source = %v, want request", metadata["source"])
	}
}

package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestBuildMessagePayloadDefaultMaxTokens(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key"))

	payload, err := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}

	if got, want := payload["max_tokens"], config.GetDefaultAnthropicMaxTokens(); got != want {
		t.Fatalf("max_tokens = %v, want config default %v", got, want)
	}
}

func TestBuildMessagePayloadExplicitMaxTokensWins(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key"))
	mt := 1000

	payload, err := provider.buildMessagePayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test", MaxTokens: &mt},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("buildMessagePayload() error = %v", err)
	}

	if payload["max_tokens"] != 1000 {
		t.Fatalf("max_tokens = %v, want 1000 (explicit request value)", payload["max_tokens"])
	}
}

func TestGetDefaultAnthropicMaxTokensEnvOverride(t *testing.T) {
	t.Setenv("WORMHOLE_ANTHROPIC_MAX_TOKENS", "8192")
	if got := config.GetDefaultAnthropicMaxTokens(); got != 8192 {
		t.Fatalf("GetDefaultAnthropicMaxTokens() = %d, want 8192", got)
	}
}

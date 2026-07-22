package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestOpenAIPayloadsIgnoreAnthropicToolCacheControl(t *testing.T) {
	t.Parallel()

	provider := New(types.NewProviderConfig("key"))
	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-test"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
		Tools: []types.Tool{{
			Name:         "lookup",
			Description:  "Look something up",
			InputSchema:  map[string]any{"type": "object"},
			CacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: types.CacheTTL1Hour},
		}},
	}

	for name, payload := range map[string]map[string]any{
		"chat completions": provider.buildChatPayload(request),
		"responses":        provider.buildResponsesPayload(request),
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			require.NotContains(t, strings.ToLower(string(encoded)), "cache_control")
		})
	}
}

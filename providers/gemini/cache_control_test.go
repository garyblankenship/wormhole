package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestGeminiPayloadIgnoresAnthropicToolCacheControl(t *testing.T) {
	t.Parallel()

	provider := New("key", types.NewProviderConfig("key"))
	payload, err := provider.buildTextPayload(types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gemini-test"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
		Tools: []types.Tool{{
			Name:         "lookup",
			Description:  "Look something up",
			InputSchema:  map[string]any{"type": "object"},
			CacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: types.CacheTTL1Hour},
		}},
	})
	require.NoError(t, err)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(encoded)), "cache_control")
}

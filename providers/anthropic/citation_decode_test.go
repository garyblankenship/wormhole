package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330AnthropicMalformedOptionalCitations(t *testing.T) {
	t.Parallel()
	for _, malformed := range []any{map[string]any{}, "invalid", []any{42, nil}} {
		t.Run("optional", func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "r", "model": "claude", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": "ok", "citations": malformed}}})
			}))
			t.Cleanup(server.Close)
			p := New(types.ProviderConfig{APIKey: "fixture", BaseURL: server.URL})
			response, err := p.Text(t.Context(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude"}, Messages: []types.Message{types.NewUserMessage("hi")}})
			require.NoError(t, err)
			require.Equal(t, "ok", response.Text)
			require.NotEmpty(t, response.Warnings)
			require.Equal(t, types.WarningMalformedCitation, response.Warnings[0].Code)
			require.Empty(t, response.Citations)
			require.NotEmpty(t, response.Sources)
			require.NotNil(t, response.Sources[0].Raw)
		})
	}
}

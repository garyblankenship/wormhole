package gemini

import (
	"encoding/json"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestThoughtSignatureRoundTrip exercises capture (response -> types.ToolCall),
// replay (assistant message -> Gemini wire part), and the absence guarantee
// (no signature -> no key emitted).
func TestThoughtSignatureRoundTrip(t *testing.T) {
	t.Parallel()
	const sig = "abc123"

	g := New("test-key", types.ProviderConfig{})

	t.Run("capture and replay non-streaming", func(t *testing.T) {
		t.Parallel()

		raw := `{"candidates":[{"content":{"role":"model","parts":[` +
			`{"functionCall":{"name":"get_weather","args":{"city":"NYC"}},` +
			`"thoughtSignature":"` + sig + `"}` +
			`]}}]}`

		var resp geminiTextResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))

		result, err := g.transformTextResponse(&resp)
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		assert.Equal(t, sig, result.ToolCalls[0].ThoughtSignature) // CAPTURE

		// Replay: feed the parsed ToolCall back through the outgoing transform.
		assistantMsg := &types.AssistantMessage{ToolCalls: result.ToolCalls}
		parts, err := g.transformMessageToParts(assistantMsg)
		require.NoError(t, err)
		require.Len(t, parts, 1)
		assert.Equal(t, sig, parts[0]["thoughtSignature"]) // REPLAY
	})

	t.Run("absent signature emits no key", func(t *testing.T) {
		t.Parallel()

		raw := `{"candidates":[{"content":{"role":"model","parts":[` +
			`{"functionCall":{"name":"ping","args":{}}}` +
			`]}}]}`

		var resp geminiTextResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))

		result, err := g.transformTextResponse(&resp)
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		assert.Empty(t, result.ToolCalls[0].ThoughtSignature) // no leak

		assistantMsg := &types.AssistantMessage{ToolCalls: result.ToolCalls}
		parts, err := g.transformMessageToParts(assistantMsg)
		require.NoError(t, err)
		require.Len(t, parts, 1)
		_, hasKey := parts[0]["thoughtSignature"]
		assert.False(t, hasKey) // key must be absent, not present-and-empty
	})

	t.Run("streaming path captures signature", func(t *testing.T) {
		t.Parallel()

		raw := `{"content":{"role":"model","parts":[` +
			`{"functionCall":{"name":"get_weather","args":{"city":"NYC"}},` +
			`"thoughtSignature":"` + sig + `"}` +
			`]}}`

		var cand candidate
		require.NoError(t, json.Unmarshal([]byte(raw), &cand))

		chunks := g.processStreamCandidate(cand)

		var got string
		for _, ch := range chunks {
			if ch.ToolCall != nil {
				got = ch.ToolCall.ThoughtSignature
			}
		}
		assert.Equal(t, sig, got) // STREAMING CAPTURE
	})
}

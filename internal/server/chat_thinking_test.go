package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
)

func TestProxyChatThinkingForwarding(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "stream"}[stream], func(t *testing.T) {
			t.Parallel()
			payloads := make(chan map[string]any, 1)
			upstream := kwargsChatUpstream(t, payloads, stream)
			proxy := newKwargsTestProxy(t, kwargsOpenAIOption(upstream.URL), wormhole.WithDiscovery(false))
			body, err := json.Marshal(map[string]any{
				"model": "openai/stub", "messages": []map[string]string{{"role": "user", "content": "hello"}},
				"thinking": map[string]any{"type": "disabled"}, "reasoning_effort": "low",
				"chat_template_kwargs": map[string]any{"enable_thinking": false}, "stream": stream,
				"ignored_extra": "never forwarded",
			})
			require.NoError(t, err)
			response := performRequest(proxy, http.MethodPost, "/v1/chat/completions", string(body))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			select {
			case payload := <-payloads:
				require.Equal(t, map[string]any{"type": "disabled"}, payload["thinking"])
				require.Equal(t, map[string]any{"enable_thinking": false}, payload["chat_template_kwargs"])
				require.Equal(t, "low", payload["reasoning_effort"])
				require.NotContains(t, payload, "ignored_extra")
			default:
				t.Fatal("request never reached provider")
			}
		})
	}
}

func TestProxyChatThinkingRejectsNative(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"anthropic", "gemini", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			proxy := newKwargsTestProxy(t, wormhole.WithDiscovery(false))
			body, err := json.Marshal(map[string]any{"model": provider + "/stub", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "thinking": map[string]any{"type": "disabled"}})
			require.NoError(t, err)
			response := performRequest(proxy, http.MethodPost, "/v1/chat/completions", string(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), "thinking is unsupported")
		})
	}
}

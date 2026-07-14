package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestChatCompletionsPath(t *testing.T) {
	t.Parallel()

	newRecordingServer := func(t *testing.T, recorded *string) *httptest.Server {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*recorded = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-path",
				Created: 100,
				Model:   "m",
				Choices: []struct {
					Index        int     `json:"index"`
					Message      message `json:"message"`
					FinishReason string  `json:"finish_reason"`
				}{{
					Message:      message{Role: "assistant", Content: "hello"},
					FinishReason: "stop",
				}},
				Usage: usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
			}))
		}))
		t.Cleanup(server.Close)
		return server
	}

	t.Run("default path", func(t *testing.T) {
		t.Parallel()
		var recorded string
		srv := newRecordingServer(t, &recorded)

		provider := New(types.ProviderConfig{BaseURL: srv.URL, APIKey: "k"})
		_, err := provider.Text(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "m"},
			Messages:    []types.Message{types.NewUserMessage("hi")},
		})
		require.NoError(t, err)
		assert.Equal(t, "/chat/completions", recorded)
	})

	t.Run("override path", func(t *testing.T) {
		t.Parallel()
		var recorded string
		srv := newRecordingServer(t, &recorded)

		provider := New(types.ProviderConfig{BaseURL: srv.URL, APIKey: "k", ChatPath: "/v4/chat/completions"})
		_, err := provider.Text(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "m"},
			Messages:    []types.Message{types.NewUserMessage("hi")},
		})
		require.NoError(t, err)
		assert.Equal(t, "/v4/chat/completions", recorded)
	})
}

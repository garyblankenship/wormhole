package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFieldPopulated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/embeddings":
			require.NoError(t, json.NewEncoder(w).Encode(embeddingsResponse{
				Object: "list",
				Model:  "text-embedding-3-small",
				Data: []struct {
					Object    string    `json:"object"`
					Index     int       `json:"index"`
					Embedding []float32 `json:"embedding"`
				}{{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3}}},
				Usage: usage{PromptTokens: 1, TotalTokens: 1},
			}))
		default:
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-field",
				Created: 100,
				Model:   "gpt-4o-mini",
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
		}
	}))
	t.Cleanup(server.Close)

	provider := New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})

	textResp, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	assert.Equal(t, "openai", textResp.Provider)

	embedResp, err := provider.Embeddings(context.Background(), types.EmbeddingsRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hi"},
	})
	require.NoError(t, err)
	assert.Equal(t, "openai", embedResp.Provider)
}

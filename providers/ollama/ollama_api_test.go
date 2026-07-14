package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func newOllamaTestProvider(t *testing.T, handler http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider, err := New(types.ProviderConfig{BaseURL: server.URL})
	require.NoError(t, err)
	return provider, server
}

func TestProviderText(t *testing.T) {
	t.Parallel()
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)

		var req chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "llama3", req.Model)
		assert.False(t, req.Stream)
		require.Len(t, req.Messages, 1)
		assert.Equal(t, roleUser, req.Messages[0].Role)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(chatResponse{
			Model:           "llama3",
			CreatedAt:       time.Unix(100, 0),
			Message:         message{Role: roleAssistant, Content: "hello"},
			Done:            true,
			PromptEvalCount: 3,
			EvalCount:       4,
		}))
	})

	resp, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	assert.Equal(t, "llama3", resp.Model)
	assert.Equal(t, "hello", resp.Text)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 7, resp.Usage.TotalTokens)
}

func TestProviderStructured(t *testing.T) {
	t.Parallel()
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "json", req.Format)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(chatResponse{
			Model:     "llama3",
			CreatedAt: time.Unix(100, 0),
			Message:   message{Role: roleAssistant, Content: `{"name":"Ada"}`},
			Done:      true,
		}))
	})

	resp, err := provider.Structured(context.Background(), types.StructuredRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("json")},
		Schema:      map[string]any{"type": "object"},
		Mode:        types.StructuredModeJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "Ada"}, resp.Data)
}

func TestProviderStructuredInvalidJSON(t *testing.T) {
	t.Parallel()
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(chatResponse{
			Model:   "llama3",
			Message: message{Role: roleAssistant, Content: `not-json`},
			Done:    true,
		}))
	})

	_, err := provider.Structured(context.Background(), types.StructuredRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("json")},
		Mode:        types.StructuredModeJSON,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured response")
}

func TestProviderEmbeddingsSequentialAndConcurrent(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embeddings", r.URL.Path)
		var req embeddingsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "embed", req.Model)
		n := requests.Add(1)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(embeddingsResponse{
			Embedding: []float64{float64(n), float64(len(req.Prompt))},
		}))
	})

	sequential, err := provider.Embeddings(context.Background(), types.EmbeddingsRequest{
		Model: "embed",
		Input: []string{"a", "bb"},
	})
	require.NoError(t, err)
	require.Len(t, sequential.Embeddings, 2)
	assert.Equal(t, 0, sequential.Embeddings[0].Index)
	assert.Equal(t, 1, sequential.Embeddings[1].Index)

	concurrent, err := provider.Embeddings(context.Background(), types.EmbeddingsRequest{
		Model: "embed",
		Input: []string{"a", "bb", "ccc", "dddd"},
	})
	require.NoError(t, err)
	require.Len(t, concurrent.Embeddings, 4)
	for i := range concurrent.Embeddings {
		assert.Equal(t, i, concurrent.Embeddings[i].Index)
		assert.Len(t, concurrent.Embeddings[i].Embedding, 2)
	}
}

func TestProviderEmbeddingsValidationAndErrors(t *testing.T) {
	t.Parallel()
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	})

	_, err := provider.Embeddings(context.Background(), types.EmbeddingsRequest{Model: "embed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input")

	_, err = provider.Embeddings(context.Background(), types.EmbeddingsRequest{
		Model: "embed",
		Input: []string{"a"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get embedding")
}

func TestProviderModelManagement(t *testing.T) {
	t.Parallel()
	provider, _ := newOllamaTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			assert.Equal(t, http.MethodGet, r.Method)
			require.NoError(t, json.NewEncoder(w).Encode(modelsResponse{
				Models: []modelInfo{{Name: "llama3:latest", Model: "llama3", Size: 123}},
			}))
		case "/api/pull", "/api/show", "/api/delete":
			assert.Contains(t, []string{http.MethodPost, http.MethodDelete}, r.Method)
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "llama3", req["name"])
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"status": "ok"}))
		default:
			http.NotFound(w, r)
		}
	})

	models, err := provider.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	assert.Equal(t, "llama3:latest", models.Models[0].Name)

	require.NoError(t, provider.PullModel(context.Background(), "llama3"))

	info, err := provider.ShowModel(context.Background(), "llama3")
	require.NoError(t, err)
	assert.Equal(t, "ok", info["status"])

	require.NoError(t, provider.DeleteModel(context.Background(), "llama3"))
}

func TestProviderUnsupportedOperations(t *testing.T) {
	t.Parallel()
	provider, err := New(types.ProviderConfig{BaseURL: "http://localhost:11434"})
	require.NoError(t, err)

	_, err = provider.Images(context.Background(), types.ImagesRequest{})
	require.Error(t, err)

	_, err = provider.Audio(context.Background(), types.AudioRequest{Type: types.AudioRequestTypeTTS})
	require.Error(t, err)

	_, err = provider.Audio(context.Background(), types.AudioRequest{Type: types.AudioRequestTypeSTT})
	require.Error(t, err)

	_, err = provider.SpeechToText(context.Background(), types.SpeechToTextRequest{})
	require.Error(t, err)

	_, err = provider.TextToSpeech(context.Background(), types.TextToSpeechRequest{})
	require.Error(t, err)

	_, err = provider.GenerateImage(context.Background(), types.ImageRequest{})
	require.Error(t, err)
}

func TestTransformHelpers(t *testing.T) {
	t.Parallel()
	provider, err := New(types.ProviderConfig{BaseURL: "http://localhost:11434"})
	require.NoError(t, err)

	topK := 5
	repeatPenalty := float32(1.2)
	presencePenalty := float32(0.1)
	frequencyPenalty := float32(0.2)
	maxTokens := 12
	seed := 99
	topP := float32(0.8)
	req := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "llama3",
			MaxTokens: &maxTokens,
			TopP:      &topP,
			Stop:      []string{"stop"},
			Seed:      &seed,
			ProviderOptions: map[string]any{
				"top_k":             topK,
				"repeat_penalty":    repeatPenalty,
				"presence_penalty":  presencePenalty,
				"frequency_penalty": frequencyPenalty,
			},
		},
		Messages: []types.Message{
			types.NewSystemMessage("system"),
			types.NewAssistantMessage("assistant"),
			types.NewToolResultMessage("tool-id", "tool"),
			types.BaseMessage{
				Role: types.RoleUser,
				Content: []types.MessagePart{
					{Type: "text", Text: "look"},
					{Type: "image", Data: "data:image/png;base64,abc123"},
				},
			},
		},
	}

	payload := provider.buildChatPayload(req)
	require.NotNil(t, payload.Options)
	assert.Equal(t, topK, *payload.Options.TopK)
	assert.Equal(t, repeatPenalty, *payload.Options.RepeatPenalty)
	assert.Equal(t, presencePenalty, *payload.Options.PresencePenalty)
	assert.Equal(t, frequencyPenalty, *payload.Options.FrequencyPenalty)
	assert.Equal(t, maxTokens, *payload.Options.NumPredict)
	assert.Equal(t, seed, *payload.Options.Seed)
	assert.Equal(t, []string{"stop"}, payload.Options.Stop)

	require.Len(t, payload.Messages, 3)
	assert.Equal(t, roleSystem, payload.Messages[0].Role)
	assert.Equal(t, roleAssistant, payload.Messages[1].Role)
	assert.Equal(t, roleUser, payload.Messages[2].Role)
	assert.Equal(t, "look", payload.Messages[2].Content)
	assert.Equal(t, []string{"abc123"}, payload.Messages[2].Images)

	assert.Equal(t, roleUser, provider.mapRole(types.Role("unknown")))
	assert.Equal(t, []string{"raw"}, convertMultimodalPartsForTest([]types.MessagePart{{Type: "image", Data: "raw"}}))
	assert.Equal(t, "not-string", extractImageData("not-string"))
	assert.Equal(t, "123", extractImageData(123))
	assert.Nil(t, provider.convertUsage(nil))
}

func convertMultimodalPartsForTest(parts []types.MessagePart) []string {
	_, images := convertMultimodalParts(parts)
	return images
}

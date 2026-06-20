package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOpenAITestProvider(t *testing.T, handler http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL}), server
}

func TestProviderTextAndEmptyResponse(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)
			assert.Equal(t, "Bearer test-key", r.Header.Get(types.HeaderAuthorization))

			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "gpt-4o-mini", req["model"])

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-1",
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
				Usage: usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
			}))
		})

		resp, err := provider.Text(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
			Messages:    []types.Message{types.NewUserMessage("hi")},
		})
		require.NoError(t, err)
		assert.Equal(t, "hello", resp.Text)
		assert.Equal(t, 5, resp.Usage.TotalTokens)
	})

	t.Run("empty response errors", func(t *testing.T) {
		t.Parallel()
		provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-empty",
				Created: 100,
				Model:   "gpt-4o-mini",
				Choices: []struct {
					Index        int     `json:"index"`
					Message      message `json:"message"`
					FinishReason string  `json:"finish_reason"`
				}{{Message: message{Role: "assistant"}}},
			}))
		})

		_, err := provider.Text(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
			Messages:    []types.Message{types.NewUserMessage("hi")},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty response")
	})
}

func TestProviderStructuredJSONAndTools(t *testing.T) {
	t.Parallel()
	t.Run("json mode", func(t *testing.T) {
		t.Parallel()
		provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, map[string]any{"type": "json_object"}, req["response_format"])

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-json",
				Created: 100,
				Model:   "gpt-4o-mini",
				Choices: []struct {
					Index        int     `json:"index"`
					Message      message `json:"message"`
					FinishReason string  `json:"finish_reason"`
				}{{Message: message{Role: "assistant", Content: `{"name":"Ada"}`}, FinishReason: "stop"}},
			}))
		})

		resp, err := provider.Structured(context.Background(), types.StructuredRequest{
			BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
			Messages:    []types.Message{types.NewUserMessage("json")},
			Mode:        types.StructuredModeJSON,
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"name": "Ada"}, resp.Data)
	})

	t.Run("tool mode", func(t *testing.T) {
		t.Parallel()
		provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.NotEmpty(t, req["tools"])
			assert.NotEmpty(t, req["tool_choice"])

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
				ID:      "chatcmpl-tool",
				Created: 100,
				Model:   "gpt-4o-mini",
				Choices: []struct {
					Index        int     `json:"index"`
					Message      message `json:"message"`
					FinishReason string  `json:"finish_reason"`
				}{{
					Message: message{Role: "assistant", ToolCalls: []toolCall{{
						ID:   "call-1",
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: "structured_output", Arguments: `{"name":"Ada"}`},
					}}},
					FinishReason: "tool_calls",
				}},
			}))
		})

		resp, err := provider.Structured(context.Background(), types.StructuredRequest{
			BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
			Messages:    []types.Message{types.NewUserMessage("tool")},
			Schema:      map[string]any{"type": "object"},
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"name": "Ada"}, resp.Data)
	})
}

func TestStructuredStrictEmitsJSONSchema(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		responseFormat, ok := req["response_format"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "json_schema", responseFormat["type"])

		jsonSchema, ok := responseFormat["json_schema"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "person", jsonSchema["name"])
		assert.Equal(t, true, jsonSchema["strict"])
		schemaData, ok := jsonSchema["schema"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, schema, schemaData)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      "chatcmpl-strict-json-schema",
			Created: 100,
			Model:   "gpt-4o-mini",
			Choices: []struct {
				Index        int     `json:"index"`
				Message      message `json:"message"`
				FinishReason string  `json:"finish_reason"`
			}{{Message: message{Role: "assistant", Content: `{"name":"Ada"}`}, FinishReason: "stop"}},
		}))
	})

	resp, err := provider.Structured(context.Background(), types.StructuredRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("strict")},
		Mode:        types.StructuredModeStrict,
		Schema:      schema,
		SchemaName:  "person",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "Ada"}, resp.Data)
}

func TestProviderEmbeddingsImagesAndAudio(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/embeddings":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, float64(128), req["dimensions"])
			require.NoError(t, json.NewEncoder(w).Encode(embeddingsResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Index     int       `json:"index"`
					Embedding []float32 `json:"embedding"`
				}{{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2}}},
				Model: "text-embedding-3-small",
				Usage: usage{PromptTokens: 1, TotalTokens: 1},
			}))
		case "/images/generations":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "gpt-image-1", req["model"])
			assert.Equal(t, "draw", req["prompt"])
			assert.Equal(t, "1024x1024", req["size"])
			assert.Equal(t, "high", req["quality"])
			assert.Equal(t, "natural", req["style"])
			assert.Equal(t, float64(1), req["n"])
			assert.Equal(t, "url", req["response_format"])
			require.NoError(t, json.NewEncoder(w).Encode(imageResponse{
				Created: 100,
				Data: []struct {
					URL     string `json:"url,omitempty"`
					B64JSON string `json:"b64_json,omitempty"`
				}{{URL: "https://example.test/image.png", B64JSON: "abc"}},
			}))
		case "/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("audio-bytes"))
		case "/audio/transcriptions":
			assert.Contains(t, r.Header.Get(types.HeaderContentType), "multipart/form-data")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"text":     "transcribed",
				"language": "en",
			}))
		default:
			http.NotFound(w, r)
		}
	})

	dimensions := 128
	embeddings, err := provider.Embeddings(context.Background(), types.EmbeddingsRequest{
		Model:      "text-embedding-3-small",
		Input:      []string{"hello"},
		Dimensions: &dimensions,
	})
	require.NoError(t, err)
	require.Len(t, embeddings.Embeddings, 1)
	assert.InEpsilonSlice(t, []float64{0.1, 0.2}, embeddings.Embeddings[0].Embedding, 0.000001)

	images, err := provider.Images(context.Background(), types.ImagesRequest{
		Model:          "gpt-image-1",
		Prompt:         "draw",
		Size:           "1024x1024",
		Quality:        "high",
		Style:          "natural",
		N:              1,
		ResponseFormat: "url",
	})
	require.NoError(t, err)
	require.Len(t, images.Images, 1)
	assert.Equal(t, "https://example.test/image.png", images.Images[0].URL)

	generatedImage, err := provider.GenerateImage(context.Background(), types.ImageRequest{
		Model:          "gpt-image-1",
		Prompt:         "draw",
		Size:           "1024x1024",
		Quality:        "high",
		Style:          "natural",
		N:              1,
		ResponseFormat: "url",
	})
	require.NoError(t, err)
	require.Len(t, generatedImage.Images, 1)
	assert.Equal(t, "https://example.test/image.png", generatedImage.Images[0].URL)

	tts, err := provider.Audio(context.Background(), types.AudioRequest{
		Type:           types.AudioRequestTypeTTS,
		Model:          "gpt-4o-mini-tts",
		Input:          "hello",
		Voice:          "alloy",
		Speed:          1.1,
		ResponseFormat: "mp3",
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("audio-bytes"), tts.Audio)
	assert.Equal(t, "mp3", tts.Format)

	stt, err := provider.Audio(context.Background(), types.AudioRequest{
		Type:        types.AudioRequestTypeSTT,
		Model:       "whisper-1",
		Input:       []byte("audio"),
		Language:    "en",
		Prompt:      "prompt",
		Temperature: func() *float32 { v := float32(0.2); return &v }(),
	})
	require.NoError(t, err)
	assert.Equal(t, "transcribed", stt.Text)
}

func TestProviderSpeechToTextInvalidInputReturnsError(t *testing.T) {
	t.Parallel()
	provider := New(types.ProviderConfig{APIKey: "test-key", BaseURL: "http://127.0.0.1"})
	_, err := provider.Audio(context.Background(), types.AudioRequest{
		Type:  types.AudioRequestTypeSTT,
		Model: "whisper-1",
		Input: "not bytes",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "speech-to-text input must be non-empty []byte audio")
}

func TestProviderStream(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chunk-1\",\"model\":\"gpt-4o-mini\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chunk-1\",\"model\":\"gpt-4o-mini\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})

	stream, err := provider.Stream(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	var chunks []types.TextChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}
	require.NotEmpty(t, chunks)
	assert.Equal(t, "hi", chunks[0].Content())
}

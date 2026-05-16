package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSupportedCapabilities(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	capabilities := provider.SupportedCapabilities()

	require.Len(t, capabilities, 8)
	assert.Contains(t, capabilities, types.CapabilityText)
	assert.Contains(t, capabilities, types.CapabilityChat)
	assert.Contains(t, capabilities, types.CapabilityStructured)
	assert.Contains(t, capabilities, types.CapabilityEmbeddings)
	assert.Contains(t, capabilities, types.CapabilityAudio)
	assert.Contains(t, capabilities, types.CapabilityImages)
	assert.Contains(t, capabilities, types.CapabilityStream)
	assert.Contains(t, capabilities, types.CapabilityFunctions)
}

func TestImageCapabilityHasGenerateImageImplementation(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/images/generations", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(imageResponse{
			Created: 100,
			Data: []struct {
				URL     string `json:"url,omitempty"`
				B64JSON string `json:"b64_json,omitempty"`
			}{{URL: "https://example.test/generated.png"}},
		}))
	})

	require.Contains(t, provider.SupportedCapabilities(), types.CapabilityImages)
	resp, err := provider.GenerateImage(context.Background(), types.ImageRequest{
		Model:  "gpt-image-1",
		Prompt: "draw",
	})
	require.NoError(t, err)
	require.Len(t, resp.Images, 1)
	assert.Equal(t, "https://example.test/generated.png", resp.Images[0].URL)
}

func TestGetMaxTokensParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   types.ProviderConfig
		model    string
		expected string
	}{
		{
			name:     "default model uses max_tokens",
			config:   types.ProviderConfig{APIKey: "test-key"},
			model:    "gpt-4o-mini",
			expected: "max_tokens",
		},
		{
			name:     "GPT-5 model uses max_completion_tokens",
			config:   types.ProviderConfig{APIKey: "test-key"},
			model:    "openai/gpt-5-mini",
			expected: "max_completion_tokens",
		},
		{
			name: "configured parameter overrides model default",
			config: types.ProviderConfig{
				APIKey: "test-key",
				Params: map[string]any{"max_tokens_param": "custom_tokens"},
			},
			model:    "gpt-5",
			expected: "custom_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := New(tt.config)
			assert.Equal(t, tt.expected, provider.getMaxTokensParam(tt.model))
		})
	}
}

func TestTransformToolChoiceOpenAIFallbacks(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})

	assert.Equal(t, "auto", provider.transformToolChoice(nil))
	assert.Equal(t, "required", provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAny}))
	assert.Equal(t, "none", provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeNone}))

	specific := provider.transformToolChoice(&types.ToolChoice{
		Type:     types.ToolChoiceTypeSpecific,
		ToolName: "lookup",
	})
	assert.Equal(t, map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "lookup",
		},
	}, specific)
}

func TestParseStreamChunkFallback(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	provider.streamingTransformer = nil

	_, err := provider.parseStreamChunk([]byte("{"))
	require.Error(t, err)

	chunk, err := provider.parseStreamChunk([]byte(`{"id":"empty","model":"gpt-4o-mini","choices":[]}`))
	require.NoError(t, err)
	assert.Nil(t, chunk)

	chunk, err = provider.parseStreamChunk([]byte(`{
		"id":"chunk-1",
		"model":"gpt-4o-mini",
		"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "chunk-1", chunk.ID)
	assert.Equal(t, "gpt-4o-mini", chunk.Model)
	assert.Equal(t, "hello", chunk.Text)
	require.NotNil(t, chunk.Delta)
	assert.Equal(t, "hello", chunk.Delta.Content)
	require.NotNil(t, chunk.FinishReason)
	assert.Equal(t, types.FinishReasonStop, *chunk.FinishReason)
	require.NotNil(t, chunk.Usage)
	assert.Equal(t, 3, chunk.Usage.TotalTokens)

	chunk, err = provider.parseStreamChunk([]byte(`{
		"id":"chunk-2",
		"model":"gpt-4o-mini",
		"choices":[{"delta":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"ada\"}"}}]}}]
	}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.Len(t, chunk.ToolCalls, 1)
	assert.Equal(t, "call-1", chunk.ToolCalls[0].ID)
	assert.Equal(t, "lookup", chunk.ToolCalls[0].Name)
	assert.Equal(t, map[string]any{"q": "ada"}, chunk.ToolCalls[0].Arguments)
}

func TestHandleSpeechToTextStatusError(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/audio/transcriptions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get(types.HeaderAuthorization))
		assert.Contains(t, r.Header.Get(types.HeaderContentType), "multipart/form-data")

		w.WriteHeader(http.StatusTooManyRequests)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"error": "slow down"}))
	})

	_, err := provider.handleSpeechToText(context.Background(), types.AudioRequest{
		Type:  types.AudioRequestTypeSTT,
		Model: "whisper-1",
		Input: []byte("audio"),
	})
	require.Error(t, err)
	assert.True(t, types.IsRateLimitError(err))
}

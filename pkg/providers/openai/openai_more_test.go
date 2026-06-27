package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

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

	tests := []struct {
		name     string
		config   types.ProviderConfig
		wantPath string
	}{
		{
			name:     "default OpenAI path",
			config:   types.ProviderConfig{APIKey: "test-key"},
			wantPath: "/images/generations",
		},
		{
			name: "configured image path",
			config: types.ProviderConfig{
				APIKey:    "test-key",
				ImagePath: "/images",
			},
			wantPath: "/images",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider, _ := newOpenAITestProviderWithConfig(t, tt.config, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, tt.wantPath, r.URL.Path)

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
		})
	}
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
		{
			name: "request policy rule selects parameter",
			config: types.ProviderConfig{
				APIKey: "test-key",
				RequestPolicy: types.ProviderRequestPolicy{
					MaxTokensParam: "max_tokens",
					MaxTokensParamRules: []types.MaxTokensParamRule{{
						ModelContains: "reasoning-model",
						Param:         "max_completion_tokens",
					}},
				},
			},
			model:    "vendor/reasoning-model-large",
			expected: "max_completion_tokens",
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

func TestRequestPolicyCapsMaxTokens(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{
		APIKey: "test-key",
		RequestPolicy: types.ProviderRequestPolicy{
			MaxTokensCap: 64,
		},
	})
	maxTokens := 128
	payload := provider.buildChatPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gpt-4o-mini",
			MaxTokens: &maxTokens,
		},
		Messages: []types.Message{types.NewUserMessage("hi")},
	})
	assert.Equal(t, 64, payload["max_tokens"])
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

func TestHandleSpeechToTextValidatesAudioInput(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	_, err := provider.handleSpeechToText(context.Background(), types.AudioRequest{
		Type:  types.AudioRequestTypeSTT,
		Model: "whisper-1",
		Input: "not-bytes",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "speech-to-text input must be non-empty []byte audio")
}

func TestHandleSpeechToTextLimitsResponseBody(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.CopyN(w, zeroReader{}, maxSpeechToTextJSONBytes+1)
	})

	_, err := provider.handleSpeechToText(context.Background(), types.AudioRequest{
		Type:  types.AudioRequestTypeSTT,
		Model: "whisper-1",
		Input: []byte("audio"),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "response body exceeded")
}

func TestHandleTextToSpeechLimitsAudioBody(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.CopyN(w, zeroReader{}, maxTextToSpeechAudioBytes+1)
	})

	_, err := provider.handleTextToSpeech(context.Background(), types.AudioRequest{
		Type:  types.AudioRequestTypeTTS,
		Model: "gpt-4o-mini-tts",
		Input: "hello",
	})

	require.Error(t, err)
	var whErr *types.WormholeError
	require.True(t, errors.As(err, &whErr))
	require.NotNil(t, whErr.Cause)
	assert.Contains(t, whErr.Cause.Error(), "response body exceeded")
}

func TestReadLimitedAllowsExactLimit(t *testing.T) {
	t.Parallel()

	data, err := readLimited(strings.NewReader("1234"), 4)

	require.NoError(t, err)
	assert.Equal(t, []byte("1234"), data)
}

func TestStreamPayloadSetsIncludeUsage(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-4o-mini"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	}

	payload := provider.buildChatPayload(&request)
	payload["stream"] = true
	payload["stream_options"] = map[string]any{"include_usage": true}

	opts, ok := payload["stream_options"].(map[string]any)
	require.True(t, ok, "stream_options must be a map")
	assert.Equal(t, true, opts["include_usage"])
}

func TestStampProviderCtxGuard(t *testing.T) {
	t.Parallel()

	provider := New(types.ProviderConfig{APIKey: "test-key"})
	finish := types.FinishReasonStop

	t.Run("stamps terminal chunk and preserves order", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		in := make(chan types.TextChunk, 2)
		in <- types.TextChunk{Text: "alpha"}
		in <- types.TextChunk{Text: "omega", FinishReason: &finish}
		close(in)

		out := provider.stampProvider(ctx, in)

		got := make([]types.TextChunk, 0, 2)
		for chunk := range out {
			got = append(got, chunk)
		}

		require.Len(t, got, 2)
		assert.Equal(t, "alpha", got[0].Text)
		assert.Equal(t, "omega", got[1].Text)
		assert.True(t, got[1].IsDone())
		assert.Equal(t, "openai", got[1].Provider)
		assert.Empty(t, got[0].Provider)
	})

	t.Run("returns when context is canceled before reading", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		in := make(chan types.TextChunk, 2)
		in <- types.TextChunk{Text: "first"}
		in <- types.TextChunk{Text: "second", FinishReason: &finish}
		close(in)

		out := provider.stampProvider(ctx, in)

		select {
		case _, ok := <-out:
			assert.False(t, ok)
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("stampProvider did not return after context cancellation")
		}
	})
}

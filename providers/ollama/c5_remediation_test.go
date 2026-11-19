package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestOllama_StreamFieldExplicitlySerialized(t *testing.T) {
	t.Parallel()

	var capturedChatBody map[string]any
	var capturedPullBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedChatBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"llama3","message":{"role":"assistant","content":"hi"},"done":true}`))
		case "/api/pull":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedPullBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := New(types.ProviderConfig{BaseURL: server.URL})
	require.NoError(t, err)

	_, err = provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("hello")},
	})
	require.NoError(t, err)

	streamVal, exists := capturedChatBody["stream"]
	require.True(t, exists, `/api/chat payload must explicitly include "stream": false`)
	assert.Equal(t, false, streamVal)

	require.NoError(t, provider.PullModel(context.Background(), "llama3"))
	pullStreamVal, exists := capturedPullBody["stream"]
	require.True(t, exists, `/api/pull payload must explicitly include "stream": false`)
	assert.Equal(t, false, pullStreamVal)
}

func TestOllama_RejectsToolsAndToolChoiceInTextAndStream(t *testing.T) {
	t.Parallel()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider, err := New(types.ProviderConfig{BaseURL: server.URL})
	require.NoError(t, err)

	reqWithTools := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("hello")},
		Tools:       []types.Tool{{Name: "get_weather"}},
	}
	reqWithToolChoice := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "llama3"},
		Messages:    []types.Message{types.NewUserMessage("hello")},
		ToolChoice:  &types.ToolChoice{Type: types.ToolChoiceTypeAuto},
	}

	for name, req := range map[string]types.TextRequest{
		"tools":       reqWithTools,
		"tool_choice": reqWithToolChoice,
	} {
		_, err := provider.Text(context.Background(), req)
		require.Error(t, err, "Text should reject %s", name)
		we, ok := types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeValidation, we.Code)

		_, err = provider.Stream(context.Background(), req)
		require.Error(t, err, "Stream should reject %s", name)
		we, ok = types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeValidation, we.Code)
	}

	assert.False(t, called, "HTTP request must not be sent when tools/tool_choice are rejected")
}

func TestOllama_BuildOptionsMergesConfigProviderOptions(t *testing.T) {
	t.Parallel()

	cfg := types.ProviderConfig{BaseURL: "http://localhost:11434"}.
		WithDefaultProviderOptions(map[string]any{
			"top_k":          20,
			"repeat_penalty": float32(1.1),
		}).
		WithProviderOptionsForModel("llama3", map[string]any{
			"top_k":            40,
			"presence_penalty": float32(0.5),
		})

	provider, err := New(cfg)
	require.NoError(t, err)

	opts := provider.buildOptions(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "llama3",
			ProviderOptions: map[string]any{
				"frequency_penalty": float32(0.25),
			},
		},
	})
	require.NotNil(t, opts)
	require.NotNil(t, opts.TopK)
	assert.Equal(t, 40, *opts.TopK, "per-model option should override default")
	require.NotNil(t, opts.RepeatPenalty)
	assert.Equal(t, float32(1.1), *opts.RepeatPenalty, "default option should be inherited")
	require.NotNil(t, opts.PresencePenalty)
	assert.Equal(t, float32(0.5), *opts.PresencePenalty, "per-model option should be applied")
	require.NotNil(t, opts.FrequencyPenalty)
	assert.Equal(t, float32(0.25), *opts.FrequencyPenalty, "per-request option should be applied")
}

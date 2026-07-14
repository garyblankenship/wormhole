package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

func TestProxyResponseFormatGatedProviders(t *testing.T) {
	t.Parallel()

	tests := []string{"anthropic/claude-sonnet-4-5", "gemini/gemini-2.5-pro", "ollama/llama3.2"}
	for _, model := range tests {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
				ID:           "chat-1",
				Model:        "gpt-test",
				Text:         "ok",
				FinishReason: types.FinishReasonStop,
			})
			p := newTestProxy(mock)

			rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_object"}}`)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var out ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "unsupported_response_format", out.Error.Code)
		})
	}
}

func TestProxyResponseFormatPassthroughOpenAI(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		Text:         "ok",
		FinishReason: types.FinishReasonStop,
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"hi"}],
		"response_format":{"type":"json_object"}
	}`)
	require.Equal(t, http.StatusOK, rec.Code)
	// MockProvider does not record the outbound request, so this test confirms the OpenAI
	// path accepts response_format; separate mapper tests cover payload threading.
}

func TestProxyResponseFormatInvalidValue(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		Text:         "ok",
		FinishReason: types.FinishReasonStop,
	}))

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"response_format":"notanobject"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "invalid_request_error", out.Error.Code)
}

func TestProxyResponseFormatDefaultProviderGate(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		Text:         "ok",
		FinishReason: types.FinishReasonStop,
	})
	p := New(Config{
		DefaultProvider: "anthropic",
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openai", wmtest.MockProviderFactory(mock)),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"some-model","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_object"}}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "unsupported_response_format", out.Error.Code)
}

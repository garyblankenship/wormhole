package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	wmtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProxy(mock *wmtest.MockProvider, opts ...wormhole.Option) *proxy {
	baseOpts := make([]wormhole.Option, 0, 4+len(opts))
	baseOpts = append(baseOpts,
		wormhole.WithCustomProvider("openai", wmtest.MockProviderFactory(mock)),
		wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithDiscovery(false),
	)
	baseOpts = append(baseOpts, opts...)

	return New(Config{
		WormholeOpts: baseOpts,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func performRequest(p *proxy, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.server.Handler.ServeHTTP(rec, req)
	return rec
}

func TestParseModelRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		model        string
		wantProvider string
		wantModel    string
	}{
		{name: "no prefix", model: "gpt-5.2", wantModel: "gpt-5.2"},
		{name: "known provider prefix", model: "anthropic/claude-sonnet-4-5", wantProvider: "anthropic", wantModel: "claude-sonnet-4-5"},
		{name: "unknown slash prefix remains model", model: "custom/model", wantModel: "custom/model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotProvider, gotModel := parseModelRoute(tt.model)
			assert.Equal(t, tt.wantProvider, gotProvider)
			assert.Equal(t, tt.wantModel, gotModel)
		})
	}
}

func TestProxyHealthAndAuth(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai")
	p := newTestProxy(mock)
	p.apiKey = "secret"

	health := performRequest(p, http.MethodGet, "/health", "")
	require.Equal(t, http.StatusOK, health.Code)

	blocked := performRequest(p, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusUnauthorized, blocked.Code)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	p.server.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestProxyChatCompletions(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		Text:         "hello from mock",
		FinishReason: types.FinishReasonStop,
		Usage:        &types.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[{"role":"system","content":"be direct"},{"role":"user","content":"hello"}],
		"temperature":0.2,
		"max_tokens":16,
		"top_p":0.9,
		"stop":["END"]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var out ChatCompletionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "chat.completion", out.Object)
	assert.Equal(t, "gpt-test", out.Model)
	assert.Equal(t, "hello from mock", out.Choices[0].Message.Content)
	require.NotNil(t, out.Usage)
	assert.Equal(t, 7, out.Usage.TotalTokens)
}

func TestProxyChatValidationAndUpstreamErrors(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai").WithError("provider unavailable"))

	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "invalid json", body: `{`, code: http.StatusBadRequest},
		{name: "missing model", body: `{"messages":[{"role":"user","content":"hello"}]}`, code: http.StatusBadRequest},
		{name: "missing messages", body: `{"model":"gpt-test"}`, code: http.StatusBadRequest},
		{name: "upstream error", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`, code: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions", tt.body)
			assert.Equal(t, tt.code, rec.Code)
			var out ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out.Error.Message)
		})
	}
}

func TestProxyRejectsOversizedChatBody(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	body := `{"model":"gpt-test","messages":[{"role":"user","content":"` +
		strings.Repeat("x", maxProxyRequestBodyBytes) + `"}]}`

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", body)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "invalid_json", out.Error.Code)
	assert.Contains(t, out.Error.Message, "request body too large")
}

func TestProxyChatStreaming(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithStreamChunks(wmtest.StreamChunksFrom("hello", " world"))
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, "hello")
	assert.Contains(t, body, "world")
	assert.Contains(t, body, "data: [DONE]")
}

func TestProxyRejectsMultipleJSONValues(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions",
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]} {}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "invalid_json", out.Error.Code)
	assert.Contains(t, out.Error.Message, "single JSON value")
}

func TestProxyEmbeddings(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithEmbeddings([]types.Embedding{
		{Index: 0, Embedding: []float64{0.1, 0.2}},
		{Index: 1, Embedding: []float64{0.3, 0.4}},
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/embeddings", `{
		"model":"openai/text-embedding-test",
		"input":["one","two"]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var out EmbeddingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "text-embedding-test", out.Model)
	require.Len(t, out.Data, 2)
	assert.Equal(t, []float64{0.1, 0.2}, out.Data[0].Embedding)
}

func TestProxyEmbeddingsAcceptsSingleStringInput(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithEmbeddings([]types.Embedding{
		{Index: 0, Embedding: []float64{0.1, 0.2}},
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/embeddings", `{
		"model":"openai/text-embedding-test",
		"input":"one"
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var out EmbeddingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Data, 1)
	assert.Equal(t, "text-embedding-test", out.Model)
}

func TestProxyEmbeddingsValidationAndUpstreamErrors(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai").WithError("embedding provider unavailable"))

	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "invalid json", body: `{`, code: http.StatusBadRequest},
		{name: "missing model", body: `{"input":["hello"]}`, code: http.StatusBadRequest},
		{name: "missing input", body: `{"model":"text-embedding-test"}`, code: http.StatusBadRequest},
		{name: "upstream error", body: `{"model":"text-embedding-test","input":["hello"]}`, code: http.StatusBadGateway},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := performRequest(p, http.MethodPost, "/v1/embeddings", tt.body)
			assert.Equal(t, tt.code, rec.Code)
			var out ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out.Error.Message)
		})
	}
}

func TestProxyModelsEmptyWhenDiscoveryDisabled(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodGet, "/v1/models", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var out ModelListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "list", out.Object)
	assert.Empty(t, out.Data)
}

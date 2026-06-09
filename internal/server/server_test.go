package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func newCapturingTestProxy(provider *capturingTextProvider) *proxy {
	return New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
				return provider, nil
			}),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

type capturingTextProvider struct {
	*wmtest.MockProvider
	mu       sync.Mutex
	requests []types.TextRequest
}

func newCapturingTextProvider(name string) *capturingTextProvider {
	return &capturingTextProvider{
		MockProvider: wmtest.NewMockProvider(name).WithTextResponse(types.TextResponse{
			ID:           "chat-1",
			Model:        "gpt-test",
			Text:         "ok",
			FinishReason: types.FinishReasonStop,
		}),
	}
}

func (p *capturingTextProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	return p.MockProvider.Text(ctx, request)
}

func (p *capturingTextProvider) lastRequest() types.TextRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		return types.TextRequest{}
	}
	return p.requests[len(p.requests)-1]
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
		{name: "ollama-openai profile prefix routes", model: "ollama-openai/llama3", wantProvider: "ollama-openai", wantModel: "llama3"},
		{name: "groq profile prefix routes", model: "groq/llama3", wantProvider: "groq", wantModel: "llama3"},
		{name: "openrouter profile prefix routes", model: "openrouter/anthropic/claude-sonnet-4-5", wantProvider: "openrouter", wantModel: "anthropic/claude-sonnet-4-5"},
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

// TestKnownProviderSetMatchesProfiles verifies that the router's known provider
// set is derived from provider_profiles.json, not a hardcoded map.
func TestKnownProviderSetMatchesProfiles(t *testing.T) {
	names := wormhole.KnownProviderNames()
	require.NotEmpty(t, names, "expected provider profiles")

	// Every profile name must be routable
	for _, name := range names {
		provider, model := parseModelRoute(name + "/test-model")
		assert.Equal(t, name, provider, "profile %q should be a routable prefix", name)
		assert.Equal(t, "test-model", model, "model should be %q for prefix %q", "test-model", name)
	}

	// Unknown prefixes must not route
	provider, model := parseModelRoute("notaprovider/foo")
	assert.Empty(t, provider, "unknown prefix should not route")
	assert.Equal(t, "notaprovider/foo", model, "unknown prefix should passthrough as full model")
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

func TestProxyChatContentStringStillBuildsTextMessage(t *testing.T) {
	t.Parallel()

	capturingProvider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(capturingProvider)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	last := capturingProvider.lastRequest()
	require.Len(t, last.Messages, 1)
	user, ok := last.Messages[0].(*types.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "hello", user.Content)
	assert.Empty(t, user.Media)
}

func TestProxyChatContentPartsDataURLRoutesToUserMedia(t *testing.T) {
	t.Parallel()

	capturingProvider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(capturingProvider)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"describe this"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}
			]
		}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	last := capturingProvider.lastRequest()
	require.Len(t, last.Messages, 1)
	user, ok := last.Messages[0].(*types.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "describe this", user.Content)
	require.Len(t, user.Media, 1)
	image, ok := user.Media[0].(*types.ImageMedia)
	require.True(t, ok)
	assert.Equal(t, "image/png", image.MimeType)
	assert.Equal(t, "aW1hZ2U=", image.Base64Data)
	assert.Empty(t, image.Data)
}

func TestProxyChatRejectsMalformedImageDataURL(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"describe this"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,not valid base64"}}
			]
		}]
	}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "invalid_json", out.Error.Code)
	assert.Contains(t, out.Error.Message, "malformed image data URL")
}

func TestProxyChatRejectsNonUserImageParts(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"messages":[{
			"role":"assistant",
			"content":[
				{"type":"text","text":"here"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}
			]
		}]
	}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "unsupported_content_part", out.Error.Code)
	assert.Contains(t, out.Error.Message, "only supported on user messages")
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

func TestMergeProviderNames(t *testing.T) {
	t.Parallel()

	got := mergeProviderNames(
		[]string{"openai", "groq"},
		[]string{"openai", "anthropic", "mistral"},
	)

	assert.Equal(t, []string{"openai", "groq", "anthropic", "mistral"}, got)
}

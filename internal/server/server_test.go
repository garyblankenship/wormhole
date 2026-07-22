package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

type shutdownErrorCloser struct {
	err error
}

func (c shutdownErrorCloser) Close() error { return c.err }

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

func TestProxyShutdownPropagatesWormholeError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("wormhole cleanup failed")
	p := New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithDiscovery(false),
			func(cfg *wormhole.Config) {
				cfg.Closers = append(cfg.Closers, shutdownErrorCloser{err: wantErr})
			},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	require.ErrorIs(t, p.Shutdown(context.Background()), wantErr)
}

type capturingTextProvider struct {
	*wmtest.MockProvider
	mu       sync.Mutex
	requests []types.TextRequest
}

type capturingRerankProvider struct {
	*wmtest.MockProvider
	request  types.RerankRequest
	response *types.RerankResponse
	err      error
}

func (p *capturingRerankProvider) Rerank(_ context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
	p.request = request
	return p.response, p.err
}

func newRerankTestProxy(provider types.Provider) *proxy {
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

func TestProxyLogsExcludeRawProviderErrors(t *testing.T) {
	t.Parallel()

	const secret = "upstream-body-with-api-key-sk-secret"
	var logs bytes.Buffer
	p := newTestProxy(wmtest.NewMockProvider("openai").WithError(secret))
	p.logger = slog.New(slog.NewTextHandler(&logs, nil))

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions",
		`{"model":"gpt-test","messages":[{"role":"user","content":"private prompt"}]}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.NotContains(t, logs.String(), secret)
	assert.NotContains(t, logs.String(), "private prompt")
	assert.Contains(t, logs.String(), "error_type")
}

func TestParseModelRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		model           string
		defaultProvider string
		configured      []string
		wantProvider    string
		wantModel       string
	}{
		{name: "no prefix", model: "gpt-5.2", wantModel: "gpt-5.2"},
		{name: "known provider prefix", model: "anthropic/claude-sonnet-4-5", wantProvider: "anthropic", wantModel: "claude-sonnet-4-5"},
		{name: "unknown slash prefix remains model", model: "custom/model", wantModel: "custom/model"},
		{name: "ollama-openai profile prefix routes", model: "ollama-openai/llama3", wantProvider: "ollama-openai", wantModel: "llama3"},
		{name: "groq profile prefix routes", model: "groq/llama3", wantProvider: "groq", wantModel: "llama3"},
		{name: "openrouter profile prefix routes", model: "openrouter/anthropic/claude-sonnet-4-5", wantProvider: "openrouter", wantModel: "anthropic/claude-sonnet-4-5"},
		{name: "openrouter default does not hijack unregistered org-prefixed model", model: "openai/gpt-4o", defaultProvider: "openrouter", configured: []string{"openrouter"}, wantModel: "openai/gpt-4o"},
		{name: "openrouter default does not hijack anthropic org-prefixed model", model: "anthropic/claude-3.5-sonnet", defaultProvider: "openrouter", wantModel: "anthropic/claude-3.5-sonnet"},
		{name: "openrouter default still routes configured local provider", model: "openai/gpt-4o", defaultProvider: "openrouter", configured: []string{"openrouter", "openai"}, wantProvider: "openai", wantModel: "gpt-4o"},
		{name: "explicit openrouter prefix still routes when already default", model: "openrouter/anthropic/claude-sonnet-4-5", defaultProvider: "openrouter", wantProvider: "openrouter", wantModel: "anthropic/claude-sonnet-4-5"},
		{name: "effective openrouter default from sole configured provider does not hijack org-prefixed model", model: "openai/gpt-4o", defaultProvider: effectiveDefaultProvider("", []string{"openrouter"}), configured: []string{"openrouter"}, wantModel: "openai/gpt-4o"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotProvider, gotModel := parseModelRoute(tt.model, tt.defaultProvider, tt.configured)
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
		provider, model := parseModelRoute(name+"/test-model", "", nil)
		assert.Equal(t, name, provider, "profile %q should be a routable prefix", name)
		assert.Equal(t, "test-model", model, "model should be %q for prefix %q", "test-model", name)
	}

	// Unknown prefixes must not route
	provider, model := parseModelRoute("notaprovider/foo", "", nil)
	assert.Empty(t, provider, "unknown prefix should not route")
	assert.Equal(t, "notaprovider/foo", model, "unknown prefix should passthrough as full model")
}

func TestProxyOpenRouterRoutingPrefersConfiguredLocalProviders(t *testing.T) {
	t.Parallel()

	openRouterProvider := newCapturingTextProvider("openrouter")
	openAIProvider := newCapturingTextProvider("openai")
	p := New(Config{
		DefaultProvider: "openrouter",
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openrouter", func(types.ProviderConfig) (types.Provider, error) {
				return openRouterProvider, nil
			}),
			wormhole.WithProviderConfig("openrouter", types.ProviderConfig{}),
			wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
				return openAIProvider, nil
			}),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[{"role":"user","content":"hi"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, len(openAIProvider.requests))
	assert.Empty(t, openRouterProvider.requests)
	assert.Equal(t, "gpt-test", openAIProvider.lastRequest().Model)
}

func TestProxyOpenRouterRoutingUsesSoleConfiguredProviderAsEffectiveDefault(t *testing.T) {
	t.Parallel()

	openRouterProvider := newCapturingTextProvider("openrouter")
	p := New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openrouter", func(types.ProviderConfig) (types.Provider, error) {
				return openRouterProvider, nil
			}),
			wormhole.WithProviderConfig("openrouter", types.ProviderConfig{}),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-4o",
		"messages":[{"role":"user","content":"hi"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, len(openRouterProvider.requests))
	assert.Equal(t, "openai/gpt-4o", openRouterProvider.lastRequest().Model)
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

func TestProxyChatSamplingControlsReachSDKRequest(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[{"role":"user","content":"hello"}],
		"frequency_penalty":0.4,
		"presence_penalty":-0.3,
		"seed":42,
		"n":1,
		"parallel_tool_calls":false
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	request := provider.lastRequest()
	require.NotNil(t, request.FrequencyPenalty)
	require.NotNil(t, request.PresencePenalty)
	require.NotNil(t, request.Seed)
	require.NotNil(t, request.ParallelToolCalls)
	assert.InDelta(t, 0.4, *request.FrequencyPenalty, 0.00001)
	assert.InDelta(t, -0.3, *request.PresencePenalty, 0.00001)
	assert.Equal(t, 42, *request.Seed)
	assert.False(t, *request.ParallelToolCalls)
}

func TestProxyRejectsUnsupportedSamplingControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "multiple choices", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"n":2}`},
		{name: "zero choices", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"n":0}`},
		{name: "frequency range", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"frequency_penalty":2.1}`},
		{name: "presence range", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"presence_penalty":-2.1}`},
		{name: "anthropic seed", body: `{"model":"anthropic/claude-test","messages":[{"role":"user","content":"hi"}],"seed":1}`},
		{name: "gemini parallel", body: `{"model":"gemini/gemini-test","messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":false}`},
		{name: "ollama parallel", body: `{"model":"ollama/llama-test","messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newTestProxy(wmtest.NewMockProvider("openai"))
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions", tt.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			var out ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "unsupported_parameter", out.Error.Code)
		})
	}
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
		{name: "unsupported message role", body: `{"model":"gpt-test","messages":[{"role":"systme","content":"hello"}]}`, code: http.StatusBadRequest},
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

func TestProxyRejectsUnsupportedMessageRole(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"messages":[{"role":"systme","content":"hello"}]
	}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "unsupported_message_role", out.Error.Code)
	assert.Contains(t, out.Error.Message, `"systme"`)
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

func TestProxyChatStreamingRefusal(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{{Refusal: "I cannot help with that."}})
	p := newTestProxy(mock)
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"stream":true,
		"messages":[{"role":"user","content":"unsafe request"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"refusal":"I cannot help with that."`)
	assert.NotContains(t, body, `"content":"I cannot help with that."`)
}

func TestProxyChatStreamingErrorBeforeCommitReturnsHTTPError(t *testing.T) {
	t.Parallel()

	upstreamErr := types.NewWormholeError(types.ErrorCodeRateLimit, "quota bucket team-alpha exhausted", true).
		WithStatusCode(http.StatusTooManyRequests).
		WithRetryAfter(1500 * time.Millisecond)
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{{Error: upstreamErr}})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEqual(t, "text/event-stream", rec.Header().Get("Content-Type"))

	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "rate_limit_error", out.Error.Type)
	assert.Equal(t, "upstream_error", out.Error.Code)
	assert.Equal(t, "upstream rate limit exceeded", out.Error.Message)
	assert.NotContains(t, out.Error.Message, "team-alpha")
	assert.Equal(t, "2", rec.Header().Get("Retry-After"))
}

func TestProxyChatStreamingErrorAfterCommitEmitsSSEError(t *testing.T) {
	t.Parallel()

	upstreamErr := types.NewWormholeError(types.ErrorCodeRateLimit, "quota bucket team-alpha exhausted", true).
		WithStatusCode(http.StatusTooManyRequests).
		WithRetryAfter(1500 * time.Millisecond)
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{
		{Text: "partial"},
		{Error: upstreamErr},
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"gpt-test",
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	body := rec.Body.String()
	assert.Contains(t, body, "partial")
	assert.Contains(t, body, `"type":"rate_limit_error"`)
	assert.Contains(t, body, `"code":"upstream_error"`)
	assert.Contains(t, body, `"message":"upstream rate limit exceeded"`)
	assert.NotContains(t, body, "team-alpha")
	assert.NotContains(t, body, "data: [DONE]")
	assert.Empty(t, rec.Header().Get("Retry-After"), "post-commit SSE failures cannot add HTTP headers")
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
	assert.Equal(t, []any{0.1, 0.2}, out.Data[0].Embedding)
}

func TestProxyEmbeddingsBase64Encoding(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithEmbeddings([]types.Embedding{{
		Index:     0,
		Embedding: []float64{1, -2.5},
	}})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/embeddings", `{
		"model":"openai/text-embedding-test",
		"input":"one",
		"encoding_format":"base64"
	}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var out EmbeddingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Data, 1)
	assert.Equal(t, "AACAPwAAIMA=", out.Data[0].Embedding)

	bad := performRequest(p, http.MethodPost, "/v1/embeddings", `{
		"model":"openai/text-embedding-test",
		"input":"one",
		"encoding_format":"hex"
	}`)
	require.Equal(t, http.StatusBadRequest, bad.Code)
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

func TestProxyRerank(t *testing.T) {
	t.Parallel()

	provider := &capturingRerankProvider{
		MockProvider: wmtest.NewMockProvider("openai"),
		response: &types.RerankResponse{
			ID:    "rerank-1",
			Model: "rerank-test",
			Results: []types.RerankResult{
				{Index: 1, RelevanceScore: 0.95, Document: "second"},
			},
			Usage: &types.Usage{TotalTokens: 12},
		},
	}
	p := newRerankTestProxy(provider)

	rec := performRequest(p, http.MethodPost, "/v1/rerank", `{
		"model":"openai/rerank-test",
		"query":"best document",
		"documents":["first","second"],
		"top_n":1
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var out RerankResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "rerank-test", provider.request.Model)
	assert.Equal(t, "best document", provider.request.Query)
	assert.Equal(t, []string{"first", "second"}, provider.request.Documents)
	require.NotNil(t, provider.request.TopN)
	assert.Equal(t, 1, *provider.request.TopN)
	assert.Equal(t, "rerank-1", out.ID)
	assert.Equal(t, "rerank-test", out.Model)
	require.Len(t, out.Results, 1)
	assert.Equal(t, "second", out.Results[0].Document.Text)
	require.NotNil(t, out.Usage)
	assert.Equal(t, 12, out.Usage.TotalTokens)
}

func TestProxyRerankValidationAndUpstreamErrors(t *testing.T) {
	t.Parallel()

	provider := &capturingRerankProvider{
		MockProvider: wmtest.NewMockProvider("openai"),
		err:          errors.New("rerank provider unavailable"),
	}
	p := newRerankTestProxy(provider)

	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "invalid json", body: `{`, code: http.StatusBadRequest},
		{name: "missing model", body: `{"query":"q","documents":["one"]}`, code: http.StatusBadRequest},
		{name: "missing query", body: `{"model":"rerank-test","documents":["one"]}`, code: http.StatusBadRequest},
		{name: "missing documents", body: `{"model":"rerank-test","query":"q"}`, code: http.StatusBadRequest},
		{name: "upstream error", body: `{"model":"rerank-test","query":"q","documents":["one"]}`, code: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := performRequest(p, http.MethodPost, "/v1/rerank", tt.body)
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

func TestProxyChatMapsToolRoleToToolResultMessage(t *testing.T) {
	t.Parallel()

	capturingProvider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(capturingProvider)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"openai/gpt-test",
		"messages":[
			{"role":"user","content":"weather?"},
			{"role":"assistant","content":"checking"},
			{"role":"tool","tool_call_id":"call_abc","content":"sunny"}
		]
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	last := capturingProvider.lastRequest()
	require.Len(t, last.Messages, 3)

	tr, ok := last.Messages[2].(*types.ToolResultMessage)
	require.True(t, ok, "role:tool must map to ToolResultMessage, not UserMessage")
	assert.Equal(t, "call_abc", tr.ToolCallID)
	assert.Equal(t, "sunny", tr.Content)
}

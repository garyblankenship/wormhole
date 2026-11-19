package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

// C1-01: custom configured providers route with prefix stripped.
func TestC1_01_CustomConfiguredProviderPrefixRouting(t *testing.T) {
	t.Parallel()

	provider, model := parseModelRoute("acme-custom/my-model", "", []string{"acme-custom"})
	assert.Equal(t, "acme-custom", provider)
	assert.Equal(t, "my-model", model)

	provider, model = parseModelRoute("acme-custom/my-model", "openrouter", []string{"openrouter", "acme-custom"})
	assert.Equal(t, "acme-custom", provider)
	assert.Equal(t, "my-model", model)

	// Unconfigured custom prefix still passes through unchanged.
	provider, model = parseModelRoute("acme-custom/my-model", "", []string{"openai"})
	assert.Empty(t, provider)
	assert.Equal(t, "acme-custom/my-model", model)

	// End-to-end proxy chat completion strips custom configured provider prefix.
	customProvider := newCapturingTextProvider("acme-custom")
	p := New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("acme-custom", func(types.ProviderConfig) (types.Provider, error) {
				return customProvider, nil
			}),
			wormhole.WithProviderConfig("acme-custom", types.ProviderConfig{}),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{
		"model":"acme-custom/my-model",
		"messages":[{"role":"user","content":"hello"}]
	}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "my-model", customProvider.lastRequest().Model)
}

type shutdownStreamProvider struct {
	*types.BaseProvider
	streamStarted chan struct{}
	ctxCanceled   chan struct{}
	closeCount    atomic.Int32
}

func newShutdownStreamProvider() *shutdownStreamProvider {
	return &shutdownStreamProvider{
		BaseProvider:  types.NewBaseProvider("openai"),
		streamStarted: make(chan struct{}),
		ctxCanceled:   make(chan struct{}),
	}
}

func (p *shutdownStreamProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityStream}
}

func (p *shutdownStreamProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	ch := make(chan types.TextChunk, 1)
	ch <- types.TextChunk{Text: "hello"}
	close(p.streamStarted)
	go func() {
		defer close(ch)
		<-ctx.Done()
		close(p.ctxCanceled)
	}()
	return ch, nil
}

func (p *shutdownStreamProvider) Close() error {
	p.closeCount.Add(1)
	return nil
}

// C1-02: BaseContext cancels active SSE streams on Shutdown.
func TestC1_02_ShutdownCancelsActiveSSEStream(t *testing.T) {
	t.Parallel()

	provider := newShutdownStreamProvider()
	p := New(Config{
		Addr: "127.0.0.1:0",
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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- p.server.Serve(ln)
	}()

	reqDone := make(chan error, 1)
	go func() {
		resp, err := http.Post("http://"+ln.Addr().String()+"/v1/chat/completions",
			"application/json",
			strings.NewReader(`{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
		if err != nil {
			reqDone <- err
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		reqDone <- nil
	}()

	select {
	case <-provider.streamStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE stream to start")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, p.Shutdown(shutdownCtx))

	select {
	case <-provider.ctxCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected SSE stream context to be canceled by Shutdown")
	}

	assert.Equal(t, int32(1), provider.closeCount.Load())
	<-reqDone
	<-serveDone
}

type shutdownCloserSpy struct {
	closed atomic.Int32
}

func (s *shutdownCloserSpy) Close() error {
	s.closed.Add(1)
	return nil
}

// C1-02: p.wh.Shutdown still gets a bounded fallback context when p.server.Shutdown exhausts ctx.
func TestC1_02_ShutdownDoesNotStarveWormholeCleanup(t *testing.T) {
	t.Parallel()

	closer := &shutdownCloserSpy{}
	p := New(Config{
		Addr: "127.0.0.1:0",
		WormholeOpts: []wormhole.Option{
			wormhole.WithDiscovery(false),
			func(cfg *wormhole.Config) {
				cfg.Closers = append(cfg.Closers, closer)
			},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	p.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- p.server.Serve(ln)
	}()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := http.Get("http://" + ln.Addr().String() + "/block")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	<-handlerStarted

	// Short deadline expires during p.server.Shutdown while handler ignores ctx.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = p.Shutdown(shutdownCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(1), closer.closed.Load(), "wormhole cleanup should run with fallback context")

	close(releaseHandler)
	<-clientDone
	<-serveDone
}

// C1-03: EmbeddingInput unmarshals JSON null as nil so handleEmbeddings returns 400 input_required.
func TestC1_03_EmbeddingsNullInputRejected(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	rec := performRequest(p, http.MethodPost, "/v1/embeddings", `{"model":"openai/text-embedding-3-small","input":null}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "input_required", out.Error.Code)
}

// C1-04: completedResponsesEnvelope falls back on zero ID and Created.
func TestC1_04_CompletedResponsesEnvelopeZeroFallbacks(t *testing.T) {
	t.Parallel()

	before := time.Now().Unix()
	envelope := completedResponsesEnvelope(&types.TextResponse{
		Text:         "hello",
		FinishReason: types.FinishReasonStop,
	}, "gpt-test", nil)
	after := time.Now().Unix()

	assert.True(t, strings.HasPrefix(envelope.ID, "resp_"))
	assert.Greater(t, len(envelope.ID), len("resp_"))
	assert.GreaterOrEqual(t, envelope.CreatedAt, before)
	assert.LessOrEqual(t, envelope.CreatedAt, after)
	require.Len(t, envelope.Output, 1)
	assert.True(t, strings.HasPrefix(envelope.Output[0].ID, "msg_"))
	assert.Greater(t, len(envelope.Output[0].ID), len("msg_"))
}

// C1-05: handleResponses forwards response metadata and streamResponses logs Stream errors.
func TestC1_05_ResponsesMetadataAndStreamErrorLogging(t *testing.T) {
	t.Parallel()

	t.Run("forwards response metadata headers", func(t *testing.T) {
		t.Parallel()
		mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
			ID:           "resp-1",
			Text:         "ok",
			FinishReason: types.FinishReasonStop,
			ResponseMetadata: &types.ResponseMetadata{
				RequestID: "req_upstream_123",
				Headers:   http.Header{"X-Request-Id": {"req_hdr_456"}},
			},
		})
		p := newTestProxy(mock)
		rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"gpt-test","input":"hello"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "req_upstream_123", rec.Header().Get("X-Wormhole-Upstream-Request-ID"))
		assert.Equal(t, "req_hdr_456", rec.Header().Get("X-Request-Id"))
	})

	t.Run("logs stream creation error safely", func(t *testing.T) {
		t.Parallel()
		const secret = "upstream-secret-token-xyz"
		var logs bytes.Buffer
		p := New(Config{
			WormholeOpts: []wormhole.Option{
				wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
					return nil, errors.New(secret)
				}),
				wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
				wormhole.WithDefaultProvider("openai"),
				wormhole.WithDiscovery(false),
			},
			Logger: slog.New(slog.NewTextHandler(&logs, nil)),
		})

		rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"gpt-test","input":"hello","stream":true}`)
		require.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Contains(t, logs.String(), "responses stream creation failed")
		assert.NotContains(t, logs.String(), secret)
	})
}

// C1-06: Prefix-only model strings (e.g. "openai/") are rejected with 400 model_required across all endpoints.
func TestC1_06_RejectsPrefixOnlyModelAcrossEndpoints(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	endpoints := []struct {
		name string
		path string
		body string
	}{
		{name: "chat", path: "/v1/chat/completions", body: `{"model":"openai/","messages":[{"role":"user","content":"hi"}]}`},
		{name: "responses", path: "/v1/responses", body: `{"model":"openai/","input":"hi"}`},
		{name: "embeddings", path: "/v1/embeddings", body: `{"model":"openai/","input":"hi"}`},
		{name: "rerank", path: "/v1/rerank", body: `{"model":"openai/","query":"hi","documents":["doc"]}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			t.Parallel()
			rec := performRequest(p, http.MethodPost, ep.path, ep.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var out ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "model_required", out.Error.Code)
		})
	}
}

package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

type routeCaptureProvider struct {
	*wmtest.MockProvider
	model string
}

func (p *routeCaptureProvider) Text(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
	p.model = req.Model
	return p.MockProvider.Text(ctx, req)
}

func (p *routeCaptureProvider) Stream(ctx context.Context, req types.TextRequest) (<-chan types.TextChunk, error) {
	p.model = req.Model
	return p.MockProvider.Stream(ctx, req)
}

func (p *routeCaptureProvider) Embeddings(ctx context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	p.model = req.Model
	return p.MockProvider.Embeddings(ctx, req)
}

func (p *routeCaptureProvider) Rerank(ctx context.Context, req types.RerankRequest) (*types.RerankResponse, error) {
	p.model = req.Model
	return p.MockProvider.Rerank(ctx, req)
}

func TestProxyModelRoutingAcrossEndpoints(t *testing.T) {
	for _, routing := range []struct {
		name, defaultProvider, model, want string
	}{
		{"explicit gateway prefix", "", "openrouter/anthropic/example", "anthropic/example"},
		{"sole configured default", "", "openai/example", "openai/example"},
		{"explicit default", "openrouter", "anthropic/example", "anthropic/example"},
		{"one prefix only", "openrouter", "openrouter/openrouter/example", "openrouter/example"},
		{"plain model", "openrouter", "example", "example"},
	} {
		for _, endpoint := range []struct {
			name, path string
			body       map[string]any
		}{
			{"chat", "/v1/chat/completions", map[string]any{"messages": []map[string]string{{"role": "user", "content": "hello"}}}},
			{"chat stream", "/v1/chat/completions", map[string]any{"messages": []map[string]string{{"role": "user", "content": "hello"}}, "stream": true}},
			{"responses", "/v1/responses", map[string]any{"input": "hello"}},
			{"responses stream", "/v1/responses", map[string]any{"input": "hello", "stream": true}},
			{"embeddings", "/v1/embeddings", map[string]any{"input": "hello"}},
			{"rerank", "/v1/rerank", map[string]any{"query": "hello", "documents": []string{"hello"}}},
		} {
			t.Run(routing.name+"/"+endpoint.name, func(t *testing.T) {
				stop := types.FinishReasonStop
				provider := &routeCaptureProvider{MockProvider: wmtest.NewMockProvider("openrouter").
					WithStreamChunks([]types.TextChunk{{Text: "ok", FinishReason: &stop}}).
					WithTextResponse(types.TextResponse{Text: "ok", FinishReason: stop})}
				p := New(Config{
					DefaultProvider: routing.defaultProvider,
					WormholeOpts: []wormhole.Option{
						wormhole.WithCustomProvider("openrouter", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
						wormhole.WithProviderConfig("openrouter", types.ProviderConfig{}),
						wormhole.WithDiscovery(false),
					},
					Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				})
				endpoint.body["model"] = routing.model
				body, err := json.Marshal(endpoint.body)
				require.NoError(t, err)
				rec := performRequest(p, http.MethodPost, endpoint.path, string(body))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, routing.want, provider.model)
			})
		}
	}
}

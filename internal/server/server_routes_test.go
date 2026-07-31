package server

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

func TestProxyRouteAllowlist(t *testing.T) {
	t.Parallel()

	p := newTestProxy(wmtest.NewMockProvider("openai"))
	patterns := make([]string, 0, len(p.routes()))
	for _, route := range p.routes() {
		patterns = append(patterns, route.pattern)
	}
	slices.Sort(patterns)

	assert.Equal(t, []string{
		"GET /health",
		"GET /v1/models",
		"POST /v1/chat/completions",
		"POST /v1/embeddings",
		"POST /v1/rerank",
		"POST /v1/responses",
	}, patterns)
}

func TestProxyRejectsProviderAdministrationWithoutProviderInvocation(t *testing.T) {
	t.Parallel()

	var factoryCalls atomic.Int32
	p := New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
				factoryCalls.Add(1)
				return wmtest.NewMockProvider("openai"), nil
			}),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() {
		assert.Equal(t, int32(0), factoryCalls.Load())
	})

	adminPaths := []string{
		"/v1/assistants",
		"/v1/threads",
		"/v1/files",
		"/v1/vector_stores",
		"/v1/batches",
		"/v1/fine_tuning/jobs",
		"/v1/messages/batches",
		"/v1/realtime",
		"/v1/realtime/sessions",
	}
	for _, path := range adminPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := performRequest(p, http.MethodPost, path, "")
			require.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

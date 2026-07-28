package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/discovery"
	"github.com/garyblankenship/wormhole/v2/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogFaultMatrixProxyOutput(t *testing.T) {
	t.Parallel()

	catalogs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/alpha/models", "/beta/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"shared-model","owned_by":"fixture"}]}`)
		case "/failed/models":
			http.Error(w, "catalog unavailable", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(catalogs.Close)

	p := New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithProviderConfig("alpha", types.ProviderConfig{BaseURL: catalogs.URL + "/alpha"}),
			wormhole.WithProviderConfig("beta", types.ProviderConfig{BaseURL: catalogs.URL + "/beta"}),
			wormhole.WithProviderConfig("failed", types.ProviderConfig{BaseURL: catalogs.URL + "/failed"}),
			wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
				DisableFileCache:         true,
				DisableBackgroundRefresh: true,
			}),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })

	rec := performRequest(p, http.MethodGet, "/v1/models", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var out ModelListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Data, 2)
	assert.Equal(t, []string{"alpha/shared-model", "beta/shared-model"}, []string{
		out.Data[0].ID,
		out.Data[1].ID,
	})
	assert.Equal(t, []string{"alpha", "beta"}, []string{
		out.Data[0].OwnedBy,
		out.Data[1].OwnedBy,
	})
	assert.NotContains(t, rec.Body.String(), "failed")
}

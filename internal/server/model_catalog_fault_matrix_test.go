package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogFaultMatrixProxyOutput(t *testing.T) {
	t.Parallel()

	catalogs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthy/models", "/partial/models", "/stale/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"shared-model","owned_by":"fixture"}]}`)
		case "/empty/models":
			_, _ = io.WriteString(w, `{"data":[]}`)
		case "/failed/models":
			http.Error(w, "catalog unavailable: sk-secret https://private.example", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(catalogs.Close)

	tests := []struct {
		name        string
		providers   []string
		cacheTTL    time.Duration
		wantIDs     []string
		wantFailed  string
		wantStale   string
		clientQuery bool
	}{
		{name: "healthy", providers: []string{"healthy"}, cacheTTL: time.Hour, wantIDs: []string{"healthy/shared-model"}, wantFailed: "0", wantStale: "0"},
		{name: "partial failure", providers: []string{"partial", "failed"}, cacheTTL: time.Hour, wantIDs: []string{"partial/shared-model"}, wantFailed: "1", wantStale: "0"},
		{name: "all failed", providers: []string{"failed"}, cacheTTL: time.Hour, wantIDs: []string{}, wantFailed: "1", wantStale: "0"},
		{name: "stale fallback", providers: []string{"stale"}, cacheTTL: -time.Hour, wantIDs: []string{"stale/shared-model"}, wantFailed: "0", wantStale: "1"},
		{name: "empty catalog", providers: []string{"empty"}, cacheTTL: time.Hour, wantIDs: []string{}, wantFailed: "0", wantStale: "0"},
		{name: "client version special response", providers: []string{"healthy"}, cacheTTL: time.Hour, wantIDs: []string{}, clientQuery: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := make([]wormhole.Option, 0, len(tt.providers)+1)
			for _, provider := range tt.providers {
				opts = append(opts, wormhole.WithProviderConfig(provider, types.ProviderConfig{BaseURL: catalogs.URL + "/" + provider}))
			}
			opts = append(opts, wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
				CacheTTL:                 tt.cacheTTL,
				DisableFileCache:         true,
				DisableBackgroundRefresh: true,
			}))
			p := New(Config{WormholeOpts: opts, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })

			path := "/v1/models"
			if tt.clientQuery {
				path += "?client_version=0.144.1"
			}
			rec := performRequest(p, http.MethodGet, path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			if tt.clientQuery {
				var out struct {
					Models []any `json:"models"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Empty(t, out.Models)
				assert.Empty(t, rec.Header().Get("X-Wormhole-Discovery-Failed-Count"))
				assert.Empty(t, rec.Header().Get("X-Wormhole-Discovery-Stale-Count"))
				return
			}

			assert.Equal(t, tt.wantFailed, rec.Header().Get("X-Wormhole-Discovery-Failed-Count"))
			assert.Equal(t, tt.wantStale, rec.Header().Get("X-Wormhole-Discovery-Stale-Count"))
			assert.NotContains(t, rec.Header().Get("X-Wormhole-Discovery-Failed-Count"), "catalog unavailable")
			assert.NotContains(t, rec.Header().Get("X-Wormhole-Discovery-Stale-Count"), "private.example")

			var out ModelListResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			gotIDs := make([]string, 0, len(out.Data))
			for _, entry := range out.Data {
				gotIDs = append(gotIDs, entry.ID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)
			assert.NotContains(t, rec.Body.String(), "sk-secret")
			assert.NotContains(t, rec.Body.String(), "private.example")
		})
	}
}

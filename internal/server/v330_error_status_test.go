package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

func TestV330DecodeFailureHTTPStatus(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "decode-id")
		_, _ = w.Write([]byte("invalid-json"))
	}))
	t.Cleanup(upstream.Close)
	zero := 0
	wrapper := providers.NewHTTPClientWrapper("openai", types.ProviderConfig{MaxRetries: &zero}, nil, &providers.NoAuthStrategy{}, upstream.Client())
	var decoded map[string]any
	metadata, err := wrapper.DoRequestWithMetadata(t.Context(), http.MethodGet, upstream.URL, nil, &decoded)
	require.Error(t, err)
	require.Equal(t, http.StatusOK, metadata.StatusCode)
	whErr, ok := types.AsWormholeError(err)
	require.True(t, ok)
	require.Zero(t, whErr.StatusCode)
	recorder := httptest.NewRecorder()
	writeUpstreamError(recorder, err)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "decode-id", recorder.Header().Get("X-Wormhole-Upstream-Request-ID"))
	require.Contains(t, recorder.Body.String(), "upstream_error")
}

func TestV330ProxyRejectsSuccessErrorStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusFound, 999} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			err := types.WrapError(types.ErrorCodeProvider, "decode failure", false, errors.New("fixture"))
			err.StatusCode = status
			writeUpstreamError(recorder, err)
			require.Equal(t, http.StatusBadGateway, recorder.Code)
		})
	}
}

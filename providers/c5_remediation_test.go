package providers

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestExtractTLSConfigFromProviderConfig_CipherSuitesUint16Slice(t *testing.T) {
	t.Parallel()

	suites := []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_CHACHA20_POLY1305_SHA256}
	cfg := types.NewProviderConfig("test-key").
		WithTLSConfigParam("cipher_suites", suites)

	extracted := ExtractTLSConfigFromProviderConfig(cfg)
	require.NotNil(t, extracted)
	assert.Equal(t, suites, extracted.CipherSuites)

	// Mutating the extracted slice must not mutate the caller's slice.
	extracted.CipherSuites[0] = tls.TLS_AES_256_GCM_SHA384
	assert.Equal(t, uint16(tls.TLS_AES_128_GCM_SHA256), suites[0])
}

func TestHTTPClientWrapper_Status529MappedToProviderError(t *testing.T) {
	t.Parallel()

	wrapper := NewHTTPClientWrapper("anthropic", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)

	assert.Equal(t, types.ErrorCodeProvider, wrapper.mapHTTPStatusToErrorCode(statusOverloaded))

	err := wrapper.buildErrorResponse(
		statusOverloaded,
		"529 Overloaded",
		"https://api.anthropic.com/v1/messages",
		http.Header{},
		[]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
	)
	we, ok := types.AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, types.ErrorCodeProvider, we.Code)
	assert.True(t, we.IsRetryable())
	assert.Equal(t, statusOverloaded, we.StatusCode)
}

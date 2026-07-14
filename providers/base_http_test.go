package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestBaseProviderHelpers(t *testing.T) {
	t.Parallel()

	provider := NewBaseProvider("test", types.ProviderConfig{BaseURL: "https://example.test"})
	if got := provider.GetBaseURL(); got != "https://example.test" {
		t.Fatalf("GetBaseURL() = %q, want configured URL", got)
	}
	empty := NewBaseProvider("test", types.ProviderConfig{})
	if got := empty.GetBaseURL(); got != "" {
		t.Fatalf("empty GetBaseURL() = %q, want empty", got)
	}

	if err := provider.NotImplementedError("Text"); err == nil || !strings.Contains(err.Error(), "Text") {
		t.Fatalf("NotImplementedError = %v", err)
	}
	if err := provider.ValidationError("bad"); !types.IsValidationError(err) {
		t.Fatalf("ValidationError = %v, want validation error", err)
	}
	if err := provider.ValidationErrorf("bad %s", "input"); !types.IsValidationError(err) {
		t.Fatalf("ValidationErrorf = %v, want validation error", err)
	}
	if err := provider.ProviderError("down"); !types.IsProviderConfigError(err) {
		t.Fatalf("ProviderError = %v, want provider error", err)
	}
	if err := provider.ProviderErrorf("down %d", 1); !types.IsProviderConfigError(err) {
		t.Fatalf("ProviderErrorf = %v, want provider error", err)
	}
	if err := provider.RequestError("request", errors.New("cause")); err == nil || !strings.Contains(err.Error(), "request") {
		t.Fatalf("RequestError = %v", err)
	}
	if err := provider.ModelError("missing"); !types.IsModelError(err) {
		t.Fatalf("ModelError = %v, want model error", err)
	}
	if err := provider.ModelErrorf("missing %s", "model"); !types.IsModelError(err) {
		t.Fatalf("ModelErrorf = %v, want model error", err)
	}
	if err := provider.AuthError("bad key"); !types.IsAuthError(err) {
		t.Fatalf("AuthError = %v, want auth error", err)
	}
	if err := provider.AuthErrorf("bad %s", "key"); !types.IsAuthError(err) {
		t.Fatalf("AuthErrorf = %v, want auth error", err)
	}
	if err := provider.WrapError(types.ErrorCodeNetwork, "network", errors.New("cause")); !types.IsNetworkError(err) {
		t.Fatalf("WrapError = %v, want network error", err)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestHTTPClientWrapperStreamRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(types.HeaderAccept) != types.ContentTypeEventStream {
			t.Fatalf("Accept = %q, want event-stream", r.Header.Get(types.HeaderAccept))
		}
		if r.Header.Get("x-custom") != "value" {
			t.Fatalf("x-custom = %q, want value", r.Header.Get("x-custom"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	t.Cleanup(server.Close)

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{
		Headers: map[string]string{"x-custom": "value"},
	}, nil, &NoAuthStrategy{}, server.Client())

	body, err := wrapper.StreamRequest(context.Background(), http.MethodPost, server.URL, map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("StreamRequest returned error: %v", err)
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if string(data) != "data: ok\n\n" {
		t.Fatalf("stream body = %q", string(data))
	}
}

func TestHTTPClientWrapperStreamRequestErrors(t *testing.T) {
	t.Parallel()

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)
	if _, err := wrapper.StreamRequest(context.Background(), http.MethodGet, "%", nil); err == nil {
		t.Fatal("StreamRequest invalid URL returned nil error")
	}

	authWrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &BearerAuthStrategy{}, nil)
	if _, err := authWrapper.StreamRequest(context.Background(), http.MethodGet, "https://example.test", nil); err == nil {
		t.Fatal("StreamRequest auth failure returned nil error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad stream"}}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	errorWrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, server.Client())
	_, err := errorWrapper.StreamRequest(context.Background(), http.MethodGet, server.URL+"?api_key=secretvalue", nil)
	if err == nil {
		t.Fatal("StreamRequest HTTP error returned nil")
	}
	var wormholeErr *types.WormholeError
	if !errors.As(err, &wormholeErr) || wormholeErr.Code != types.ErrorCodeRateLimit {
		t.Fatalf("StreamRequest HTTP error = %T %[1]v, want rate limit WormholeError", err)
	}
	if strings.Contains(wormholeErr.Details, "secretvalue") {
		t.Fatalf("error details leaked API key: %s", wormholeErr.Details)
	}
}

func TestHTTPTransportConfigHelpers(t *testing.T) {
	t.Parallel()

	tlsConfig := config.StrictTLSConfig()
	proxyURL, err := url.Parse("http://proxy.example.test")
	if err != nil {
		t.Fatal(err)
	}
	proxy := func(*http.Request) (*url.URL, error) { return proxyURL, nil }

	transport := DefaultHTTPTransportConfig().
		WithTLSConfig(&tlsConfig).
		WithProxy(proxy)
	if transport.TLSConfig != &tlsConfig {
		t.Fatal("WithTLSConfig did not set TLS config")
	}
	gotProxy, err := transport.Proxy(&http.Request{})
	if err != nil {
		t.Fatalf("proxy returned error: %v", err)
	}
	if gotProxy.String() != proxyURL.String() {
		t.Fatalf("proxy = %s, want %s", gotProxy, proxyURL)
	}

	if got := extractHostFromBaseURL("https://example.test:8443/v1"); got != "example.test:8443" {
		t.Fatalf("extractHostFromBaseURL = %q, want example.test:8443", got)
	}
	if got := extractHostFromBaseURL("%"); got != "" {
		t.Fatalf("extractHostFromBaseURL invalid = %q, want empty", got)
	}
	if key := transport.CacheKey("%"); key == "" {
		t.Fatal("CacheKey invalid base URL returned empty key")
	}

	proxyA := func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	proxyB := func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	fingerprintA := DefaultHTTPTransportConfig().WithProxy(proxyA).Fingerprint()
	fingerprintB := DefaultHTTPTransportConfig().WithProxy(proxyB).Fingerprint()
	if fingerprintA == fingerprintB {
		t.Fatal("transport fingerprints should distinguish different proxy functions")
	}
}

func TestHTTPTransportConfigValidateFailures(t *testing.T) {
	t.Parallel()

	insecureTLS := config.InsecureTLSConfig()
	tests := []struct {
		name   string
		config HTTPTransportConfig
	}{
		{name: "insecure tls", config: DefaultHTTPTransportConfig().WithTLSConfig(&insecureTLS)},
		{name: "max idle", config: DefaultHTTPTransportConfig().WithConnectionPooling(-1, 1, 1, time.Second)},
		{name: "max idle per host", config: DefaultHTTPTransportConfig().WithConnectionPooling(1, -1, 1, time.Second)},
		{name: "max conns per host", config: DefaultHTTPTransportConfig().WithConnectionPooling(1, 1, -1, time.Second)},
		{name: "idle timeout", config: DefaultHTTPTransportConfig().WithConnectionPooling(1, 1, 1, -time.Second)},
		{name: "dial timeout", config: DefaultHTTPTransportConfig().WithTimeouts(-time.Second, 0, 0, 0, 0)},
		{name: "tls handshake timeout", config: DefaultHTTPTransportConfig().WithTimeouts(0, 0, -time.Second, 0, 0)},
		{name: "expect continue timeout", config: DefaultHTTPTransportConfig().WithTimeouts(0, 0, 0, -time.Second, 0)},
		{name: "response header timeout", config: DefaultHTTPTransportConfig().WithTimeouts(0, 0, 0, 0, -time.Second)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.config.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

package providers

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/config"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestTransportRemediationClosesAllOwnedTransports(t *testing.T) {
	t.Parallel()
	closed := make(chan struct{}, 2)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{BaseURL: server.URL}, nil, &NoAuthStrategy{}, nil)
	t.Cleanup(func() { _ = wrapper.Close() })
	transportConfig := DefaultHTTPTransportConfig()
	transportConfig.MaxIdleConns++
	extra := wrapper.transportCache.newSecureHTTPClient(0, nil, &transportConfig, server.URL)
	for _, client := range []*http.Client{wrapper.GetHTTPClient(), extra} {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("consume response: read=%v close=%v", readErr, closeErr)
		}
	}
	if got := wrapper.transportCache.Metrics().Size; got != 2 {
		t.Fatalf("cached transports = %d, want 2", got)
	}
	if err := wrapper.Close(); err != nil {
		t.Fatal(err)
	}
	if got := wrapper.transportCache.Metrics().Size; got != 0 {
		t.Fatalf("closed cache retains %d transports", got)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	for range 2 {
		select {
		case <-closed:
		case <-ctx.Done():
			t.Fatal("owned idle connection did not close")
		}
	}
}

func TestTransportRemediationClosesEvictedDefaultTransport(t *testing.T) {
	t.Parallel()
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{BaseURL: server.URL}, nil, &NoAuthStrategy{}, nil)
	t.Cleanup(func() { _ = wrapper.Close() })
	defaultTransport := wrapper.GetHTTPClient().Transport.(*http.Transport)
	cache := wrapper.transportCache
	cache.mu.Lock()
	for _, entry := range cache.transports {
		entry.lastUsedNano.Store(0)
	}
	cache.mu.Unlock()
	for i := range maxCachedTransports {
		cache.set(time.Duration(i).String(), &http.Transport{})
	}
	cache.mu.RLock()
	retained := false
	for _, entry := range cache.transports {
		retained = retained || entry.transport == defaultTransport
	}
	cache.mu.RUnlock()
	if retained {
		t.Fatal("default transport was not evicted")
	}

	resp, err := wrapper.GetHTTPClient().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wrapper.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("reused default idle connection did not close after eviction")
	}
}

type transportRemediationCallerTransport struct {
	closes int
}

func (*transportRemediationCallerTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("unused")
}

func (tr *transportRemediationCallerTransport) CloseIdleConnections() { tr.closes++ }

func TestTransportRemediationPreservesInjectedTransport(t *testing.T) {
	t.Parallel()
	transport := &transportRemediationCallerTransport{}
	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, &http.Client{Transport: transport})
	for range 2 {
		if err := wrapper.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if transport.closes != 0 {
		t.Fatalf("closed caller transport %d times", transport.closes)
	}
}

func TestTransportRemediationPreservesReplacementTransport(t *testing.T) {
	t.Parallel()
	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)
	transport := &transportRemediationCallerTransport{}
	wrapper.GetHTTPClient().Transport = transport
	for range 2 {
		if err := wrapper.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if transport.closes != 0 {
		t.Fatalf("closed caller replacement transport %d times", transport.closes)
	}
}

func TestTransportRemediationEffectiveTLSHandshakeTimeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		tlsTimeout time.Duration
		want       time.Duration
	}{
		{name: "configured TLS", tlsTimeout: 3 * time.Second, want: 3 * time.Second},
		{name: "unspecified TLS", want: 7 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tlsConfig := config.DefaultTLSConfig().WithHandshakeTimeout(tt.tlsTimeout)
			transportConfig := DefaultHTTPTransportConfig()
			transportConfig.TLSHandshakeTimeout = 7 * time.Second
			client := NewSecureHTTPClient(0, &tlsConfig, &transportConfig, "")
			transport := client.Transport.(*http.Transport)
			t.Cleanup(transport.CloseIdleConnections)
			if transport.TLSHandshakeTimeout != tt.want {
				t.Fatalf("handshake timeout = %v, want %v", transport.TLSHandshakeTimeout, tt.want)
			}
			if tlsConfig.HandshakeTimeout != tt.tlsTimeout || transportConfig.TLSHandshakeTimeout != 7*time.Second {
				t.Fatal("client creation mutated caller configuration")
			}
		})
	}
	client := NewSecureHTTPClient(0, nil, nil, "")
	transport := client.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	if want := DefaultHTTPTransportConfig().TLSHandshakeTimeout; transport.TLSHandshakeTimeout != want {
		t.Fatalf("default handshake timeout = %v, want %v", transport.TLSHandshakeTimeout, want)
	}
}

func TestTransportRemediationTLSHandshakeCacheIdentity(t *testing.T) {
	t.Parallel()
	cache := NewTransportCache()
	t.Cleanup(cache.closeIdleConnections)
	firstTLS := config.DefaultTLSConfig().WithHandshakeTimeout(3 * time.Second)
	secondTLS := firstTLS.WithHandshakeTimeout(4 * time.Second)
	if firstTLS.Fingerprint() == secondTLS.Fingerprint() {
		t.Fatal("TLS fingerprint ignores handshake timeout")
	}
	first := cache.newSecureHTTPClient(0, &firstTLS, nil, "")
	second := cache.newSecureHTTPClient(0, &secondTLS, nil, "")
	repeated := cache.newSecureHTTPClient(0, &firstTLS, nil, "")
	if first.Transport == second.Transport {
		t.Fatal("different effective handshake timeouts share a transport")
	}
	if first.Transport != repeated.Transport {
		t.Fatal("identical handshake configuration did not reuse transport")
	}
	if first.Transport.(*http.Transport).TLSHandshakeTimeout != 3*time.Second || second.Transport.(*http.Transport).TLSHandshakeTimeout != 4*time.Second {
		t.Fatal("cache identity does not match effective transport timeout")
	}
}

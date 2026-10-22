package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func metadataWrapper(client HTTPClient, additional ...string) *HTTPClientWrapper {
	zero := 0
	return NewHTTPClientWrapper("fixture", types.ProviderConfig{MaxRetries: &zero, ResponseHeaderNames: additional}, nil, &NoAuthStrategy{}, client)
}

func TestV330HTTPMetadataConcurrentAndFiltered(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", r.URL.Query().Get("id"))
		w.Header().Set("X-Ratelimit-Remaining-Tokens", "42")
		w.Header().Set("X-Custom", "private-sdk")
		w.Header().Set("Authorization", "credential")
		w.Header().Set("Set-Cookie", "session=secret")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	wrapper := metadataWrapper(server.Client(), "x-custom", "AUTHORIZATION", "Set-Cookie")
	var group sync.WaitGroup
	for i := range 16 {
		group.Go(func() {
			id := fmt.Sprint(i)
			metadata, err := wrapper.DoRequestWithMetadata(t.Context(), "GET", server.URL+"?id="+id, nil, nil)
			if err != nil {
				t.Error(err)
				return
			}
			if metadata.RequestID != id || metadata.Provider != "fixture" || metadata.StatusCode != 200 {
				t.Errorf("metadata = %#v", metadata)
			}
			headers := http.Header(metadata.Headers)
			if headers.Get("X-Custom") != "private-sdk" || headers.Get("X-Ratelimit-Remaining-Tokens") != "42" {
				t.Errorf("headers = %#v", headers)
			}
			if headers.Get("Authorization") != "" || headers.Get("Set-Cookie") != "" {
				t.Errorf("credential exposed: %#v", headers)
			}
			if SafeResponseHeaders(headers).Get("X-Custom") != "" {
				t.Error("SDK extra leaked to proxy allowlist")
			}
		})
	}
	group.Wait()
}

func TestV330HTTPMetadataFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"decode", "invalid-json", 200}, {"provider", `{"error":{"message":"bad input"}}`, 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Request-ID", scenario.name)
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.body)
			}))
			t.Cleanup(server.Close)
			var response map[string]any
			metadata, err := metadataWrapper(server.Client()).DoRequestWithMetadata(t.Context(), "GET", server.URL, nil, &response)
			var providerErr *types.WormholeError
			if !errors.As(err, &providerErr) || metadata == nil || metadata.RequestID != scenario.name || providerErr.ResponseMetadata.RequestID != scenario.name {
				t.Fatalf("metadata=%#v err=%v", metadata, err)
			}
			if scenario.status == http.StatusOK && providerErr.StatusCode != 0 {
				t.Fatalf("decode error uses success HTTP status: %d", providerErr.StatusCode)
			}
			if scenario.status >= http.StatusBadRequest && providerErr.StatusCode != scenario.status {
				t.Fatalf("provider error lost status: %d", providerErr.StatusCode)
			}
			metadata.Headers["Request-Id"][0] = "changed"
			if providerErr.ResponseMetadata.RequestID != scenario.name || http.Header(providerErr.ResponseMetadata.Headers).Get("Request-ID") != scenario.name {
				t.Fatal("error shares metadata headers")
			}
		})
	}
}

func TestV330HTTPMetadataRetryUsesExposedAttempt(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := attempts.Add(1)
		w.Header().Set("X-Request-ID", fmt.Sprint(attempt))
		if attempt == 1 {
			w.WriteHeader(503)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	wrapper := metadataWrapper(server.Client())
	wrapper.retryClient.Config.MaxRetries = 1
	wrapper.retryClient.Config.InitialDelay = 1
	wrapper.retryClient.Config.MaxDelay = 1
	metadata, err := wrapper.DoRequestWithMetadata(t.Context(), "GET", server.URL, nil, nil)
	if err != nil || metadata.RequestID != "2" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	wrapper.retryClient.Config.MaxRetries = 1
	attempts.Store(0)
	// Force both attempts to return the first attempt's status using a separate fixture.
	failure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "exhausted")
		w.WriteHeader(503)
	}))
	t.Cleanup(failure.Close)
	metadata, err = wrapper.DoRequestWithMetadata(t.Context(), "GET", failure.URL, nil, nil)
	var providerErr *types.WormholeError
	if !errors.As(err, &providerErr) || metadata.RequestID != "exhausted" || providerErr.ResponseMetadata.RequestID != "exhausted" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
}

type metadataHTTPClient func(*http.Request) (*http.Response, error)

func (f metadataHTTPClient) Do(r *http.Request) (*http.Response, error) { return f(r) }

type metadataBrokenBody struct{ closed bool }

func (b *metadataBrokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (b *metadataBrokenBody) Close() error             { b.closed = true; return nil }

func TestV330HTTPMetadataStreamingReadAndClose(t *testing.T) {
	t.Parallel()
	body := &metadataBrokenBody{}
	var requestContext context.Context
	client := metadataHTTPClient(func(req *http.Request) (*http.Response, error) {
		requestContext = req.Context()
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"stream"}}, Body: body}, nil
	})
	wrapper := metadataWrapper(client)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream, metadata, err := wrapper.StreamRequestWithMetadata(ctx, "POST", "https://fixture.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(stream)
	var providerErr *types.WormholeError
	if !errors.Is(err, io.ErrUnexpectedEOF) || !errors.As(err, &providerErr) || providerErr.ResponseMetadata.RequestID != "stream" || metadata.RequestID != "stream" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	cancel()
	if requestContext.Err() == nil {
		t.Fatal("stream did not inherit cancellation")
	}
	if err := stream.Close(); err != nil || !body.closed {
		t.Fatalf("close err=%v closed=%v", err, body.closed)
	}
}

func TestV330HTTPMetadataConnectionHeadersExcluded(t *testing.T) {
	t.Parallel()
	headers := http.Header{"Connection": []string{"X-Custom, X-Request-ID"}, "X-Custom": []string{"unsafe"}, "X-Request-Id": []string{"unsafe"}, "Upgrade": []string{"websocket"}}
	if got := FilterResponseHeaders(headers, []string{"x-custom", "upgrade"}); len(got) != 0 {
		t.Fatalf("hop-by-hop exposed: %#v", got)
	}
	filtered := FilterResponseHeaders(http.Header{"x-custom": []string{"one"}}, []string{"X-CUSTOM"})
	if !strings.EqualFold(filtered.Get("X-Custom"), "one") {
		t.Fatal(filtered)
	}
}

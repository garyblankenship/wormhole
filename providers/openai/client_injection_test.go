package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type ownedTransport struct{ calls, closes int }

func (t *ownedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))}, nil
}
func (t *ownedTransport) CloseIdleConnections() { t.closes++ }

func TestInjectedClientRemainsCallerOwned(t *testing.T) {
	t.Parallel()
	transport := &ownedTransport{}
	client := &http.Client{Transport: transport}
	p := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client)
	if _, err := p.Text(context.Background(), types.TextRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if transport.closes != 0 {
		t.Fatal("provider closed caller transport")
	}
	if _, err := p.TextWithEvidence(context.Background(), types.TextRequest{}); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 2 {
		t.Fatalf("calls=%d", transport.calls)
	}
}

func TestInjectedNonstandardClientConcreteFacadeHasNoFallback(t *testing.T) {
	t.Parallel()
	client := &evidenceClient{body: `{}`}
	p := NewWithHTTPClient(types.ProviderConfig{NoAuth: true}, client)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://unreachable.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("fallback replaced supplied client")
	}
}

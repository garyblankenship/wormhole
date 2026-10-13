package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func adapterDecisionRequest() types.DecisionsRequest {
	return types.DecisionsRequest{Model: "~typesafe/jev-latest", State: "state", Questions: map[string]types.DecisionQuestion{"q": {Type: types.DecisionNoul, Instructions: "decide"}}}
}

const adapterDecisionBody = `{"answers":{"q":{"type":"noul","noul":0}}}`

func TestDecisionsEndpointAndUnsupported(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ base, path string }{{"/api/v1", "/api/alpha/decisions"}, {"/api/v1/", "/api/alpha/decisions"}, {"/api", "/api/alpha/decisions"}, {"", "/alpha/decisions"}, {"/custom/v1/path/v1", "/custom/v1/path/alpha/decisions"}, {"/custom%2Fsegment/v1", "/custom/segment/alpha/decisions"}} {
		t.Run(tc.base, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%s want %s", r.URL.Path, tc.path)
				}
				if _, err := fmt.Fprint(w, adapterDecisionBody); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			p := NewWithName("openrouter", types.ProviderConfig{BaseURL: server.URL + tc.base, NoAuth: true})
			t.Cleanup(func() { _ = p.Close() })
			if _, err := p.Decisions(t.Context(), adapterDecisionRequest()); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("unsupported no HTTP", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
		t.Cleanup(server.Close)
		p := NewWithName("openai", types.ProviderConfig{BaseURL: server.URL, NoAuth: true})
		t.Cleanup(func() { _ = p.Close() })
		if _, err := p.Decisions(t.Context(), adapterDecisionRequest()); err == nil {
			t.Fatal("expected unsupported")
		}
		if calls.Load() != 0 {
			t.Fatal("unsupported called HTTP")
		}
	})
	t.Run("malformed base", func(t *testing.T) {
		t.Parallel()
		p := NewWithName("openrouter", types.ProviderConfig{BaseURL: "://bad", NoAuth: true})
		t.Cleanup(func() { _ = p.Close() })
		if _, err := p.Decisions(t.Context(), adapterDecisionRequest()); err == nil {
			t.Fatal("expected URL error")
		}
	})
}

func TestDecisionsHTTPErrorAndRetry(t *testing.T) {
	t.Parallel()
	for _, status := range []int{401, 422, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(status)
				if _, err := fmt.Fprint(w, `{"error":{"message":"fixture rejected"}}`); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			zero := 0
			p := NewWithName("openrouter", types.ProviderConfig{BaseURL: server.URL, NoAuth: true, MaxRetries: &zero})
			t.Cleanup(func() { _ = p.Close() })
			_, err := p.Decisions(t.Context(), adapterDecisionRequest())
			var we *types.WormholeError
			if !errors.As(err, &we) || we.StatusCode != status {
				t.Fatalf("error=%v want HTTP %d", err, status)
			}
			if status == 429 && (we.Code != types.ErrorCodeRateLimit || we.RetryAfter != 2*time.Second) {
				t.Fatalf("lost rate limit diagnostics: %+v", we)
			}
			if status == 401 && we.Code != types.ErrorCodeAuth {
				t.Fatalf("lost auth code: %+v", we)
			}
		})
	}
	t.Run("429 retry", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				if _, err := fmt.Fprint(w, `{"error":{"message":"retry"}}`); err != nil {
					t.Error(err)
				}
				return
			}
			if _, err := fmt.Fprint(w, adapterDecisionBody); err != nil {
				t.Error(err)
			}
		}))
		t.Cleanup(server.Close)
		one := 1
		delay := time.Millisecond
		p := NewWithName("openrouter", types.ProviderConfig{BaseURL: server.URL, NoAuth: true, MaxRetries: &one, RetryDelay: &delay, RetryMaxDelay: &delay})
		t.Cleanup(func() { _ = p.Close() })
		if _, err := p.Decisions(t.Context(), adapterDecisionRequest()); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 2 {
			t.Fatalf("calls=%d", calls.Load())
		}
	})
	t.Run("cancel retry", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
			if _, err := fmt.Fprint(w, `{"error":{"message":"retry"}}`); err != nil {
				t.Error(err)
			}
			cancel()
		}))
		t.Cleanup(server.Close)
		one := 1
		p := NewWithName("openrouter", types.ProviderConfig{BaseURL: server.URL, NoAuth: true, MaxRetries: &one})
		t.Cleanup(func() { _ = p.Close() })
		_, err := p.Decisions(ctx, adapterDecisionRequest())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel=%v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("retried after cancel: %d", calls.Load())
		}
	})
}

func TestDecisionsRejectsMalformedSuccess(t *testing.T) {
	t.Parallel()
	for _, body := range []string{``, `null`, `{}`, `{"answers":{}}`, `{"answers":{"q":{"type":"noul","noul":null}}}`, `{"answers":{"q":{"type":"noul","noul":"yes"}}}`, `{"answers":{"q":{"type":"noul","noul":2}}}`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := fmt.Fprint(w, body); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			p := NewWithName("openrouter", types.ProviderConfig{BaseURL: server.URL, NoAuth: true})
			t.Cleanup(func() { _ = p.Close() })
			if _, err := p.Decisions(t.Context(), adapterDecisionRequest()); err == nil {
				t.Fatal("accepted malformed success")
			}
		})
	}
}

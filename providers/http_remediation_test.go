package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestRemediationC4I01DeterministicRetryJitter(t *testing.T) {
	t.Parallel()
	baseConfig := retryConfig{InitialDelay: 100 * time.Millisecond, MaxDelay: time.Second, BackoffMultiple: 2, Jitter: true}
	client := newRetryableHTTPClient(nil, baseConfig)
	for _, tc := range []struct {
		name    string
		attempt int
		sample  float64
		want    time.Duration
	}{
		{"negative first jitter", 0, -1, 80 * time.Millisecond},
		{"positive first jitter", 0, 1, 120 * time.Millisecond},
		{"negative capped jitter", 20, -1, 800 * time.Millisecond},
		{"positive capped jitter", 20, 1, time.Second},
		{"neutral exponential", 2, 0, 400 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newRetryableHTTPClient(nil, baseConfig)
			if got := client.backoffDelay(tc.attempt, tc.sample); got != tc.want {
				t.Fatalf("delay=%v want=%v", got, tc.want)
			}
		})
	}
	client.Config.Jitter = false
	if got := client.backoffDelay(0, -1); got != 100*time.Millisecond {
		t.Fatalf("disabled jitter=%v", got)
	}
	client.Config.Jitter = true
	client.Config.InitialDelay = time.Nanosecond
	if got := client.backoffDelay(0, -1); got != time.Nanosecond {
		t.Fatalf("positive floor=%v", got)
	}
	if got := client.calculateDelay(0, 2*time.Second); got != time.Second {
		t.Fatalf("retry-after cap=%v", got)
	}
}

// C4-R01: HTTP deadlines continue through body reads; disabled timeouts defer
// lifetime control to the caller. A stream is never retried after body output.
func TestRemediationC4R01StreamLifetime(t *testing.T) {
	t.Parallel()
	for _, finite := range []bool{true, false} {
		t.Run(map[bool]string{true: "finite", false: "disabled"}[finite], func(t *testing.T) {
			t.Parallel()
			release := make(chan struct{})
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.WriteString(w, "data: first\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = io.WriteString(w, "data: final\n\n")
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			timeout := time.Duration(0)
			if finite {
				timeout = 100 * time.Millisecond
			}
			wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}.WithHTTPTimeout(timeout), nil, &NoAuthStrategy{}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			body, err := wrapper.StreamRequest(ctx, http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = body.Close() }()
			first := make([]byte, len("data: first\n\n"))
			if _, err := io.ReadFull(body, first); err != nil || string(first) != "data: first\n\n" {
				t.Fatalf("first=%q err=%v", first, err)
			}
			done := make(chan error, 1)
			var remainder []byte
			go func() { var readErr error; remainder, readErr = io.ReadAll(body); done <- readErr }()
			if finite {
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("finite deadline did not stop body read")
					}
				case <-time.After(time.Second):
					t.Fatal("finite body read blocked")
				}
			} else {
				select {
				case err := <-done:
					t.Fatalf("disabled timeout ended stream: %v", err)
				case <-time.After(150 * time.Millisecond):
				}
				close(release)
				select {
				case err := <-done:
					if err != nil || !strings.Contains(string(remainder), "final") {
						t.Fatalf("remainder=%q err=%v", remainder, err)
					}
				case <-time.After(time.Second):
					t.Fatal("released stream blocked")
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("partial stream replayed: requests=%d", requests.Load())
			}
		})
	}
}

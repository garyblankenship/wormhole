package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestHTTPAttemptRetryAndStatus(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithRetries(1, time.Nanosecond).WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, server.Client())
	var result map[string]any
	if err := wrapper.DoRequest(context.Background(), http.MethodGet, server.URL+"/sensitive-path", nil, &result); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("underlying Do calls = %d, want 2", calls)
	}
	assertHTTPAttemptPair(t, events, []int{http.StatusServiceUnavailable, http.StatusOK})
}

func TestHTTPAttemptZeroRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithNoRetries().WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, server.Client())
	if err := wrapper.DoRequest(context.Background(), http.MethodGet, server.URL, nil, nil); err == nil {
		t.Fatal("expected HTTP error")
	}
	if calls != 1 {
		t.Fatalf("underlying Do calls = %d, want 1", calls)
	}
	assertHTTPAttemptPair(t, events, []int{http.StatusServiceUnavailable})
}

func TestHTTPAttemptCancellationStopsRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithRetries(2, time.Nanosecond).WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
		if event.Phase == types.HTTPAttemptComplete {
			cancel()
		}
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, server.Client())
	if err := wrapper.DoRequest(ctx, http.MethodGet, server.URL, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("DoRequest error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("underlying Do calls = %d, want 1", calls)
	}
	assertHTTPAttemptPair(t, events, []int{http.StatusServiceUnavailable})
}

func TestHTTPAttemptCanceledDo(t *testing.T) {
	t.Parallel()
	calls := 0
	client := metadataHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, context.Canceled
	})
	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithNoRetries().WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, client)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wrapper.retryClient.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || calls != 1 || len(events) != 2 ||
		events[1].Outcome != types.HTTPAttemptCanceled || events[1].StatusCode != 0 {
		t.Fatalf("error = %v, calls = %d, events = %+v", err, calls, events)
	}
}

func TestHTTPAttemptPreCanceledDoesNotIssueDo(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client := metadataHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, nil
	})
	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithNoRetries().WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wrapper.retryClient.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || calls != 0 || len(events) != 0 {
		t.Fatalf("error = %v, calls = %d, events = %+v", err, calls, events)
	}
}

func TestHTTPAttemptTransportFailurePrivacy(t *testing.T) {
	t.Parallel()
	secret := "secret-token-in-transport-error"
	calls := 0
	client := metadataHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New(secret)
	})
	var events []types.HTTPAttemptEvent
	cfg := types.ProviderConfig{}.WithNoRetries().WithHTTPAttemptCallback(func(event types.HTTPAttemptEvent) {
		events = append(events, event)
	})
	wrapper := NewHTTPClientWrapper("fixture", cfg, nil, &NoAuthStrategy{}, client)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := wrapper.retryClient.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil || calls != 1 {
		t.Fatalf("error = %v; calls = %d, want error and one call", err, calls)
	}
	if len(events) != 2 || events[0] != (types.HTTPAttemptEvent{Phase: types.HTTPAttemptStart, Attempt: 1}) ||
		events[1].Phase != types.HTTPAttemptComplete || events[1].Attempt != 1 ||
		events[1].Outcome != types.HTTPAttemptTransportError || events[1].StatusCode != 0 || events[1].Duration < 0 {
		t.Fatalf("unexpected transport event sequence: %+v", events)
	}
	for _, event := range events {
		if strings.Contains(fmt.Sprint(event), secret) || strings.Contains(fmt.Sprint(event), "/private") {
			t.Fatalf("event leaked request or error data: %+v", event)
		}
	}
	for i := range reflect.TypeFor[types.HTTPAttemptEvent]().NumField() {
		if reflect.TypeFor[types.HTTPAttemptEvent]().Field(i).Type.Kind() == reflect.String && reflect.TypeFor[types.HTTPAttemptEvent]().Field(i).Name != "Phase" && reflect.TypeFor[types.HTTPAttemptEvent]().Field(i).Name != "Outcome" {
			t.Fatal("event has an unbounded string field")
		}
	}
}

func assertHTTPAttemptPair(t *testing.T, events []types.HTTPAttemptEvent, statuses []int) {
	t.Helper()
	if len(events) != 2*len(statuses) {
		t.Fatalf("events = %+v, want %d start/completion events", events, 2*len(statuses))
	}
	for i, status := range statuses {
		start, complete := events[2*i], events[2*i+1]
		if start != (types.HTTPAttemptEvent{Phase: types.HTTPAttemptStart, Attempt: i + 1}) ||
			complete.Phase != types.HTTPAttemptComplete || complete.Attempt != i+1 ||
			complete.StatusCode != status || complete.Outcome != types.HTTPAttemptResponse || complete.Duration < 0 {
			t.Fatalf("attempt %d events = %+v, %+v", i+1, start, complete)
		}
	}
}

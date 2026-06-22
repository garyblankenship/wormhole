package providers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "temporary timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var _ net.Error = timeoutErr{}

func TestHTTPClientWrapperTimeoutsAndClientFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout int
		want    time.Duration
	}{
		{name: "zero means unlimited", timeout: 0, want: 0},
		{name: "positive seconds", timeout: 7, want: 7 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{Timeout: tt.timeout}, nil, &NoAuthStrategy{}, nil)
			if got := wrapper.GetHTTPTimeout(); got != tt.want {
				t.Fatalf("GetHTTPTimeout() = %v, want %v", got, tt.want)
			}
			if got := wrapper.GetHTTPClient(); got == nil {
				t.Fatal("GetHTTPClient() returned nil")
			}
		})
	}
}

func TestHTTPClientWrapperBuildRequestAndParseResponse(t *testing.T) {
	t.Parallel()

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{
		APIKey:  "secret",
		Headers: map[string]string{"x-custom": "value"},
	}, nil, &BearerAuthStrategy{}, nil)

	req, err := wrapper.buildRequest(context.Background(), http.MethodPost, "https://example.test", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("buildRequest returned error: %v", err)
	}
	if req.Header.Get(types.HeaderContentType) != types.ContentTypeJSON {
		t.Fatalf("Content-Type = %q, want %q", req.Header.Get(types.HeaderContentType), types.ContentTypeJSON)
	}
	if req.Header.Get(types.HeaderAuthorization) != "Bearer secret" {
		t.Fatalf("Authorization = %q, want Bearer secret", req.Header.Get(types.HeaderAuthorization))
	}
	if req.Header.Get("x-custom") != "value" {
		t.Fatalf("x-custom = %q, want value", req.Header.Get("x-custom"))
	}
	if req.GetBody == nil || req.ContentLength == 0 {
		t.Fatal("request body was not made replayable")
	}

	var decoded map[string]string
	if err := wrapper.parseResponse([]byte(`{"ok":"true"}`), &decoded); err != nil {
		t.Fatalf("parseResponse returned error: %v", err)
	}
	if decoded["ok"] != "true" {
		t.Fatalf("decoded response = %#v", decoded)
	}
	if err := wrapper.parseResponse(nil, &decoded); err != nil {
		t.Fatalf("parseResponse with empty body returned error: %v", err)
	}
	if err := wrapper.parseResponse([]byte(`{`), &decoded); err == nil {
		t.Fatal("parseResponse with invalid JSON returned nil error")
	}
}

func TestHTTPClientWrapperLimitsProviderResponseBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("x", maxProviderResponseBodyBytes) + `"}`))
	}))
	t.Cleanup(server.Close)

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, server.Client())
	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodGet, server.URL, nil, &out)
	if err == nil {
		t.Fatal("DoRequest returned nil error for oversized response body")
	}
	if !strings.Contains(err.Error(), "provider response body exceeded") {
		t.Fatalf("DoRequest error = %v, want response body limit", err)
	}
}

func TestHTTPClientWrapperLimitsStreamErrorBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", maxProviderResponseBodyBytes+1)))
	}))
	t.Cleanup(server.Close)

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, server.Client())
	_, err := wrapper.StreamRequest(context.Background(), http.MethodGet, server.URL, nil)
	if err == nil {
		t.Fatal("StreamRequest returned nil error for oversized error body")
	}
	if !strings.Contains(err.Error(), "provider response body exceeded") {
		t.Fatalf("StreamRequest error = %v, want response body limit", err)
	}
}

func TestHTTPClientWrapperErrorHelpers(t *testing.T) {
	t.Parallel()

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)

	statusTests := []struct {
		status int
		want   types.ErrorCode
	}{
		{status: 401, want: types.ErrorCodeAuth},
		{status: 404, want: types.ErrorCodeModel},
		{status: 429, want: types.ErrorCodeRateLimit},
		{status: 400, want: types.ErrorCodeRequest},
		{status: 408, want: types.ErrorCodeTimeout},
		{status: 500, want: types.ErrorCodeProvider},
		{status: 418, want: types.ErrorCodeNetwork},
	}
	for _, tt := range statusTests {
		t.Run(string(tt.want)+"_"+http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()

			if got := wrapper.mapHTTPStatusToErrorCode(tt.status); got != tt.want {
				t.Fatalf("mapHTTPStatusToErrorCode(%d) = %s, want %s", tt.status, got, tt.want)
			}
		})
	}

	if got := wrapper.extractErrorMessage(400, "400 Bad Request", []byte(`{"error":{"message":"bad input"}}`)); got != "bad input" {
		t.Fatalf("extractErrorMessage = %q, want bad input", got)
	}
	if got := wrapper.extractErrorMessage(400, "400 Bad Request", []byte(`not-json`)); got != "HTTP 400: 400 Bad Request" {
		t.Fatalf("extractErrorMessage fallback = %q", got)
	}
	if got := wrapper.maskAPIKeyInURL("https://example.test/path?api_key=abcdefghijkl&token=short&x=1"); got != "https://example.test/path?api_key=abcd%2A%2A%2A%2Aijkl&token=%2A%2A%2A%2A&x=1" {
		t.Fatalf("maskAPIKeyInURL = %q", got)
	}
	if got := wrapper.maskAPIKeyInURL("%"); got != "%" {
		t.Fatalf("maskAPIKeyInURL invalid = %q, want %%", got)
	}

	err := wrapper.handleRequestError(context.Background(), timeoutErr{})
	var wormholeErr *types.WormholeError
	if !errors.As(err, &wormholeErr) || wormholeErr.Code != types.ErrorCodeTimeout {
		t.Fatalf("timeout error = %T %[1]v, want Wormhole timeout", err)
	}

	err = wrapper.handleRequestError(context.Background(), errors.New("dial tcp 127.0.0.1:8000: connect: operation not permitted"))
	if !strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("network error = %v, want low-level cause in top-level string", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wrapper.handleRequestError(ctx, errors.New("network")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}
}

func TestHTTPClientWrapperClose(t *testing.T) {
	t.Parallel()

	wrapper := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, &http.Client{Transport: http.DefaultTransport})
	if err := wrapper.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

// FIX: buildErrorResponse must surface the provider's structured error
// type/code in Details so ClassifyError can tell an OpenAI 429
// "insufficient_quota" (quota cap) from a plain rate-limit, and an Anthropic
// "overloaded_error" from a generic 5xx.
func TestBuildErrorResponseSurfacesProviderTypeCode(t *testing.T) {
	t.Parallel()
	w := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantSubstr []string
	}{
		{
			name:       "openai insufficient_quota",
			statusCode: 429,
			body:       `{"error":{"message":"You exceeded your quota","type":"insufficient_quota","code":"insufficient_quota"}}`,
			wantSubstr: []string{"type=insufficient_quota", "code=insufficient_quota"},
		},
		{
			name:       "anthropic overloaded_error",
			statusCode: 529,
			body:       `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			wantSubstr: []string{"type=overloaded_error"},
		},
		{
			name:       "gemini resource_exhausted",
			statusCode: 429,
			body:       `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"Resource has been exhausted"}}`,
			wantSubstr: []string{"status=RESOURCE_EXHAUSTED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := w.buildErrorResponse(tt.statusCode, "", "https://example.test", nil, []byte(tt.body))
			wErr, ok := types.AsWormholeError(err)
			if !ok {
				t.Fatalf("expected *types.WormholeError, got %T", err)
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(wErr.Details, want) {
					t.Fatalf("Details %q missing %q", wErr.Details, want)
				}
			}
		})
	}
}

// FIX: the surfaced type/code must make ClassifyError robust — an OpenAI
// insufficient_quota 429 classifies as quota, not a retryable rate-limit.
func TestBuildErrorResponseClassifiesInsufficientQuotaAsQuota(t *testing.T) {
	t.Parallel()
	w := NewHTTPClientWrapper("test", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)
	body := `{"error":{"message":"You exceeded your quota","type":"insufficient_quota","code":"insufficient_quota"}}`
	err := w.buildErrorResponse(429, "", "https://example.test", nil, []byte(body))
	if got := types.ClassifyError(err); got != types.ErrorClassQuota {
		t.Fatalf("ClassifyError = %v, want %v", got, types.ErrorClassQuota)
	}
}

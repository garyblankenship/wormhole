package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

// erroringTextProvider returns a configurable error from Text, used to verify
// the proxy maps structured WormholeError status codes instead of flattening
// every failure to 502.
type erroringTextProvider struct {
	*wmtest.MockProvider
	err error
}

type erroringStreamProvider struct {
	*wmtest.MockProvider
	err error
}

func (p *erroringStreamProvider) Stream(_ context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	return nil, p.err
}

func (p *erroringTextProvider) Text(_ context.Context, _ types.TextRequest) (*types.TextResponse, error) {
	return nil, p.err
}

func newErroringProxy(err error) *proxy {
	prov := &erroringTextProvider{MockProvider: wmtest.NewMockProvider("openai"), err: err}
	return New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
				return prov, nil
			}),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func newErroringStreamProxy(err error) *proxy {
	prov := &erroringStreamProvider{MockProvider: wmtest.NewMockProvider("openai"), err: err}
	return New(Config{
		WormholeOpts: []wormhole.Option{
			wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) {
				return prov, nil
			}),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithDiscovery(false),
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestProxyUpstreamRetryAfterHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantHeader string
	}{
		{
			name: "exact seconds and upstream status preserved",
			err: types.NewWormholeError(types.ErrorCodeProvider, "unavailable", true).
				WithStatusCode(http.StatusServiceUnavailable).
				WithRetryAfter(3 * time.Second),
			wantStatus: http.StatusServiceUnavailable,
			wantHeader: "3",
		},
		{
			name: "fractional duration rounds up",
			err: types.NewWormholeError(types.ErrorCodeRateLimit, "limited", true).
				WithStatusCode(http.StatusTooManyRequests).
				WithRetryAfter(1500 * time.Millisecond),
			wantStatus: http.StatusTooManyRequests,
			wantHeader: "2",
		},
		{
			name:       "code default does not synthesize header",
			err:        types.NewWormholeError(types.ErrorCodeRateLimit, "limited", true),
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "plain error omits header",
			err:        errors.New("unavailable"),
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newErroringProxy(tt.err)
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions",
				`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantHeader, rec.Header().Get("Retry-After"))
		})
	}
}

func TestProxyStreamingCreationErrorPreservesRetryAfter(t *testing.T) {
	t.Parallel()

	err := types.NewWormholeError(types.ErrorCodeProvider, "unavailable", true).
		WithStatusCode(http.StatusServiceUnavailable).
		WithRetryAfter(2500 * time.Millisecond)
	p := newErroringStreamProxy(err)
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions",
		`{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "3", rec.Header().Get("Retry-After"))
}

func TestProxyMapsUpstreamErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantType   string
		wantMsg    string
		notMsg     string
	}{
		{
			name:       "rate limit maps to 429",
			err:        types.NewWormholeError(types.ErrorCodeRateLimit, "quota bucket team-alpha exhausted", true).WithStatusCode(http.StatusTooManyRequests),
			wantStatus: http.StatusTooManyRequests,
			wantType:   "rate_limit_error",
			wantMsg:    "upstream rate limit exceeded",
			notMsg:     "team-alpha",
		},
		{
			name:       "auth maps to 401",
			err:        types.NewWormholeError(types.ErrorCodeAuth, "invalid API key sk-test...abcd", false).WithStatusCode(http.StatusUnauthorized),
			wantStatus: http.StatusUnauthorized,
			wantType:   "authentication_error",
			wantMsg:    "upstream authentication failed",
			notMsg:     "sk-test",
		},
		{
			name:       "local invalid request maps to 400 with actionable message",
			err:        types.NewWormholeError(types.ErrorCodeValidation, "model name is required", false),
			wantStatus: http.StatusBadRequest,
			wantType:   "invalid_request_error",
			wantMsg:    "model name is required",
			notMsg:     "upstream request rejected",
		},
		{
			name:       "plain error falls back to 502",
			err:        errors.New("boom"),
			wantStatus: http.StatusBadGateway,
			wantType:   "api_error",
			wantMsg:    "upstream provider error",
			notMsg:     "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newErroringProxy(tt.err)
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions",
				`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)

			require.Equal(t, tt.wantStatus, rec.Code)

			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.wantType, body.Error.Type)
			assert.Equal(t, tt.wantMsg, body.Error.Message)
			assert.NotContains(t, body.Error.Message, tt.notMsg)
		})
	}
}

func TestProxyMapsDeveloperAndFunctionRoles(t *testing.T) {
	t.Parallel()

	prov := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(prov)

	body := `{"model":"gpt-test","messages":[` +
		`{"role":"developer","content":"be terse"},` +
		`{"role":"function","tool_call_id":"call_1","content":"result"},` +
		`{"role":"user","content":"hi"}` +
		`]}`

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, rec.Code)

	msgs := prov.lastRequest().Messages
	require.Len(t, msgs, 3)
	assert.Equal(t, types.RoleSystem, msgs[0].GetRole(), "developer role must map to system")
	assert.Equal(t, types.RoleTool, msgs[1].GetRole(), "function role must map to tool result")
	assert.Equal(t, types.RoleUser, msgs[2].GetRole())

	toolMsg, ok := msgs[1].(*types.ToolResultMessage)
	require.True(t, ok, "function message must be a *types.ToolResultMessage")
	assert.Equal(t, "call_1", toolMsg.ToolCallID)
}

func TestUpstreamErrorStatus_SDKInternalErrorsMapByCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code       types.ErrorCode
		wantStatus int
	}{
		{types.ErrorCodeAuth, http.StatusUnauthorized},
		{types.ErrorCodeRateLimit, http.StatusTooManyRequests},
		{types.ErrorCodeTimeout, http.StatusGatewayTimeout},
		{types.ErrorCodeRequest, http.StatusBadRequest},
		{types.ErrorCodeModel, http.StatusBadRequest},
	}
	for _, tc := range cases {
		err := types.NewWormholeError(tc.code, "msg", false) // StatusCode left 0
		status, _, _ := upstreamErrorStatus(err)
		assert.Equalf(t, tc.wantStatus, status, "code %s should map to %d, got %d", tc.code, tc.wantStatus, status)
	}
	// An upstream-provided status must still win over the code-based mapping.
	withStatus := types.NewWormholeError(types.ErrorCodeAuth, "msg", false).WithStatusCode(403)
	if st, _, _ := upstreamErrorStatus(withStatus); st != 403 {
		t.Fatalf("upstream StatusCode must win: got %d, want 403", st)
	}
}

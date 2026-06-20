package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	wmtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// erroringTextProvider returns a configurable error from Text, used to verify
// the proxy maps structured WormholeError status codes instead of flattening
// every failure to 502.
type erroringTextProvider struct {
	*wmtest.MockProvider
	err error
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

func TestProxyMapsUpstreamErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantType   string
	}{
		{
			name:       "rate limit maps to 429",
			err:        types.NewWormholeError(types.ErrorCodeRateLimit, "rate limit exceeded", true).WithStatusCode(http.StatusTooManyRequests),
			wantStatus: http.StatusTooManyRequests,
			wantType:   "rate_limit_error",
		},
		{
			name:       "auth maps to 401",
			err:        types.NewWormholeError(types.ErrorCodeAuth, "invalid API key", false).WithStatusCode(http.StatusUnauthorized),
			wantStatus: http.StatusUnauthorized,
			wantType:   "authentication_error",
		},
		{
			name:       "plain error falls back to 502",
			err:        errors.New("boom"),
			wantStatus: http.StatusBadGateway,
			wantType:   "api_error",
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

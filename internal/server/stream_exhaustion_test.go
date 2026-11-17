package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	wmtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

type exhaustionProvider struct {
	*wmtest.MockProvider
	failure error
}

func (p *exhaustionProvider) Stream(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
	return nil, p.failure
}

func (p *exhaustionProvider) Text(context.Context, types.TextRequest) (*types.TextResponse, error) {
	return nil, p.failure
}

func exhaustedStreamError(t *testing.T, failure error) error {
	t.Helper()
	p := &exhaustionProvider{MockProvider: wmtest.NewMockProvider("openai"), failure: failure}
	client := wormhole.New(wormhole.WithDiscovery(false), wormhole.WithDefaultProvider("openai"),
		wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) { return p, nil }),
		wormhole.WithProviderConfig("openai", types.ProviderConfig{}))
	stream, err := client.Text().Model("first").Prompt("hello").WithFallback("second").Stream(context.Background())
	require.NoError(t, err)
	var result error
	for chunk := range stream {
		require.Nil(t, result)
		result = chunk.Error
	}
	require.Error(t, result)
	require.ErrorIs(t, result, failure)
	return result
}

func TestProxyExhaustedStreamErrorsKeepSafeMessageAndFinalStatus(t *testing.T) {
	failure := types.ErrRateLimited.WithDetails("secret rate details").WithStatusCode(429)
	failure.RetryAfter = 2 * time.Second
	aggregate := exhaustedStreamError(t, failure)
	// Single route errors keep their original identity and mapping.
	status, kind, message := upstreamErrorStatus(failure)
	require.Equal(t, 429, status)
	require.Equal(t, "rate_limit_error", kind)
	require.Equal(t, "upstream rate limit exceeded", message)
	status, kind, message = upstreamErrorStatus(aggregate)
	require.Equal(t, 429, status)
	require.Equal(t, "rate_limit_error", kind)
	require.Equal(t, aggregate.Error(), message)

	rec := httptest.NewRecorder()
	writeUpstreamError(rec, aggregate)
	require.Equal(t, 429, rec.Code)
	require.Equal(t, "2", rec.Header().Get("Retry-After"))
	require.Contains(t, rec.Body.String(), aggregate.Error())
	require.NotContains(t, rec.Body.String(), "secret")

	rec = httptest.NewRecorder()
	writeStreamError(rec, rec, aggregate)
	require.Contains(t, rec.Body.String(), aggregate.Error())
	require.NotContains(t, rec.Body.String(), "[DONE]")
	require.NotContains(t, rec.Body.String(), "secret")

	rec = httptest.NewRecorder()
	writer := responsesSSEWriter{w: rec}
	writeResponsesFailure(&writer, "response-id", "model", 0, aggregate)
	require.NoError(t, writer.err)
	require.Contains(t, rec.Body.String(), "response.failed")
	require.Contains(t, rec.Body.String(), aggregate.Error())
	require.NotContains(t, rec.Body.String(), "secret")
}

type orderedExhaustionError struct{ failures []error }

func (e *orderedExhaustionError) Error() string                { return "safe exhausted attempts" }
func (e *orderedExhaustionError) Unwrap() []error              { return e.failures }
func (e *orderedExhaustionError) StreamAttemptErrors() []error { return e.failures }

func TestProxyExhaustionSelectsFinalAttemptForClassification(t *testing.T) {
	validation := types.ErrValidation.WithDetails("secret invalid input")
	rateLimit := types.ErrRateLimited.WithStatusCode(http.StatusTooManyRequests)
	aggregate := &orderedExhaustionError{failures: []error{validation, rateLimit}}
	require.ErrorIs(t, aggregate, validation)
	require.ErrorIs(t, aggregate, rateLimit)
	var first *types.WormholeError
	require.ErrorAs(t, aggregate, &first)
	require.Same(t, validation, first)
	status, kind, message := upstreamErrorStatus(aggregate)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_error", kind)
	require.NotContains(t, message, "secret")
	require.Equal(t, "rate_limit_error", responsesErrorCode(aggregate))

	aggregate.failures = []error{rateLimit, errors.New("secret raw provider error")}
	status, kind, message = upstreamErrorStatus(aggregate)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "api_error", kind)
	require.NotContains(t, message, "secret")
	require.Equal(t, "upstream_error", responsesErrorCode(aggregate))
}

func TestProxyRoutesExhaustedStreamFailure(t *testing.T) {
	aggregate := exhaustedStreamError(t, types.ErrRateLimited.WithDetails("secret provider body").WithStatusCode(429))
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
		for _, streaming := range []bool{false, true} {
			t.Run(endpoint+"/"+fmt.Sprint(streaming), func(t *testing.T) {
				provider := &exhaustionProvider{MockProvider: wmtest.NewMockProvider("openai"), failure: aggregate}
				p := New(Config{WormholeOpts: []wormhole.Option{
					wormhole.WithDiscovery(false), wormhole.WithDefaultProvider("openai"),
					wormhole.WithCustomProvider("openai", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
					wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
				}})
				body := map[string]any{"model": "test", "stream": streaming}
				if endpoint == "/v1/responses" {
					body["input"] = "hello"
				} else {
					body["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
				}
				data, err := json.Marshal(body)
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				p.server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(data))))
				wantStatus := http.StatusTooManyRequests
				if streaming && endpoint == "/v1/responses" {
					wantStatus = http.StatusOK
					require.Contains(t, rec.Body.String(), "response.failed")
				}
				require.Equal(t, wantStatus, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), aggregate.Error())
				require.NotContains(t, rec.Body.String(), "secret")
				require.NotContains(t, rec.Body.String(), "[DONE]")
				require.NotContains(t, rec.Body.String(), "response.completed")
			})
		}
	}
}

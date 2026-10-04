package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/providers/openai"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestProxyOpenAIResponsesStreamEmitsCompletedToolCallsOnce(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/responses", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`data: {"type":"response.output_item.added","item_id":"item-1","item":{"type":"function_call","call_id":"call-1","name":"lookup","arguments":""}}` + "\n\n",
			`data: {"type":"response.output_item.added","item_id":"item-2","item":{"type":"function_call","call_id":"call-2","name":"lookup","arguments":""}}` + "\n\n",
			`data: {"type":"response.function_call_arguments.delta","item_id":"item-1","delta":"{\"q\":\"A\"}"}` + "\n\n",
			`data: {"type":"response.function_call_arguments.delta","item_id":"item-2","delta":"{\"q\":\"B\"}"}` + "\n\n",
			`data: {"type":"response.completed","response":{"id":"resp-1","model":"gpt-5","status":"completed","output":[{"type":"function_call","id":"item-1","call_id":"call-1","name":"lookup","arguments":"{\"q\":\"A\"}"},{"type":"function_call","id":"item-2","call_id":"call-2","name":"lookup","arguments":"{\"q\":\"B\"}"}]}}` + "\n\n",
		} {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	t.Cleanup(upstream.Close)

	p := New(Config{WormholeOpts: []wormhole.Option{
		wormhole.WithCustomProvider("openai", func(cfg types.ProviderConfig) (types.Provider, error) {
			cfg.BaseURL = upstream.URL
			cfg.UseResponsesAPI = true
			return openai.New(cfg), nil
		}),
		wormhole.WithProviderConfig("openai", types.ProviderConfig{APIKey: "test-key"}),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithDiscovery(false),
	}})
	t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-5","stream":true,"messages":[{"role":"user","content":"lookup"}]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Equal(t, 1, strings.Count(body, `"id":"call-1"`))
	assert.Equal(t, 1, strings.Count(body, `"id":"call-2"`))
	assert.Contains(t, body, `"name":"lookup"`)
	assert.Equal(t, 1, strings.Count(body, `"arguments":"{\"q\":\"A\"}"`))
	assert.Equal(t, 1, strings.Count(body, `"arguments":"{\"q\":\"B\"}"`))
	assert.NotContains(t, body, `"id":"item-1"`)
	assert.NotContains(t, body, `"id":"item-2"`)
}

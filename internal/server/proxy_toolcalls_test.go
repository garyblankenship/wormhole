package server

import (
	"encoding/json"
	"net/http"
	"testing"

	wmtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyResponseToolCallsPassthrough(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		FinishReason: types.FinishReasonToolCalls,
		ToolCalls: []types.ToolCall{{
			ID:        "call_1",
			Type:      "function",
			Name:      "get_weather",
			Arguments: map[string]any{"city": "SF"},
		}},
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"weather?"}]}`)

	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Choices, 1)
	require.Len(t, out.Choices[0].Message.ToolCalls, 1)

	toolCall := out.Choices[0].Message.ToolCalls[0]
	assert.Equal(t, "call_1", toolCall.ID)
	assert.Equal(t, "function", toolCall.Type)
	assert.Equal(t, "get_weather", toolCall.Function.Name)

	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(toolCall.Function.Arguments), &args))
	assert.Equal(t, "SF", args["city"])
}

func TestProxyToolChoiceAndToolsAccepted(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID:           "chat-1",
		Model:        "gpt-test",
		Text:         "ok",
		FinishReason: types.FinishReasonStop,
	})
	p := newTestProxy(mock)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"d","parameters":{"type":"object"}}}],"tool_choice":"auto"}`)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestProxyRedactsUpstreamErrorDetails(t *testing.T) {
	t.Parallel()

	leakErr := types.NewWormholeError(types.ErrorCodeRateLimit, "quota bucket team-alpha exhausted", true).
		WithStatusCode(http.StatusTooManyRequests).
		WithDetails(`URL: https://api.example.com/v1?api_key=sk-SECRET-12345\nResponse: {"error":"key sk-SECRET-12345 invalid"}`)
	p := newErroringProxy(leakErr)

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "upstream rate limit exceeded")
	assert.NotContains(t, rec.Body.String(), "team-alpha")
	assert.NotContains(t, rec.Body.String(), "sk-SECRET-12345")
	assert.NotContains(t, rec.Body.String(), "URL:")
	assert.NotContains(t, rec.Body.String(), "api.example.com")
}

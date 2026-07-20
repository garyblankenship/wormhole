package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
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

func TestProxyDropsNonPortableToolTypes(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	body := `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"namespace","name":"multi_agent_v1"},{"type":"web_search"},{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}]}`

	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", body)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, provider.lastRequest().Tools, 1)
	assert.Equal(t, "get_weather", provider.lastRequest().Tools[0].Name)
}

func TestProxyRejectsInvalidToolRequestsBeforeProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty tool name", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":""}}]}`},
		{name: "unknown tool choice", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tool_choice":"sometimes"}`},
		{name: "malformed tool choice", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"function"}}`},
		{name: "undeclared selected tool", body: `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"declared"}}],"tool_choice":{"type":"function","function":{"name":"missing"}}}`},
		{name: "malformed assistant arguments", body: `{"model":"gpt-test","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{bad"}}]}]}`},
		{name: "non object assistant arguments", body: `{"model":"gpt-test","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"[]"}}]}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newCapturingTextProvider("openai")
			p := newCapturingTestProxy(provider)
			rec := performRequest(p, http.MethodPost, "/v1/chat/completions", test.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var response ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			assert.Equal(t, "invalid_request_error", response.Error.Type)
			assert.Equal(t, "invalid_request_error", response.Error.Code)
			assert.Empty(t, provider.lastRequest().Model, "provider must not be invoked")
		})
	}
}

func TestProxyCanonicalizesEmptyAssistantToolArguments(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	rec := performRequest(p, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]}]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, provider.lastRequest().Messages, 1)
	assistant, ok := provider.lastRequest().Messages[0].(*types.AssistantMessage)
	require.True(t, ok)
	require.Len(t, assistant.ToolCalls, 1)
	assert.Equal(t, `{}`, assistant.ToolCalls[0].Function.Arguments)
	assert.NotNil(t, assistant.ToolCalls[0].Arguments)
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

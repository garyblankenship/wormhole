package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
	wmtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

func TestProxyResponsesTextStream(t *testing.T) {
	t.Parallel()

	stop := types.FinishReasonStop
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{
		{Text: "O"},
		{Text: "K", FinishReason: &stop, Usage: &types.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}},
	})
	p := newTestProxy(mock)
	rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"glm-5.2","input":"reply OK","stream":true}`)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, want := range []string{"response.created", "response.output_item.added", "response.content_part.added", `"content":[]`, `"delta":"O"`, `"delta":"K"`, "response.output_text.done", `"text":"OK"`, "response.content_part.done", "response.output_item.done", "response.completed"} {
		assert.Contains(t, body, want)
	}
	ordered := []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}
	last := -1
	for _, eventType := range ordered {
		index := strings.Index(body[last+1:], `"type":"`+eventType+`"`)
		require.GreaterOrEqual(t, index, 0, "missing ordered event %s", eventType)
		last += index + 1
	}
	assertResponsesSequenceNumbers(t, body)
}

func TestProxyResponsesRefusalStream(t *testing.T) {
	t.Parallel()

	stop := types.FinishReasonStop
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{
		{Refusal: "I cannot"},
		{Refusal: " help with that.", FinishReason: &stop},
	})
	p := newTestProxy(mock)
	rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"glm-5.2","input":"unsafe request","stream":true}`)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, want := range []string{
		`"type":"refusal"`,
		`"type":"response.refusal.delta"`,
		`"delta":"I cannot"`,
		`"delta":" help with that."`,
		`"type":"response.refusal.done"`,
		`"refusal":"I cannot help with that."`,
		`"type":"response.content_part.done"`,
		`"type":"response.output_item.done"`,
		`"type":"response.completed"`,
	} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, `"type":"response.output_text.delta"`)
	assertResponsesSequenceNumbers(t, body)
}

func assertResponsesSequenceNumbers(t *testing.T, body string) {
	t.Helper()
	sequence := 0
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event responsesEvent
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		assert.Equal(t, sequence, event.SequenceNumber)
		sequence++
	}
	require.Positive(t, sequence)
}

func TestProxyResponsesToolContinuation(t *testing.T) {
	t.Parallel()

	toolCalls := types.FinishReasonToolCalls
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{
		{ToolCalls: []types.ToolCall{{ID: "call_1", Name: "read_file", Function: &types.ToolCallFunction{Name: "read_file", Arguments: `{"path":`}}}},
		{ToolCalls: []types.ToolCall{{ID: "", Function: &types.ToolCallFunction{Arguments: `"x"}`}}}, FinishReason: &toolCalls},
	})
	p := newTestProxy(mock)
	body := `{"model":"glm-5.2","stream":true,"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"read x"}]},` +
		`{"type":"function_call","call_id":"old_call","name":"read_file","arguments":"{\"path\":\"old\"}"},` +
		`{"type":"function_call_output","call_id":"old_call","output":"old contents"}` +
		`],"tools":[{"type":"function","name":"read_file","description":"read","parameters":{"type":"object"}}]}`
	rec := performRequest(p, http.MethodPost, "/v1/responses", body)

	require.Equal(t, http.StatusOK, rec.Code)
	responseBody := rec.Body.String()
	assert.Contains(t, responseBody, `"type":"function_call"`)
	assert.Contains(t, responseBody, `"call_id":"call_1"`)
	assert.Contains(t, responseBody, `"arguments":"{\"path\":\"x\"}"`)
	assert.Contains(t, responseBody, "response.function_call_arguments.delta")
	assert.Contains(t, responseBody, "response.function_call_arguments.done")
	assert.Less(t, strings.Index(responseBody, "response.output_item.added"), strings.LastIndex(responseBody, "response.output_item.done"))
}

func TestProxyResponsesCustomTool(t *testing.T) {
	t.Parallel()

	toolCalls := types.FinishReasonToolCalls
	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{{
		ToolCalls:    []types.ToolCall{{ID: "call_patch", Name: "apply_patch", Function: &types.ToolCallFunction{Name: "apply_patch", Arguments: `{"input":"*** Begin Patch"}`}}},
		FinishReason: &toolCalls,
	}})
	p := newTestProxy(mock)
	rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"glm-5.2","stream":true,"input":"patch it","tools":[{"type":"custom","name":"apply_patch","description":"apply a patch","format":{"type":"grammar"}}]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"type":"custom_tool_call"`)
	assert.Contains(t, rec.Body.String(), `"input":"*** Begin Patch"`)
	assert.Contains(t, rec.Body.String(), "response.custom_tool_call_input.delta")
	assert.Contains(t, rec.Body.String(), "response.custom_tool_call_input.done")
}

func TestProxyResponsesCustomToolChoice(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	body := `{"model":"glm-5.2","input":"patch it",` +
		`"tools":[{"type":"custom","name":"apply_patch","description":"patch"}],` +
		`"tool_choice":{"type":"custom","name":"apply_patch"}}`
	rec := performRequest(p, http.MethodPost, "/v1/responses", body)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, provider.lastRequest().ToolChoice)
	assert.Equal(t, types.ToolChoiceTypeSpecific, provider.lastRequest().ToolChoice.Type)
	assert.Equal(t, "apply_patch", provider.lastRequest().ToolChoice.ToolName)
}

func TestProxyResponsesRejectsUnsupportedToolsAndChoices(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"model":"glm-5.2","input":"search","tools":[{"type":"web_search","name":"search"}]}`,
		`{"model":"glm-5.2","input":"call","tools":[{"type":"function","name":""}]}`,
		`{"model":"glm-5.2","input":"call","tools":[{"type":"function","name":"declared"}],"tool_choice":{"type":"function","name":"missing"}}`,
		`{"model":"glm-5.2","input":"call","tool_choice":"sometimes"}`,
	}
	for _, body := range tests {
		provider := newCapturingTextProvider("openai")
		rec := performRequest(newCapturingTestProxy(provider), http.MethodPost, "/v1/responses", body)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Empty(t, provider.lastRequest().Model)
	}
}

func TestProxyResponsesFailureEventIsSequenced(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithStreamChunks([]types.TextChunk{{Error: errors.New("upstream failed")}})
	rec := performRequest(newTestProxy(mock), http.MethodPost, "/v1/responses", `{"model":"glm-5.2","input":"hello","stream":true}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "response.failed")
	assertResponsesSequenceNumbers(t, rec.Body.String())
}

func TestProxyResponsesAllowedToolsFiltersPortableTools(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	body := `{"model":"glm-5.2","input":"read only",` +
		`"tools":[{"type":"function","name":"read_file"},{"type":"custom","name":"apply_patch"}],` +
		`"tool_choice":{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"read_file"}]}}`
	rec := performRequest(p, http.MethodPost, "/v1/responses", body)

	require.Equal(t, http.StatusOK, rec.Code)
	req := provider.lastRequest()
	require.Len(t, req.Tools, 1)
	assert.Equal(t, "read_file", req.Tools[0].Name)
	require.NotNil(t, req.ToolChoice)
	assert.Equal(t, types.ToolChoiceTypeAny, req.ToolChoice.Type)
}

func TestProxyResponsesZAIReasoningUsesThinkingOption(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("zai")
	p := newCapturingTestProxy(provider)
	p.defaultProvider = "zai"
	rec := performRequest(p, http.MethodPost, "/v1/responses", `{"model":"glm-5.2","input":"think","reasoning":{"effort":"high"}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	req := provider.lastRequest()
	assert.Nil(t, req.Reasoning)
	assert.Equal(t, map[string]any{"thinking": map[string]any{"type": "enabled"}}, req.ProviderOptions)
}

func TestResponsesMessagesRejectsMissingToolResultCallID(t *testing.T) {
	t.Parallel()

	_, err := responsesMessages(responsesRequest{Input: responsesInput{Items: []responsesInputItem{{
		Type: "function_call_output", Output: json.RawMessage(`"contents"`),
	}}}})
	require.EqualError(t, err, "function_call_output requires call_id")
}

func TestProxyResponsesInputTranslation(t *testing.T) {
	t.Parallel()

	provider := newCapturingTextProvider("openai")
	p := newCapturingTestProxy(provider)
	body := `{"model":"glm-5.2","instructions":"be precise","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"read x"}]},` +
		`{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"x\"}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"contents"}` +
		`]}`
	rec := performRequest(p, http.MethodPost, "/v1/responses", body)

	require.Equal(t, http.StatusOK, rec.Code)
	req := provider.lastRequest()
	require.Len(t, req.Messages, 4)
	assert.Equal(t, types.RoleSystem, req.Messages[0].GetRole())
	assert.Equal(t, "read x", req.Messages[1].GetContent())
	assert.Equal(t, types.RoleTool, req.Messages[3].GetRole())
}

func TestResponsesMessagesGroupsParallelToolCalls(t *testing.T) {
	t.Parallel()

	req := responsesRequest{Input: responsesInput{Items: []responsesInputItem{
		{Type: "message", Role: "user", Content: json.RawMessage(`"do both"`)},
		{Type: "function_call", CallID: "call_1", Name: "one", Arguments: `{}`},
		{Type: "function_call", CallID: "call_2", Name: "two", Arguments: `{}`},
	}}}
	messages, err := responsesMessages(req)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assistant, ok := messages[1].(*types.AssistantMessage)
	require.True(t, ok)
	assert.Len(t, assistant.ToolCalls, 2)
}

func TestResponsesMessagesAcceptsUserImage(t *testing.T) {
	t.Parallel()

	req := responsesRequest{Input: responsesInput{Items: []responsesInputItem{{
		Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"inspect"},{"type":"input_image","image_url":"https://example.com/a.png"}]`),
	}}}}
	messages, err := responsesMessages(req)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	user, ok := messages[0].(*types.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "inspect", user.Content)
	assert.Len(t, user.Media, 1)
}

func TestResponsesErrorCodePreservesSafeProviderCode(t *testing.T) {
	t.Parallel()

	err := types.ProviderError("openai", "failed", "context_length_exceeded")
	assert.Equal(t, "context_length_exceeded", responsesErrorCode(err))
	unsafe := types.ProviderError("openai", "failed", `{"secret":"value"}`)
	assert.Equal(t, string(types.ErrorCodeProvider), responsesErrorCode(unsafe))
}

func TestCompletedResponsesEnvelopeReportsTruncation(t *testing.T) {
	t.Parallel()

	envelope := completedResponsesEnvelope(&types.TextResponse{ID: "1", FinishReason: types.FinishReasonLength}, "glm-5.2", nil)
	assert.Equal(t, "incomplete", envelope.Status)
	assert.Equal(t, map[string]string{"reason": "max_output_tokens"}, envelope.IncompleteDetails)
}

func TestCompletedResponsesEnvelopeMapsRefusalAndUsageDetails(t *testing.T) {
	t.Parallel()

	envelope := completedResponsesEnvelope(&types.TextResponse{
		ID: "1", Refusal: "I cannot help with that", FinishReason: types.FinishReasonContentFilter,
		Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14, CacheReadTokens: 3, CacheWriteTokens: 2, ReasoningTokens: 1},
	}, "gpt-test", nil)

	require.Len(t, envelope.Output, 1)
	require.Len(t, envelope.Output[0].Content, 1)
	assert.Equal(t, "refusal", envelope.Output[0].Content[0].Type)
	assert.Equal(t, "I cannot help with that", envelope.Output[0].Content[0].Refusal)
	require.NotNil(t, envelope.Usage)
	assert.Equal(t, 3, envelope.Usage.InputTokenDetails.CachedTokens)
	assert.Equal(t, 2, envelope.Usage.InputTokenDetails.CacheWriteTokens)
	assert.Equal(t, 1, envelope.Usage.OutputTokenDetails.ReasoningTokens)
}

func TestChatResponseMapsRefusalAndUsageDetails(t *testing.T) {
	t.Parallel()

	mock := wmtest.NewMockProvider("openai").WithTextResponse(types.TextResponse{
		ID: "chat-1", Model: "gpt-test", Refusal: "No", FinishReason: types.FinishReasonContentFilter,
		Usage: &types.Usage{PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10, CacheReadTokens: 4, CacheWriteTokens: 3, ReasoningTokens: 2},
	})
	rec := performRequest(newTestProxy(mock), http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"request"}]}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var response ChatCompletionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Choices, 1)
	assert.Equal(t, "No", response.Choices[0].Message.Refusal)
	require.NotNil(t, response.Usage.PromptTokensDetails)
	assert.Equal(t, 4, response.Usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 3, response.Usage.PromptTokensDetails.CacheWriteTokens)
	require.NotNil(t, response.Usage.CompletionTokensDetails)
	assert.Equal(t, 2, response.Usage.CompletionTokensDetails.ReasoningTokens)
}

func TestProxyModelsCodexCompatibility(t *testing.T) {
	t.Parallel()
	p := newTestProxy(wmtest.NewMockProvider("openai"))

	codex := performRequest(p, http.MethodGet, "/v1/models?client_version=0.144.1", "")
	require.Equal(t, http.StatusOK, codex.Code)
	var codexBody map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(codex.Body.Bytes(), &codexBody))
	assert.Contains(t, codexBody, "models")
	assert.NotContains(t, codexBody, "data")

	standard := performRequest(p, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusOK, standard.Code)
	var standardBody map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(standard.Body.Bytes(), &standardBody))
	assert.Contains(t, standardBody, "data")
}

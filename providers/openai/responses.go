package openai

import (
	"context"
	"net/http"

	"github.com/garyblankenship/wormhole/v2/providers"
	providerstream "github.com/garyblankenship/wormhole/v2/providers/internal/stream"
	"github.com/garyblankenship/wormhole/v2/types"
)

const (
	responsesItemMessage            = "message"
	responsesItemFunctionCall       = "function_call"
	responsesItemFunctionCallOutput = "function_call_output"
	responsesContentInputText       = "input_text"
	responsesContentInputImage      = "input_image"
	responsesContentOutputText      = "output_text"
	responsesContentRefusal         = "refusal"
	responsesEventOutputTextDelta   = "response.output_text.delta"
	responsesEventOutputItemAdded   = "response.output_item.added"
	responsesEventFunctionArgsDelta = "response.function_call_arguments.delta"
	responsesEventReasoningDelta    = "response.reasoning_summary_text.delta"
	responsesEventReasoningDone     = "response.reasoning_summary_part.done"
	responsesEventCompleted         = "response.completed"
	responsesEventFailed            = "response.failed"
	responsesEventIncomplete        = "response.incomplete"
)

func (p *Provider) responsesText(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if err := p.validateResponsesSampling(request); err != nil {
		return nil, err
	}
	payload := p.buildResponsesPayload(&request)

	var response responsesResponse
	if err := p.DoRequest(ctx, http.MethodPost, p.responsesURL(), payload, &response); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, p.ProviderError(response.Error.Message, response.Error.Code)
	}

	textResponse := p.transformResponsesTextResponse(&response)
	textResponse.Provider = p.Name()

	if textResponse.Text == "" && len(textResponse.ToolCalls) == 0 {
		return nil, p.ProviderError("received empty response from OpenAI Responses API", "no output text or tool calls returned")
	}

	return textResponse, nil
}

func (p *Provider) responsesStream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	if err := p.validateResponsesSampling(request); err != nil {
		return nil, err
	}
	payload := p.buildResponsesPayload(&request)
	payload["stream"] = true

	body, err := p.StreamRequest(ctx, http.MethodPost, p.responsesURL(), payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(ctx, providerstream.ProcessSSE(ctx, body, p.parseResponsesStreamChunk, 100)), nil
}

// normalizeResponsesFormat adapts a Chat Completions response_format value to the
// shape the Responses API expects under text.format. For json_schema, the Chat
// shape nests {name,strict,schema} under a "json_schema" key; the Responses API
// wants those fields flattened alongside "type". Any other format (e.g. a
// json_object value) or non-map value is returned unchanged.
func normalizeResponsesFormat(rf any) any {
	m, ok := rf.(map[string]any)
	if !ok {
		return rf
	}
	if m["type"] != "json_schema" {
		return rf
	}
	js, ok := m["json_schema"].(map[string]any)
	if !ok {
		return rf
	}
	flat := map[string]any{"type": "json_schema"}
	for k, v := range js {
		flat[k] = v
	}
	return flat
}

func (p *Provider) buildResponsesPayload(request *types.TextRequest) map[string]any {
	messages, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		messages = request.Messages // fall through; provider will surface the issue
	}
	payload := map[string]any{
		"model": request.Model,
		"input": p.transformResponsesInput(messages),
	}

	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_output_tokens"] = p.maxTokensValue(*request.MaxTokens)
	}
	if request.ParallelToolCalls != nil {
		payload["parallel_tool_calls"] = *request.ParallelToolCalls
	}

	if reasoning := reasoningPayload(request.Reasoning); len(reasoning) > 0 {
		payload["reasoning"] = reasoning
	}

	if len(request.Tools) > 0 {
		payload["tools"] = p.transformResponsesTools(request.Tools)
		if request.ToolChoice != nil {
			payload["tool_choice"] = p.transformResponsesToolChoice(request.ToolChoice)
		}
	}

	if request.ResponseFormat != nil {
		payload["text"] = map[string]any{
			"format": normalizeResponsesFormat(request.ResponseFormat),
		}
	}

	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	return payload
}

func (p *Provider) validateResponsesSampling(request types.TextRequest) error {
	if request.FrequencyPenalty != nil || request.PresencePenalty != nil || request.Seed != nil {
		return p.ValidationError("frequency_penalty, presence_penalty, and seed are not supported by the OpenAI Responses API")
	}
	return nil
}

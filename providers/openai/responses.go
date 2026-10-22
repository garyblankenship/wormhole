package openai

import (
	"context"
	"net/http"

	"github.com/garyblankenship/wormhole/v3/providers"

	providerstream "github.com/garyblankenship/wormhole/v3/providers/internal/stream"
	"github.com/garyblankenship/wormhole/v3/types"
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

func (p *Provider) responsesText(ctx context.Context, request types.TextRequest, prepared []types.Message) (*types.TextResponse, error) {
	if err := p.validateResponsesSampling(request); err != nil {
		return nil, err
	}
	payload := p.buildResponsesPayload(&request, prepared)

	var response responsesResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, p.responsesURL(), payload, &response)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, providers.WithResponseMetadata(p.ProviderError(response.Error.Message, response.Error.Code), metadata)
	}

	textResponse := p.transformResponsesTextResponse(&response)
	textResponse.ResponseMetadata = metadata
	textResponse.Provider = p.Name()

	if textResponse.Text == "" && len(textResponse.ToolCalls) == 0 && !p.isPrewarm(request) {
		return nil, providers.WithResponseMetadata(p.ProviderError("received empty response from OpenAI Responses API", "no output text or tool calls returned"), metadata)
	}

	return textResponse, nil
}

func (p *Provider) responsesStream(ctx context.Context, request types.TextRequest, prepared []types.Message) (<-chan types.TextChunk, error) {
	if err := p.validateResponsesSampling(request); err != nil {
		return nil, err
	}
	payload := p.buildResponsesPayload(&request, prepared)
	payload["stream"] = true

	body, metadata, err := p.StreamRequestWithMetadata(ctx, http.MethodPost, p.responsesURL(), payload)
	if err != nil {
		return nil, err
	}

	state := newResponsesAnnotationState()
	parse := func(data []byte) (*types.TextChunk, error) {
		return p.parseResponsesStreamChunkWithState(data, state)
	}
	return p.stampMetadata(ctx, p.stampProvider(ctx, providerstream.ProcessSSE(ctx, body, parse, 100)), metadata), nil
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

func (p *Provider) buildResponsesPayload(request *types.TextRequest, messages []types.Message) map[string]any {
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

	if reasoning := responsesReasoningPayload(request.Reasoning); len(reasoning) > 0 {
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

func (p *Provider) isPrewarm(request types.TextRequest) bool {
	request.ProviderOptions = p.Config.MergedProviderOptions(request.Model, request.ProviderOptions)
	return types.IsCachePrewarm(request)
}

func responsesReasoningPayload(reasoning *types.Reasoning) map[string]any {
	if reasoning == nil {
		return nil
	}
	out := map[string]any{}
	if reasoning.Effort != "" {
		out["effort"] = string(reasoning.Effort)
	}
	if reasoning.Enabled != nil && !*reasoning.Enabled {
		out["effort"] = "none"
	}
	if reasoning.IncludeThoughts != nil && *reasoning.IncludeThoughts {
		out["summary"] = "auto"
	}
	return out
}

package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/internal/pool"
	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
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
	responsesEventCompleted         = "response.completed"
	responsesEventFailed            = "response.failed"
	responsesEventIncomplete        = "response.incomplete"
)

func (p *Provider) responsesText(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
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
	payload := p.buildResponsesPayload(&request)
	payload["stream"] = true

	body, err := p.StreamRequest(ctx, http.MethodPost, p.responsesURL(), payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(utils.ProcessStream(ctx, body, p.parseResponsesStreamChunk, 100)), nil
}

func (p *Provider) buildResponsesPayload(request *types.TextRequest) map[string]any {
	messages, err := providers.PrepareMessages(request.Messages)
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
		payload["max_output_tokens"] = *request.MaxTokens
	}

	if len(request.Tools) > 0 {
		payload["tools"] = p.transformResponsesTools(request.Tools)
		if request.ToolChoice != nil {
			payload["tool_choice"] = p.transformResponsesToolChoice(request.ToolChoice)
		}
	}

	if request.ResponseFormat != nil {
		payload["text"] = map[string]any{
			"format": request.ResponseFormat,
		}
	}

	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	return payload
}

func (p *Provider) transformResponsesInput(messages []types.Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch m := msg.(type) {
		case *types.AssistantMessage:
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					items = append(items, responsesMessageItem(types.RoleAssistant, m.Content))
				}
				for _, tc := range m.ToolCalls {
					items = append(items, responsesFunctionCallItem(tc))
				}
				continue
			}
			items = append(items, responsesMessageItem(types.RoleAssistant, m.Content))
		case *types.ToolResultMessage:
			items = append(items, map[string]any{
				"type":    responsesItemFunctionCallOutput,
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		default:
			items = append(items, responsesMessageItem(msg.GetRole(), msg.GetContent()))
		}
	}
	return items
}

func responsesMessageItem(role types.Role, content any) map[string]any {
	return map[string]any{
		"type":    responsesItemMessage,
		"role":    string(role),
		"content": responsesMessageContent(content),
	}
}

func responsesMessageContent(content any) any {
	parts, ok := content.([]types.MessagePart)
	if !ok {
		return content
	}

	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, map[string]any{
				"type": responsesContentInputText,
				"text": part.Text,
			})
		case "image":
			item := map[string]any{
				"type": responsesContentInputImage,
			}
			switch data := part.Data.(type) {
			case string:
				item["image_url"] = data
			case map[string]any:
				for k, v := range data {
					item[k] = v
				}
			default:
				item["image_url"] = data
			}
			out = append(out, item)
		}
	}
	return out
}

func responsesFunctionCallItem(tc types.ToolCall) map[string]any {
	callID := tc.ID
	args := tc.Arguments
	if tc.Function != nil {
		callID = tc.ID
	}

	arguments := ""
	if tc.Function != nil && tc.Function.Arguments != "" {
		arguments = tc.Function.Arguments
	} else if len(args) > 0 {
		if b, err := pool.Marshal(args); err == nil {
			arguments = string(b)
			pool.Return(b)
		}
	}

	name := tc.Name
	if name == "" && tc.Function != nil {
		name = tc.Function.Name
	}

	return map[string]any{
		"type":      responsesItemFunctionCall,
		"id":        callID,
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
}

func (p *Provider) transformResponsesTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := tool.Name
		description := tool.Description
		parameters := tool.InputSchema
		strict := false
		if tool.Function != nil {
			if tool.Function.Name != "" {
				name = tool.Function.Name
			}
			if tool.Function.Description != "" {
				description = tool.Function.Description
			}
			if tool.Function.Parameters != nil {
				parameters = tool.Function.Parameters
			}
		}

		out := map[string]any{
			"type":        "function",
			"name":        name,
			"description": description,
			"parameters":  parameters,
			"strict":      strict,
		}
		result = append(result, out)
	}
	return result
}

func (p *Provider) transformResponsesToolChoice(choice *types.ToolChoice) any {
	if choice == nil {
		return toolChoiceAuto
	}

	switch choice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return "auto"
	case types.ToolChoiceTypeAny:
		return "required"
	case types.ToolChoiceTypeSpecific:
		if choice.ToolName != "" {
			return map[string]any{
				"type": "function",
				"name": choice.ToolName,
			}
		}
	}
	return toolChoiceAuto
}

func (p *Provider) transformResponsesTextResponse(response *responsesResponse) *types.TextResponse {
	text := response.OutputText
	var toolCalls []types.ToolCall
	for _, item := range response.Output {
		switch item.Type {
		case responsesItemMessage:
			if text == "" {
				text += responsesOutputText(item.Content)
			}
		case responsesItemFunctionCall:
			toolCalls = append(toolCalls, responseFunctionCallToToolCall(item))
		}
	}

	return &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         text,
		ToolCalls:    toolCalls,
		FinishReason: responsesFinishReason(response, toolCalls),
		Usage:        response.Usage.toUsage(),
		Created:      time.Unix(response.CreatedAt, 0),
	}
}

func responsesOutputText(parts []responsesContentPart) string {
	var text string
	for _, part := range parts {
		switch part.Type {
		case responsesContentOutputText:
			text += part.Text
		case responsesContentRefusal:
			text += part.Refusal
		}
	}
	return text
}

func responseFunctionCallToToolCall(item responsesOutputItem) types.ToolCall {
	callID := item.CallID
	if callID == "" {
		callID = item.ID
	}

	argsMap := make(map[string]any)
	if item.Arguments != "" {
		_ = json.Unmarshal([]byte(item.Arguments), &argsMap)
	}

	return types.ToolCall{
		ID:        callID,
		Type:      "function",
		Name:      item.Name,
		Arguments: argsMap,
		Function: &types.ToolCallFunction{
			Name:      item.Name,
			Arguments: item.Arguments,
		},
	}
}

func responsesFinishReason(response *responsesResponse, toolCalls []types.ToolCall) types.FinishReason {
	if len(toolCalls) > 0 {
		return types.FinishReasonToolCalls
	}
	if response.IncompleteDetails != nil {
		switch response.IncompleteDetails.Reason {
		case "max_output_tokens":
			return types.FinishReasonLength
		case "content_filter":
			return types.FinishReasonContentFilter
		}
	}
	return types.FinishReasonStop
}

func (u responsesUsage) toUsage() *types.Usage {
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 {
		return nil
	}
	return &types.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func (p *Provider) parseResponsesStreamChunk(data []byte) (*types.TextChunk, error) {
	var event responsesStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	switch event.Type {
	case responsesEventOutputTextDelta:
		return &types.TextChunk{
			Text: event.Delta,
			Delta: &types.ChunkDelta{
				Content: event.Delta,
			},
		}, nil
	case responsesEventCompleted, responsesEventIncomplete:
		if event.Response == nil {
			return nil, nil
		}
		resp := p.transformResponsesTextResponse(event.Response)
		reason := resp.FinishReason
		return &types.TextChunk{
			ID:           resp.ID,
			Model:        resp.Model,
			ToolCalls:    resp.ToolCalls,
			FinishReason: &reason,
			Usage:        resp.Usage,
		}, nil
	case responsesEventFailed:
		if event.Response != nil && event.Response.Error != nil {
			return &types.TextChunk{
				Error: p.ProviderError(event.Response.Error.Message, event.Response.Error.Code),
			}, nil
		}
	}

	return nil, nil
}

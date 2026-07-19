package openai

import (
	"encoding/json"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

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

	argsMap, parseErrMsg := types.ParseToolArgs(item.Arguments, map[string]any{})

	toolCall := types.ToolCall{
		ID:        callID,
		Type:      "function",
		Name:      item.Name,
		Arguments: argsMap,
		Function: &types.ToolCallFunction{
			Name:      item.Name,
			Arguments: item.Arguments,
		},
	}
	toolCall.MarkArgsError(parseErrMsg)
	return toolCall
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
	case responsesEventOutputItemAdded:
		if event.Item == nil || event.Item.Type != responsesItemFunctionCall {
			return nil, nil
		}
		toolCall := responseFunctionCallToToolCall(*event.Item)
		return responsesToolCallChunk(event.ItemID, event.responseModel(), toolCall), nil
	case responsesEventFunctionArgsDelta:
		toolCall := types.ToolCall{
			ID:   event.ItemID,
			Type: "function",
			Function: &types.ToolCallFunction{
				Arguments: event.Delta,
			},
		}
		return responsesToolCallChunk(event.ItemID, event.responseModel(), toolCall), nil
	case responsesEventReasoningDelta:
		thinking := &types.Thinking{Content: event.Delta}
		return &types.TextChunk{
			ID:       event.ItemID,
			Model:    event.responseModel(),
			Thinking: thinking,
			Delta:    &types.ChunkDelta{Thinking: thinking},
		}, nil
	case responsesEventReasoningDone:
		thinking := &types.Thinking{Signature: event.ItemID, Provider: "openai"}
		return &types.TextChunk{
			ID:       event.ItemID,
			Model:    event.responseModel(),
			Thinking: thinking,
			Delta:    &types.ChunkDelta{Thinking: thinking},
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

func responsesToolCallChunk(itemID, model string, toolCall types.ToolCall) *types.TextChunk {
	return &types.TextChunk{
		ID:        itemID,
		Model:     model,
		ToolCall:  &toolCall,
		ToolCalls: []types.ToolCall{toolCall},
		Delta:     &types.ChunkDelta{ToolCalls: []types.ToolCall{toolCall}},
	}
}

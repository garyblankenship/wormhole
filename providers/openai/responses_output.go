package openai

import (
	"encoding/json"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Provider) transformResponsesTextResponse(response *responsesResponse) *types.TextResponse {
	return p.transformResponsesTextResponseWithState(response, newResponsesAnnotationState())
}

func (p *Provider) transformResponsesTextResponseWithState(response *responsesResponse, state *responsesAnnotationState) *types.TextResponse {
	text := response.OutputText
	var toolCalls []types.ToolCall
	var summary string
	for _, item := range response.Output {
		switch item.Type {
		case responsesItemMessage:
			if response.OutputText == "" {
				text += responsesOutputText(item.Content)
			}
		case "reasoning":
			for _, part := range item.Summary {
				if part.Type == "summary_text" {
					summary += part.Text
				}
			}
		case responsesItemFunctionCall:
			toolCalls = append(toolCalls, responseFunctionCallToToolCall(item))
		}
	}

	result := &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         text,
		ToolCalls:    toolCalls,
		FinishReason: responsesFinishReason(response, toolCalls),
		Usage:        response.Usage.toUsage(),
		Created:      time.Unix(response.CreatedAt, 0),
		Metadata:     map[string]any{"provider_usage": response.Usage.Raw},
	}
	if summary != "" {
		result.Thinking = &types.Thinking{Content: summary, Provider: "openai"}
	}
	state.normalizeResponse(response, result)
	return result
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
	result := &types.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.InputTokensDetails != nil {
		result.CacheReadTokens = u.InputTokensDetails.CachedTokens
	}
	if u.OutputTokensDetails != nil {
		result.ReasoningTokens = u.OutputTokensDetails.ReasoningTokens
	}
	return result
}

func (p *Provider) parseResponsesStreamChunk(data []byte) (*types.TextChunk, error) {
	return p.parseResponsesStreamChunkWithState(data, newResponsesAnnotationState())
}

func (p *Provider) parseResponsesStreamChunkWithState(data []byte, state *responsesAnnotationState) (*types.TextChunk, error) {
	var event responsesStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	switch event.Type {
	case "response.output_text.annotation.added":
		snapshot := &types.TextResponse{}
		state.normalize(event.Annotation, event.OutputIndex, event.ContentIndex, event.AnnotationIndex, snapshot)
		return &types.TextChunk{Sources: snapshot.Sources, Citations: snapshot.Citations, Warnings: snapshot.Warnings, Metadata: snapshot.Metadata}, nil
	case responsesEventOutputTextDelta:
		return &types.TextChunk{
			Text: event.Delta,
			Delta: &types.ChunkDelta{
				Content: event.Delta,
			},
		}, nil
	case responsesEventOutputItemAdded, responsesEventFunctionArgsDelta:
		// Like Chat Completions, expose only complete tool calls. The terminal
		// response contains authoritative arguments and call IDs; forwarding
		// fragments in any carrier would duplicate them for stream consumers.
		return nil, nil
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
		// The terminal response is a complete snapshot, using IDs assigned
		// by prior additions even when traversal order differs.
		snapshot := &responsesAnnotationState{sources: state.sources, seen: map[string]bool{}, snapshotSources: map[string]bool{}}
		resp := p.transformResponsesTextResponseWithState(event.Response, snapshot)
		reason := resp.FinishReason
		return &types.TextChunk{
			ID:           resp.ID,
			Model:        resp.Model,
			ToolCalls:    resp.ToolCalls,
			FinishReason: &reason,
			Usage:        resp.Usage,
			Metadata:     resp.Metadata,
			Sources:      resp.Sources, Citations: resp.Citations, Warnings: resp.Warnings,
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

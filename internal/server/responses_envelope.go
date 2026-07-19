package server

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

func completedResponsesEnvelope(resp *types.TextResponse, model string, customTools map[string]bool) responsesEnvelope {
	outputs := make([]responsesOutputItem, 0, 1+len(resp.ToolCalls))
	if resp.Text != "" || resp.Refusal != "" {
		outputs = append(outputs, completedMessageOutput("msg_"+resp.ID, resp.Text, resp.Refusal))
	}
	for _, call := range resp.ToolCalls {
		outputs = append(outputs, completedToolOutput(call, len(outputs), customTools[call.Name]))
	}
	status, incompleteDetails := responsesStatus(resp.FinishReason)
	return responsesEnvelope{ID: "resp_" + resp.ID, Object: "response", CreatedAt: resp.Created.Unix(), Status: status, Model: model,
		Output: outputs, Usage: toResponsesUsage(resp.Usage), Error: nil, IncompleteDetails: incompleteDetails}
}

func responsesStatus(reason types.FinishReason) (string, any) {
	switch reason {
	case types.FinishReasonLength:
		return "incomplete", map[string]string{"reason": "max_output_tokens"}
	case types.FinishReasonContentFilter:
		return "incomplete", map[string]string{"reason": "content_filter"}
	default:
		return "completed", nil
	}
}

func completedMessageOutput(id, text, refusal string) responsesOutputItem {
	content := make([]responsesOutputText, 0, 2)
	if text != "" {
		content = append(content, responsesOutputText{Type: "output_text", Text: text, Annotations: []any{}})
	}
	if refusal != "" {
		content = append(content, responsesOutputText{Type: "refusal", Refusal: refusal})
	}
	return responsesOutputItem{ID: id, Type: "message", Status: "completed", Role: "assistant", Content: content}
}

func completedToolOutput(call types.ToolCall, index int, custom bool) responsesOutputItem {
	arguments := "{}"
	if call.Function != nil {
		arguments = call.Function.Arguments
	}
	if arguments == "" && call.Arguments != nil {
		if encoded, err := json.Marshal(call.Arguments); err == nil {
			arguments = string(encoded)
		}
	}
	callID := call.ID
	if callID == "" {
		callID = fmt.Sprintf("call_%d", index)
	}
	if custom {
		var payload struct {
			Input string `json:"input"`
		}
		_ = json.Unmarshal([]byte(arguments), &payload)
		return responsesOutputItem{ID: fmt.Sprintf("ctc_%d", index), Type: "custom_tool_call", Status: "completed", CallID: callID, Name: call.Name, Input: payload.Input}
	}
	return responsesOutputItem{ID: fmt.Sprintf("fc_%d", index), Type: "function_call", Status: "completed", CallID: callID, Name: call.Name, Arguments: arguments}
}

func toResponsesUsage(usage *types.Usage) *responsesUsage {
	if usage == nil {
		return nil
	}
	return &responsesUsage{
		InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		InputTokenDetails:  responsesInputTokenDetails{CachedTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens},
		OutputTokenDetails: responsesOutputTokenDetails{ReasoningTokens: usage.ReasoningTokens},
	}
}

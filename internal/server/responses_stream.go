package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

func (p *proxy) streamResponses(w http.ResponseWriter, r *http.Request, execution responsesExecution) {
	model := execution.model
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported", "api_error")
		return
	}
	stream, err := execution.builder.Stream(r.Context())
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	responseID := fmt.Sprintf("resp_wh-%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_wh-%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()
	outputIndex := 0
	messageOpened := false
	textOpened := false
	refusalOpened := false
	textContentIndex := -1
	refusalContentIndex := -1
	nextContentIndex := 0
	var text strings.Builder
	var refusal strings.Builder
	toolDeltas := newStreamToolState()
	tools := map[int]ChatToolCall{}
	var usage *types.Usage
	var finishReason types.FinishReason

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	sse := responsesSSEWriter{w: w}
	sse.write(responsesEvent{Type: "response.created", Response: &responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: "in_progress", Model: model, Output: []responsesOutputItem{}, Error: nil, IncompleteDetails: nil}})
	flusher.Flush()
	openMessage := func() {
		if messageOpened {
			return
		}
		messageOpened = true
		index := outputIndex
		item := responsesOutputItem{ID: messageID, Type: "message", Status: "in_progress", Role: "assistant", Content: []responsesOutputText{}}
		sse.write(responsesEvent{Type: "response.output_item.added", OutputIndex: &index, Item: &item})
		outputIndex++
	}

	for chunk := range stream {
		if chunk.Error != nil {
			writeResponsesFailure(&sse, responseID, model, createdAt, chunk.Error)
			flusher.Flush()
			return
		}
		if content := chunk.Content(); content != "" {
			openMessage()
			if !textOpened {
				textOpened = true
				textContentIndex = nextContentIndex
				nextContentIndex++
				index := 0
				part := responsesOutputText{Type: "output_text", Text: "", Annotations: []any{}}
				sse.write(responsesEvent{Type: "response.content_part.added", OutputIndex: &index, ContentIndex: &textContentIndex, ItemID: messageID, Part: &part})
			}
			text.WriteString(content)
			index := 0
			sse.write(responsesEvent{Type: "response.output_text.delta", OutputIndex: &index, ContentIndex: &textContentIndex, ItemID: messageID, Delta: content})
		}
		if chunk.Refusal != "" {
			openMessage()
			if !refusalOpened {
				refusalOpened = true
				refusalContentIndex = nextContentIndex
				nextContentIndex++
				index := 0
				part := responsesOutputText{Type: "refusal", Refusal: ""}
				sse.write(responsesEvent{Type: "response.content_part.added", OutputIndex: &index, ContentIndex: &refusalContentIndex, ItemID: messageID, Part: &part})
			}
			refusal.WriteString(chunk.Refusal)
			index := 0
			sse.write(responsesEvent{Type: "response.refusal.delta", OutputIndex: &index, ContentIndex: &refusalContentIndex, ItemID: messageID, Delta: chunk.Refusal})
		}
		for _, delta := range toolDeltas.delta(chunk) {
			if delta.Index == nil {
				continue
			}
			current := tools[*delta.Index]
			if delta.ID != "" {
				current.ID = delta.ID
			}
			if delta.Function.Name != "" {
				current.Function.Name = delta.Function.Name
			}
			current.Function.Arguments += delta.Function.Arguments
			tools[*delta.Index] = current
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		flusher.Flush()
	}

	outputs := make([]responsesOutputItem, 0, outputIndex+len(tools))
	if messageOpened {
		index := 0
		finalText := text.String()
		finalRefusal := refusal.String()
		item := responsesOutputItem{ID: messageID, Type: "message", Status: "completed", Role: "assistant", Content: make([]responsesOutputText, nextContentIndex)}
		if textOpened {
			item.Content[textContentIndex] = responsesOutputText{Type: "output_text", Text: finalText, Annotations: []any{}}
		}
		if refusalOpened {
			item.Content[refusalContentIndex] = responsesOutputText{Type: "refusal", Refusal: finalRefusal}
		}
		outputs = append(outputs, item)
		if textOpened {
			part := item.Content[textContentIndex]
			sse.write(responsesEvent{Type: "response.output_text.done", OutputIndex: &index, ContentIndex: &textContentIndex, ItemID: messageID, Text: finalText})
			sse.write(responsesEvent{Type: "response.content_part.done", OutputIndex: &index, ContentIndex: &textContentIndex, ItemID: messageID, Part: &part})
		}
		if refusalOpened {
			part := item.Content[refusalContentIndex]
			sse.write(responsesEvent{Type: "response.refusal.done", OutputIndex: &index, ContentIndex: &refusalContentIndex, ItemID: messageID, Refusal: finalRefusal})
			sse.write(responsesEvent{Type: "response.content_part.done", OutputIndex: &index, ContentIndex: &refusalContentIndex, ItemID: messageID, Part: &part})
		}
		sse.write(responsesEvent{Type: "response.output_item.done", OutputIndex: &index, Item: &item})
	}
	for index := 0; index < len(tools); index++ {
		call := tools[index]
		item := completedToolOutput(types.ToolCall{ID: call.ID, Name: call.Function.Name, Function: &types.ToolCallFunction{Name: call.Function.Name, Arguments: call.Function.Arguments}}, outputIndex, execution.customTools[call.Function.Name])
		idx := outputIndex
		outputs = append(outputs, item)
		added := item
		added.Status = "in_progress"
		added.Arguments = ""
		added.Input = ""
		sse.write(responsesEvent{Type: "response.output_item.added", OutputIndex: &idx, Item: &added})
		if item.Type == "custom_tool_call" {
			sse.write(responsesEvent{Type: "response.custom_tool_call_input.delta", OutputIndex: &idx, ItemID: item.ID, Delta: item.Input})
			sse.write(responsesEvent{Type: "response.custom_tool_call_input.done", OutputIndex: &idx, ItemID: item.ID, Input: item.Input})
		} else {
			sse.write(responsesEvent{Type: "response.function_call_arguments.delta", OutputIndex: &idx, ItemID: item.ID, Delta: item.Arguments})
			sse.write(responsesEvent{Type: "response.function_call_arguments.done", OutputIndex: &idx, ItemID: item.ID, Arguments: item.Arguments})
		}
		sse.write(responsesEvent{Type: "response.output_item.done", OutputIndex: &idx, Item: &item})
		outputIndex++
	}
	status, incompleteDetails := responsesStatus(finishReason)
	completed := responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: status, Model: model, Output: outputs, Usage: toResponsesUsage(usage), Error: nil, IncompleteDetails: incompleteDetails}
	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	}
	sse.write(responsesEvent{Type: eventType, Response: &completed})
	flusher.Flush()
}

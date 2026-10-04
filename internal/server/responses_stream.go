package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type responsesLiveTool struct {
	index     int
	item      responsesOutputItem
	arguments strings.Builder
}

func (p *proxy) streamResponses(w http.ResponseWriter, r *http.Request, execution responsesExecution) {
	model := execution.model
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported", "api_error")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	stream, err := execution.builder.Stream(ctx)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	responseID := fmt.Sprintf("resp_wh-%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_wh-%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()
	outputIndex := 0
	messageIndex := -1
	textIndex, refusalIndex := -1, -1
	nextContentIndex := 0
	var text, refusal strings.Builder
	toolDeltas := newStreamToolState()
	tools := map[int]*responsesLiveTool{}
	toolOrder := []int{}
	var usage *types.Usage
	var finishReason types.FinishReason
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	sse := responsesSSEWriter{w: w, cancel: cancel, ctx: ctx}
	sse.write(responsesEvent{Type: "response.created", Response: &responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: "in_progress", Model: model, Output: []responsesOutputItem{}, Error: nil, IncompleteDetails: nil}})
	if sse.err != nil {
		return
	}
	flusher.Flush()
	openMessage := func() {
		if messageIndex >= 0 {
			return
		}
		messageIndex = outputIndex
		outputIndex++
		item := responsesOutputItem{ID: messageID, Type: "message", Status: "in_progress", Role: "assistant", Content: []responsesOutputText{}}
		sse.write(responsesEvent{Type: "response.output_item.added", OutputIndex: &messageIndex, Item: &item})
	}
	for {
		if ctx.Err() != nil {
			return
		}
		var chunk types.TextChunk
		select {
		case <-ctx.Done():
			return
		case next, open := <-stream:
			if !open {
				goto complete
			}
			chunk = next
		}
		if chunk.Error != nil {
			writeResponsesFailure(&sse, responseID, model, createdAt, chunk.Error)
			if sse.err == nil {
				flusher.Flush()
			}
			return
		}
		if content := chunk.Content(); content != "" {
			openMessage()
			if textIndex < 0 {
				textIndex = nextContentIndex
				nextContentIndex++
				part := responsesOutputText{Type: "output_text", Text: "", Annotations: []any{}}
				sse.write(responsesEvent{Type: "response.content_part.added", OutputIndex: &messageIndex, ContentIndex: &textIndex, ItemID: messageID, Part: &part})
			}
			text.WriteString(content)
			sse.write(responsesEvent{Type: "response.output_text.delta", OutputIndex: &messageIndex, ContentIndex: &textIndex, ItemID: messageID, Delta: content})
		}
		if chunk.Refusal != "" {
			openMessage()
			if refusalIndex < 0 {
				refusalIndex = nextContentIndex
				nextContentIndex++
				part := responsesOutputText{Type: "refusal", Refusal: ""}
				sse.write(responsesEvent{Type: "response.content_part.added", OutputIndex: &messageIndex, ContentIndex: &refusalIndex, ItemID: messageID, Part: &part})
			}
			refusal.WriteString(chunk.Refusal)
			sse.write(responsesEvent{Type: "response.refusal.delta", OutputIndex: &messageIndex, ContentIndex: &refusalIndex, ItemID: messageID, Delta: chunk.Refusal})
		}
		for _, delta := range toolDeltas.delta(chunk) {
			if delta.Index == nil {
				continue
			}
			live := tools[*delta.Index]
			if live == nil {
				item := completedToolOutput(types.ToolCall{ID: delta.ID, Name: delta.Function.Name}, outputIndex, execution.customTools[delta.Function.Name])
				item.Status = "in_progress"
				item.Arguments = ""
				item.Input = ""
				live = &responsesLiveTool{index: outputIndex, item: item}
				tools[*delta.Index] = live
				toolOrder = append(toolOrder, *delta.Index)
				outputIndex++
				sse.write(responsesEvent{Type: "response.output_item.added", OutputIndex: &live.index, Item: &live.item})
			}
			live.arguments.WriteString(delta.Function.Arguments)
			if live.item.Type == "custom_tool_call" {
				input := partialCustomToolInput(live.arguments.String())
				if strings.HasPrefix(input, live.item.Input) && len(input) > len(live.item.Input) {
					sse.write(responsesEvent{Type: "response.custom_tool_call_input.delta", OutputIndex: &live.index, ItemID: live.item.ID, Delta: input[len(live.item.Input):]})
					live.item.Input = input
				}
			} else if delta.Function.Arguments != "" {
				sse.write(responsesEvent{Type: "response.function_call_arguments.delta", OutputIndex: &live.index, ItemID: live.item.ID, Delta: delta.Function.Arguments})
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		if sse.err != nil {
			return
		}
		flusher.Flush()
	}
complete:
	if ctx.Err() != nil || sse.err != nil {
		return
	}
	outputs := make([]responsesOutputItem, outputIndex)
	if messageIndex >= 0 {
		item := responsesOutputItem{ID: messageID, Type: "message", Status: "completed", Role: "assistant", Content: make([]responsesOutputText, nextContentIndex)}
		if textIndex >= 0 {
			part := responsesOutputText{Type: "output_text", Text: text.String(), Annotations: []any{}}
			item.Content[textIndex] = part
			sse.write(responsesEvent{Type: "response.output_text.done", OutputIndex: &messageIndex, ContentIndex: &textIndex, ItemID: messageID, Text: text.String()})
			sse.write(responsesEvent{Type: "response.content_part.done", OutputIndex: &messageIndex, ContentIndex: &textIndex, ItemID: messageID, Part: &part})
		}
		if refusalIndex >= 0 {
			part := responsesOutputText{Type: "refusal", Refusal: refusal.String()}
			item.Content[refusalIndex] = part
			sse.write(responsesEvent{Type: "response.refusal.done", OutputIndex: &messageIndex, ContentIndex: &refusalIndex, ItemID: messageID, Refusal: refusal.String()})
			sse.write(responsesEvent{Type: "response.content_part.done", OutputIndex: &messageIndex, ContentIndex: &refusalIndex, ItemID: messageID, Part: &part})
		}
		outputs[messageIndex] = item
		sse.write(responsesEvent{Type: "response.output_item.done", OutputIndex: &messageIndex, Item: &item})
	}
	for _, key := range toolOrder {
		live := tools[key]
		live.item.Status = "completed"
		if live.item.Type == "custom_tool_call" {
			sse.write(responsesEvent{Type: "response.custom_tool_call_input.done", OutputIndex: &live.index, ItemID: live.item.ID, Input: live.item.Input})
		} else {
			live.item.Arguments = live.arguments.String()
			sse.write(responsesEvent{Type: "response.function_call_arguments.done", OutputIndex: &live.index, ItemID: live.item.ID, Arguments: live.item.Arguments})
		}
		outputs[live.index] = live.item
		sse.write(responsesEvent{Type: "response.output_item.done", OutputIndex: &live.index, Item: &live.item})
	}
	if ctx.Err() != nil || sse.err != nil {
		return
	}
	status, incompleteDetails := responsesStatus(finishReason)
	completed := responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: status, Model: model, Output: outputs, Usage: toResponsesUsage(usage), Error: nil, IncompleteDetails: incompleteDetails}
	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	}
	sse.write(responsesEvent{Type: eventType, Response: &completed})
	if sse.err == nil {
		flusher.Flush()
	}
}

// Custom tools use a JSON wrapper on the provider path. Expose decoded input
// as soon as a complete character is available, withholding incomplete escapes.
func partialCustomToolInput(raw string) string {
	var full struct {
		Input string `json:"input"`
	}
	if json.Unmarshal([]byte(raw), &full) == nil {
		return full.Input
	}
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return ""
	}
	raw = strings.TrimSpace(raw[1:])
	if !strings.HasPrefix(raw, `"input"`) {
		return ""
	}
	raw = strings.TrimSpace(raw[len(`"input"`):])
	if !strings.HasPrefix(raw, ":") {
		return ""
	}
	raw = strings.TrimSpace(raw[1:])
	if !strings.HasPrefix(raw, `"`) {
		return ""
	}
	for end := len(raw); end >= 1; end-- {
		candidate := raw[:end]
		if decoded, err := strconv.Unquote(candidate); err == nil {
			return decoded
		}
		if decoded, err := strconv.Unquote(candidate + `"`); err == nil {
			return decoded
		}
	}
	return ""
}

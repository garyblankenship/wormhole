package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
)

func (p *proxy) streamChat(w http.ResponseWriter, r *http.Request, builder *wormhole.TextRequestBuilder, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported",
			"Streaming not supported", "api_error")
		return
	}

	stream, err := builder.Stream(r.Context())
	if err != nil {
		p.logger.Error("stream creation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	id := fmt.Sprintf("wh-%d", time.Now().UnixNano())
	toolState := newStreamToolState()
	committed := false

	for chunk := range stream {
		if chunk.Error != nil {
			p.logger.Error("stream chunk error", "error", types.SafeErrorValue(chunk.Error))
			if !committed {
				status, errType, clientMsg := upstreamErrorStatus(chunk.Error)
				writeError(w, status, "upstream_error", clientMsg, errType)
				return
			}
			writeStreamError(w, flusher, chunk.Error)
			return
		}

		if !committed {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			flusher.Flush()
			committed = true
		}

		delta := &ChatMessage{Role: "assistant", Content: chunk.Content(), Refusal: chunk.Refusal}
		if tcs := toolState.delta(chunk); len(tcs) > 0 {
			delta.ToolCalls = tcs
		}
		chunkResp := ChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []ChatChoice{{
				Index: 0,
				Delta: delta,
			}},
		}

		if chunk.FinishReason != nil {
			fr := string(normalizedFinishReason(*chunk.FinishReason))
			chunkResp.Choices[0].FinishReason = &fr
		}
		if chunk.Usage != nil {
			chunkResp.Usage = toChatUsage(chunk.Usage)
		}

		data, marshalErr := json.Marshal(chunkResp)
		if marshalErr != nil {
			p.logger.Error("failed to marshal chunk", "error", types.SafeErrorValue(marshalErr))
			break
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			p.logger.Error("failed to write stream chunk", "error", types.SafeErrorValue(err))
			break
		}
		flusher.Flush()
	}

	if !committed {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		p.logger.Error("failed to write stream terminator", "error", types.SafeErrorValue(err))
		return
	}
	flusher.Flush()
}

func toChatUsage(usage *types.Usage) *ChatUsage {
	if usage == nil {
		return nil
	}
	out := &ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if usage.CacheReadTokens != 0 || usage.CacheWriteTokens != 0 {
		out.PromptTokensDetails = &ChatPromptTokenDetails{
			CachedTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		}
	}
	if usage.ReasoningTokens != 0 {
		out.CompletionTokensDetails = &ChatCompletionTokenDetails{ReasoningTokens: usage.ReasoningTokens}
	}
	return out
}

func normalizedFinishReason(reason types.FinishReason) types.FinishReason {
	switch reason {
	case types.FinishReasonStop, types.FinishReasonLength, types.FinishReasonToolCalls,
		types.FinishReasonContentFilter, types.FinishReasonOther:
		return reason
	default:
		return types.FinishReasonOther
	}
}

func writeStreamError(w http.ResponseWriter, flusher http.Flusher, err error) {
	_, errType, clientMsg := upstreamErrorStatus(err)
	payload := ErrorResponse{
		Error: ErrorDetail{
			Message: clientMsg,
			Type:    errType,
			Code:    "upstream_error",
		},
	}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

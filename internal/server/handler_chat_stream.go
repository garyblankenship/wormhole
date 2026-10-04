package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *proxy) streamChat(w http.ResponseWriter, r *http.Request, builder *wormhole.TextRequestBuilder, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported",
			"Streaming not supported", "api_error")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stream, err := builder.Stream(ctx)
	if err != nil {
		p.logger.Error("stream creation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(model))
		writeUpstreamError(w, err)
		return
	}

	id := fmt.Sprintf("wh-%d", time.Now().UnixNano())
	toolState := newStreamToolState()
	committed := false
	roleSent := false

	for {
		var chunk types.TextChunk
		select {
		case <-ctx.Done():
			return
		case next, open := <-stream:
			if !open {
				goto finished
			}
			chunk = next
		}
		if ctx.Err() != nil {
			return
		}
		if chunk.Error != nil {
			p.logger.Error("stream chunk error", "error", types.SafeErrorValue(chunk.Error))
			if !committed {
				writeUpstreamError(w, chunk.Error)
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

		delta := &ChatMessage{Content: chunk.Content(), Refusal: chunk.Refusal}
		if !roleSent {
			delta.Role = "assistant"
			roleSent = true
		}
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
			return
		}
		if err := writeChatFrame(w, fmt.Sprintf("data: %s\n\n", data)); err != nil {
			p.logger.Error("failed to write stream chunk", "error", types.SafeErrorValue(err))
			return
		}
		flusher.Flush()
	}

finished:
	if ctx.Err() != nil {
		return
	}
	if !committed {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	if err := writeChatFrame(w, "data: [DONE]\n\n"); err != nil {
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
	if err := writeChatFrame(w, fmt.Sprintf("data: %s\n\n", data)); err != nil {
		return
	}
	flusher.Flush()
}

func writeChatFrame(w http.ResponseWriter, frame string) error {
	n, err := w.Write([]byte(frame))
	if err == nil && n != len(frame) {
		return io.ErrShortWrite
	}
	return err
}

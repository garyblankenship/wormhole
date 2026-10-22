package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *proxy) streamChat(w http.ResponseWriter, r *http.Request, builder *wormhole.TextRequestBuilder, model string, extensions ...*WormholeRequest) {
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

	var extension *WormholeRequest
	if len(extensions) > 0 {
		extension = extensions[0]
	}
	state := newChatStreamState(model, extension)

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
			if !state.committed {
				writeUpstreamError(w, chunk.Error)
				return
			}
			writeStreamError(w, flusher, chunk.Error)
			return
		}

		state.absorb(chunk)
		if !state.committed {
			state.commit(w, flusher)
		}
		if !state.frame(p, w, flusher, chunk) {
			return
		}
	}

finished:
	if ctx.Err() != nil {
		return
	}
	state.finish(p, w, flusher)
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

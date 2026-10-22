package server

import (
	"strconv"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func chatCompletionResponse(resp *types.TextResponse, model string, extension *WormholeRequest, now time.Time) ChatCompletionResponse {
	id := resp.ID
	if id == "" {
		id = strconv.FormatInt(now.UnixNano(), 10)
	}
	created := resp.Created
	if created.IsZero() {
		created = now
	}

	fr := string(normalizedFinishReason(resp.FinishReason))
	msg := &ChatMessage{Role: "assistant", Content: resp.Text, Refusal: resp.Refusal, Annotations: nativeChatAnnotations(resp.Citations)}
	if len(resp.ToolCalls) > 0 {
		msg.ToolCalls = fromWormholeToolCalls(resp.ToolCalls)
	}
	out := ChatCompletionResponse{
		ID:      "wh-" + id,
		Object:  "chat.completion",
		Created: created.Unix(),
		Model:   model,
		Choices: []ChatChoice{{Index: 0, Message: msg, FinishReason: &fr}},
	}
	if resp.Usage != nil {
		out.Usage = toChatUsage(resp.Usage)
	}
	out.Wormhole = extensionSnapshot(extension, resp.ResponseMetadata, resp.Usage, resp.Sources, resp.Citations, resp.Warnings)
	return out
}

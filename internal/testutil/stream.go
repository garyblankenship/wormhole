package testutil

import (
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// MergeTextChunks combines streamed chunks for test assertions.
func MergeTextChunks(chunks []types.TextChunk) *types.TextResponse {
	var text strings.Builder
	var toolCalls []types.ToolCall
	var usage *types.Usage
	var finishReason types.FinishReason
	var id, model string

	for _, chunk := range chunks {
		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Text != "" {
			text.WriteString(chunk.Text)
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		if chunk.Usage != nil && !chunk.Usage.IsZero() {
			usage = chunk.Usage
		}
		if chunk.ToolCall != nil {
			toolCalls = append(toolCalls, *chunk.ToolCall)
		}
		if len(chunk.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.ToolCalls...)
		}
	}

	return &types.TextResponse{
		ID:           id,
		Model:        model,
		Text:         text.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}
}

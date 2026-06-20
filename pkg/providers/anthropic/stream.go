package anthropic

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// parseStreamChunk parses a streaming chunk
func (p *Provider) parseStreamChunk(data []byte) (*types.StreamChunk, error) {
	// First, determine the event type
	var baseEvent streamEvent
	if err := json.Unmarshal(data, &baseEvent); err != nil {
		return nil, err
	}

	chunk := &types.StreamChunk{}

	switch baseEvent.Type {
	case "message_start":
		var event messageStartEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		chunk.ID = event.Message.ID
		chunk.Model = event.Message.Model

	case "content_block_start":
		var event contentBlockStartEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		// Only tool_use blocks open a tool call; text/thinking blocks are no-ops
		// here (their content arrives via content_block_delta).
		if event.ContentBlock.Type == "tool_use" {
			chunk.Delta = &types.ChunkDelta{
				ToolCalls: []types.ToolCall{{
					ID:   event.ContentBlock.ID,
					Type: "tool_use",
					Name: event.ContentBlock.Name,
					Function: &types.ToolCallFunction{
						Name:      event.ContentBlock.Name,
						Arguments: "",
					},
				}},
			}
		}

	case "content_block_delta":
		var event contentBlockDeltaEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Delta.Type == "text_delta" {
			chunk.Delta = &types.ChunkDelta{
				Content: event.Delta.Text,
			}
		} else if event.Delta.Type == "thinking_delta" {
			thinking := &types.Thinking{Content: event.Delta.Thinking}
			chunk.Thinking = thinking
			chunk.Delta = &types.ChunkDelta{Thinking: thinking}
		} else if event.Delta.Type == "signature_delta" {
			thinking := &types.Thinking{Signature: event.Delta.Signature}
			chunk.Thinking = thinking
			chunk.Delta = &types.ChunkDelta{Thinking: thinking}
		} else if event.Delta.Type == "input_json_delta" {
			// Tool-call argument fragment; carries no id/name (continuation).
			chunk.Delta = &types.ChunkDelta{
				ToolCalls: []types.ToolCall{{
					Function: &types.ToolCallFunction{
						Arguments: event.Delta.PartialJSON,
					},
				}},
			}
		}

	case "message_delta":
		var event messageDeltaEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Delta.StopReason != "" {
			reason := p.mapStopReason(event.Delta.StopReason)
			chunk.FinishReason = &reason
		}
		if event.Delta.Usage.InputTokens > 0 || event.Delta.Usage.OutputTokens > 0 {
			chunk.Usage = p.convertUsage(event.Delta.Usage)
		}

	case "message_stop":
		// End of stream
		return nil, nil
	}

	return chunk, nil
}

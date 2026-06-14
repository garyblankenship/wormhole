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

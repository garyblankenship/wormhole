package ollama

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// transformTextResponse converts Ollama response to internal format
func (p *Provider) transformTextResponse(response *chatResponse) *types.TextResponse {
	// Generate a simple ID since Ollama doesn't provide one
	id := fmt.Sprintf("ollama_%d", time.Now().UnixNano())

	// Extract content as string
	var content string
	if str, ok := response.Message.Content.(string); ok {
		content = str
	} else {
		content = fmt.Sprintf("%v", response.Message.Content)
	}

	return &types.TextResponse{
		ID:           id,
		Model:        response.Model,
		Text:         content,
		FinishReason: p.mapFinishReason(response.DoneReason),
		Usage:        p.convertUsage(response),
		Created:      response.CreatedAt,
	}
}

// parseStreamChunk parses a streaming chunk from Ollama
func (p *Provider) parseStreamChunk(data []byte) (*types.TextChunk, error) {
	// Try to use unified streaming transformer if available
	if p.streamingTransformer != nil {
		return p.streamingTransformer.ParseChunk(data)
	}

	// Fall back to original implementation
	var response streamResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	// Generate a simple ID since Ollama doesn't provide one
	id := fmt.Sprintf("ollama_%d", time.Now().UnixNano())

	// Extract content as string
	var content string
	if str, ok := response.Message.Content.(string); ok {
		content = str
	} else {
		content = fmt.Sprintf("%v", response.Message.Content)
	}

	chunk := &types.StreamChunk{
		ID:    id,
		Model: response.Model,
		Delta: &types.ChunkDelta{
			Content: content,
		},
	}

	if response.Done {
		reason := p.mapFinishReason(response.DoneReason)
		chunk.FinishReason = &reason
	}

	if response.Done {
		chunk.Usage = p.convertUsage(&chatResponse{
			Model:              response.Model,
			CreatedAt:          response.CreatedAt,
			TotalDuration:      response.TotalDuration,
			LoadDuration:       response.LoadDuration,
			PromptEvalCount:    response.PromptEvalCount,
			PromptEvalDuration: response.PromptEvalDuration,
			EvalCount:          response.EvalCount,
			EvalDuration:       response.EvalDuration,
		})
	}

	return chunk, nil
}

// Helper functions

// mapFinishReason maps Ollama's done_reason to finish reason.
// Ollama returns done_reason values: "stop", "length", "load", "unload".
func (p *Provider) mapFinishReason(doneReason string) types.FinishReason {
	switch doneReason {
	case "stop":
		return types.FinishReasonStop
	case "length":
		return types.FinishReasonLength
	case "load", "unload":
		// Model load/unload — not a normal generation stop.
		return types.FinishReasonOther
	default:
		return types.FinishReasonOther
	}
}

// convertUsage converts Ollama response to usage info
func (p *Provider) convertUsage(response *chatResponse) *types.Usage {
	if response == nil {
		return nil
	}

	// Calculate token usage from Ollama's eval counts
	// Ollama provides prompt_eval_count and eval_count
	promptTokens := response.PromptEvalCount
	completionTokens := response.EvalCount
	totalTokens := promptTokens + completionTokens

	return &types.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

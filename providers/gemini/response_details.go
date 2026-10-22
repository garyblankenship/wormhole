package gemini

import (
	"context"
	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

func geminiReasoningWarnings(reasoning *types.Reasoning) []types.ResponseWarning {
	if reasoning == nil || reasoning.Enabled == nil {
		return nil
	}
	return []types.ResponseWarning{{Code: types.WarningAmbiguousReasoning, Message: "Gemini legacy Enabled controls thought visibility; use IncludeThoughts", Field: "reasoning.enabled"}}
}

// streamDetails adds request identity and metadata, preserving first-chunk fallback.
func (g *Gemini) streamDetails(ctx context.Context, in <-chan types.TextChunk, metadata *types.ResponseMetadata, request types.TextRequest) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		first := true
		for chunk := range in {
			chunk.Model = request.Model
			if first || chunk.IsDone() || chunk.Error != nil {
				chunk.ResponseMetadata = types.CloneResponseMetadata(metadata)
				chunk.Warnings = append(chunk.Warnings, geminiReasoningWarnings(request.Reasoning)...)
			}
			if chunk.Error != nil {
				chunk.Error = providers.WithResponseMetadata(chunk.Error, metadata)
			}
			first = false
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func providerUsage(usage *usageMetadata) any {
	if usage == nil {
		return nil
	}
	if usage.Raw != nil {
		return usage.Raw
	}
	return usage
}

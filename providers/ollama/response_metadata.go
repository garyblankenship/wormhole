package ollama

import (
	"context"
	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Provider) streamMetadata(ctx context.Context, in <-chan types.TextChunk, metadata *types.ResponseMetadata) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		first := true
		for chunk := range in {
			if first || chunk.IsDone() || chunk.Error != nil {
				chunk.ResponseMetadata = types.CloneResponseMetadata(metadata)
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

package openai

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Provider) validateV330Request(request types.TextRequest) error {
	if r := request.Reasoning; r != nil && r.Enabled != nil && !*r.Enabled && r.MaxTokens > 0 {
		return p.ValidationError("disabled reasoning conflicts with a positive thinking budget")
	}
	for _, message := range request.Messages {
		user, ok := message.(*types.UserMessage)
		if !ok {
			continue
		}
		for _, media := range user.Media {
			ref, ok := media.(*types.FileReferenceMedia)
			if !ok {
				continue
			}
			if err := ref.Validate(); err != nil {
				return err
			}
			if ref.Provider != p.Name() {
				return p.ValidationError("file reference belongs to a different provider")
			}
			if ref.ID == "" {
				return p.ValidationError("OpenAI file references require a file ID")
			}
			if ref.Kind != "document" && (!p.Config.UseResponsesAPI || ref.Kind != "image") {
				return p.ValidationError(fmt.Sprintf("unsupported file reference kind %q for this OpenAI route", ref.Kind))
			}
		}
	}
	return nil
}

// decorateStream attaches provider identity and response metadata in delivery order.
// The goroutine exits on upstream close or cancellation and owns out.
func (p *Provider) decorateStream(ctx context.Context, in <-chan types.TextChunk, metadata *types.ResponseMetadata) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		first := true
		for {
			var chunk types.TextChunk
			select {
			case <-ctx.Done():
				return
			case next, ok := <-in:
				if !ok {
					return
				}
				chunk = next
			}
			if ctx.Err() != nil {
				return
			}
			if chunk.IsDone() {
				chunk.Provider = p.Name()
			}
			if first || chunk.IsDone() || chunk.Error != nil {
				chunk.ResponseMetadata = types.CloneResponseMetadata(metadata)
			}
			first = false
			if chunk.Error != nil {
				chunk.Error = providers.WithResponseMetadata(chunk.Error, metadata)
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

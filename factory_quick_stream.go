package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/types"
)

func quickStreamWithClient(ctx context.Context, client *Wormhole, model, prompt string) (<-chan types.TextChunk, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	input, err := client.Text().Model(model).Prompt(prompt).Stream(streamCtx)
	if err != nil {
		cancel()
		_ = client.Close()
		return nil, err
	}
	output := make(chan types.TextChunk)
	// Sole closer; exits on upstream completion or caller cancellation.
	go func() {
		defer close(output)
		defer func() {
			cancel()
			_ = client.Close()
		}()
		for {
			select {
			case <-streamCtx.Done():
				return
			case chunk, ok := <-input:
				if !ok {
					return
				}
				select {
				case output <- chunk:
				case <-streamCtx.Done():
					return
				}
			}
		}
	}()
	return output, nil
}

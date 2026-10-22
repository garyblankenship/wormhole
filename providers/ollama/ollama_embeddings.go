package ollama

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// processEmbeddingsSequentially handles small batches sequentially
func (p *Provider) processEmbeddingsSequentially(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	embeddings := make([]types.Embedding, 0, len(request.Input))
	var metadata *types.ResponseMetadata

	for i, input := range request.Input {
		payload := &embeddingsRequest{
			Model:  request.Model,
			Prompt: input,
		}

		url := p.GetBaseURL() + "/api/embeddings"

		var response embeddingsResponse
		responseMetadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
		if err != nil {
			return nil, p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to get embedding for input %d", i), err)
		}

		metadata = responseMetadata
		embeddings = append(embeddings, types.Embedding{
			Index:     i,
			Embedding: response.Embedding,
		})
	}

	return &types.EmbeddingsResponse{
		ResponseMetadata: metadata,
		Provider:         p.Name(),
		Model:            request.Model,
		Embeddings:       embeddings,
		Usage:            nil, // Ollama doesn't provide usage info for embeddings
		Created:          time.Now(),
	}, nil
}

// processEmbeddingsConcurrently handles larger batches with controlled concurrency
func (p *Provider) processEmbeddingsConcurrently(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	// Limit concurrency to avoid overwhelming local Ollama instance.
	const maxConcurrency = 3
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	jobs := make(chan int)
	embeddings := make([]types.Embedding, len(request.Input))
	var metadata *types.ResponseMetadata
	var workers sync.WaitGroup

	// Workers exit on closed jobs or batch cancellation. The producer joins all
	// of them before returning, so no request outlives its embedding operation.
	for range maxConcurrency {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				payload := &embeddingsRequest{
					Model:  request.Model,
					Prompt: request.Input[index],
				}
				var response embeddingsResponse
				responseMetadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, p.GetBaseURL()+"/api/embeddings", payload, &response)
				if err != nil {
					cancel(p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to get embedding for input %d", index), err))
					return
				}
				embeddings[index] = types.Embedding{Index: index, Embedding: response.Embedding}
				if index == len(request.Input)-1 {
					metadata = responseMetadata
				}
			}
		})
	}

dispatch:
	for index := range request.Input {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}

	return &types.EmbeddingsResponse{
		ResponseMetadata: metadata,
		Provider:         p.Name(),
		Model:            request.Model,
		Embeddings:       embeddings,
		Usage:            nil, // Ollama doesn't provide usage info for embeddings
		Created:          time.Now(),
	}, nil
}

package ollama

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// processEmbeddingsSequentially handles small batches sequentially
func (p *Provider) processEmbeddingsSequentially(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	embeddings := make([]types.Embedding, 0, len(request.Input))

	for i, input := range request.Input {
		payload := &embeddingsRequest{
			Model:  request.Model,
			Prompt: input,
		}

		url := p.GetBaseURL() + "/api/embeddings"

		var response embeddingsResponse
		err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
		if err != nil {
			return nil, p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to get embedding for input %d", i), err)
		}

		embeddings = append(embeddings, types.Embedding{
			Index:     i,
			Embedding: response.Embedding,
		})
	}

	return &types.EmbeddingsResponse{
		Provider:   p.Name(),
		Model:      request.Model,
		Embeddings: embeddings,
		Usage:      nil, // Ollama doesn't provide usage info for embeddings
		Created:    time.Now(),
	}, nil
}

// processEmbeddingsConcurrently handles larger batches with controlled concurrency
func (p *Provider) processEmbeddingsConcurrently(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	type result struct {
		index     int
		embedding types.Embedding
		err       error
	}

	// Limit concurrency to avoid overwhelming local Ollama instance
	const maxConcurrency = 3
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan result, len(request.Input))

	// Start concurrent workers
	for i, input := range request.Input {
		go func(idx int, txt string) {
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			payload := &embeddingsRequest{
				Model:  request.Model,
				Prompt: txt,
			}

			url := p.GetBaseURL() + "/api/embeddings"

			var response embeddingsResponse
			err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)

			if err != nil {
				results <- result{index: idx, err: p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to get embedding for input %d", idx), err)}
			} else {
				results <- result{
					index: idx,
					embedding: types.Embedding{
						Index:     idx,
						Embedding: response.Embedding,
					},
				}
			}
		}(i, input)
	}

	// Collect results
	embeddings := make([]types.Embedding, len(request.Input))
	for i := 0; i < len(request.Input); i++ {
		res := <-results
		if res.err != nil {
			return nil, res.err
		}
		embeddings[res.index] = res.embedding
	}

	return &types.EmbeddingsResponse{
		Provider:   p.Name(),
		Model:      request.Model,
		Embeddings: embeddings,
		Usage:      nil, // Ollama doesn't provide usage info for embeddings
		Created:    time.Now(),
	}, nil
}

package wormhole

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/garyblankenship/wormhole/v2/types"
)

func encodeEmbeddingsResponse(response *types.EmbeddingsResponse, format types.EmbeddingEncodingFormat) *types.EmbeddingsResponse {
	if response == nil || format != types.EmbeddingEncodingBase64 {
		return response
	}
	for i := range response.Embeddings {
		if response.Embeddings[i].Base64 == "" {
			vector := response.Embeddings[i].Embedding
			encoded := make([]byte, len(vector)*4)
			for j, value := range vector {
				binary.LittleEndian.PutUint32(encoded[j*4:], math.Float32bits(float32(value)))
			}
			response.Embeddings[i].Base64 = base64.StdEncoding.EncodeToString(encoded)
		}
		response.Embeddings[i].Embedding = nil
	}
	return response
}

func validEmbeddingEncodingFormat(format types.EmbeddingEncodingFormat) bool {
	return format == "" || format == types.EmbeddingEncodingFloat || format == types.EmbeddingEncodingBase64
}

func (b *EmbeddingsRequestBuilder) executeEmbeddings(ctx context.Context, request *types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	provider, release, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}
	defer release()

	ctx = contextWithProviderOperation(ctx, provider, "embeddings")
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyEmbeddings(provider.Embeddings)
		return handler(ctx, *request)
	}

	return provider.Embeddings(ctx, *request)
}

func placeEmbeddingBatch(out []types.Embedding, start, count int, embeddings []types.Embedding) error {
	if len(embeddings) != count {
		return fmt.Errorf("got %d vectors for %d inputs", len(embeddings), count)
	}
	seen := make([]bool, count)
	for _, embedding := range embeddings {
		if embedding.Index < 0 || embedding.Index >= count {
			return fmt.Errorf("response index %d out of range [0,%d)", embedding.Index, count)
		}
		if seen[embedding.Index] {
			return fmt.Errorf("duplicate response index %d", embedding.Index)
		}
		seen[embedding.Index] = true
		embedding.Index += start
		out[embedding.Index] = embedding
	}
	for i, ok := range seen {
		if !ok {
			return fmt.Errorf("missing response index %d", i)
		}
	}
	return nil
}

func cloneEmbeddingsResponseHeader(src *types.EmbeddingsResponse) *types.EmbeddingsResponse {
	if src == nil {
		return nil
	}
	cloned := &types.EmbeddingsResponse{
		ID:       src.ID,
		Provider: src.Provider,
		Model:    src.Model,
		Created:  src.Created,
	}
	if len(src.Metadata) > 0 {
		cloned.Metadata = make(map[string]any, len(src.Metadata))
		for key, value := range src.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func mergeUsage(current, next *types.Usage) *types.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		return &cloned
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	current.CacheReadTokens += next.CacheReadTokens
	current.CacheWriteTokens += next.CacheWriteTokens
	return current
}

func cloneEmbeddingsRequest(src *types.EmbeddingsRequest) *types.EmbeddingsRequest {
	if src == nil {
		return &types.EmbeddingsRequest{}
	}

	cloned := &types.EmbeddingsRequest{
		Model:          src.Model,
		EncodingFormat: src.EncodingFormat,
	}
	if src.Dimensions != nil {
		dimensions := *src.Dimensions
		cloned.Dimensions = &dimensions
	}
	if len(src.Input) > 0 {
		cloned.Input = make([]string, len(src.Input))
		copy(cloned.Input, src.Input)
	}
	cloned.ProviderOptions = cloneProviderOptions(src.ProviderOptions)
	return cloned
}

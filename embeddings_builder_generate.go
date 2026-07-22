package wormhole

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// Generate executes the request and returns embeddings
func (b *EmbeddingsRequestBuilder) Generate(ctx context.Context) (*types.EmbeddingsResponse, error) {
	if b.request == nil {
		return nil, types.NewValidationError("request", "already_used", nil, "builder already used; create a new builder for each request")
	}
	// CRITICAL: Return request to pool to prevent memory leak
	defer func() {
		putEmbeddingsRequest(b.request)
		b.request = nil
	}()

	request := cloneEmbeddingsRequest(b.request)

	// Validate request
	if len(request.Input) == 0 {
		return nil, types.NewValidationError("input", "required", nil, "no input provided")
	}
	if request.Model == "" {
		return nil, types.NewValidationError("model", "required", nil, "no model specified")
	}
	if !validEmbeddingEncodingFormat(request.EncodingFormat) {
		return nil, types.NewValidationError("encoding_format", "enum", request.EncodingFormat, "must be float or base64")
	}
	if err := b.getWormhole().validateModelAttempt(b.getProvider(), request.Model, nil, []types.ModelCapability{types.CapabilityEmbeddings}); err != nil {
		return nil, err
	}

	response, err := executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("embeddings.generate"), request, func(ctx context.Context) (*types.EmbeddingsResponse, error) {
		return b.executeEmbeddings(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return encodeEmbeddingsResponse(response, request.EncodingFormat), nil
}

// GenerateBatched executes the embeddings request in sub-batches and returns
// embeddings in the same order as the caller's input slice. Each provider
// response must contain exactly one embedding per input and every embedding
// Index must refer to an item in that sub-batch.
func (b *EmbeddingsRequestBuilder) GenerateBatched(ctx context.Context, batchSize int) (*types.EmbeddingsResponse, error) {
	if b.request == nil {
		return nil, types.NewValidationError("request", "already_used", nil, "builder already used; create a new builder for each request")
	}
	// CRITICAL: Return request to pool to prevent memory leak
	defer func() {
		putEmbeddingsRequest(b.request)
		b.request = nil
	}()

	request := cloneEmbeddingsRequest(b.request)
	if len(request.Input) == 0 {
		return nil, types.NewValidationError("input", "required", nil, "no input provided")
	}
	if request.Model == "" {
		return nil, types.NewValidationError("model", "required", nil, "no model specified")
	}
	if batchSize <= 0 {
		return nil, types.NewValidationError("batch_size", "positive", batchSize, "must be a positive integer")
	}
	if !validEmbeddingEncodingFormat(request.EncodingFormat) {
		return nil, types.NewValidationError("encoding_format", "enum", request.EncodingFormat, "must be float or base64")
	}
	if err := b.getWormhole().validateModelAttempt(b.getProvider(), request.Model, nil, []types.ModelCapability{types.CapabilityEmbeddings}); err != nil {
		return nil, err
	}

	response, err := executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("embeddings.generate_batched"), request, func(ctx context.Context) (*types.EmbeddingsResponse, error) {
		out := make([]types.Embedding, len(request.Input))
		var combined *types.EmbeddingsResponse
		var usage *types.Usage

		for start := 0; start < len(request.Input); start += batchSize {
			end := start + batchSize
			if end > len(request.Input) {
				end = len(request.Input)
			}
			batchRequest := cloneEmbeddingsRequestMetadata(request)
			batchRequest.Input = append([]string(nil), request.Input[start:end]...)

			resp, err := b.executeEmbeddings(ctx, batchRequest)
			if err != nil {
				return nil, fmt.Errorf("embeddings batch [%d:%d]: %w", start, end, err)
			}
			if resp == nil {
				return nil, fmt.Errorf("embeddings batch [%d:%d]: provider returned nil response", start, end)
			}
			if combined == nil {
				combined = cloneEmbeddingsResponseHeader(resp)
			}
			usage = mergeUsage(usage, resp.Usage)

			if err := placeEmbeddingBatch(out, start, end-start, resp.Embeddings); err != nil {
				return nil, fmt.Errorf("embeddings batch [%d:%d]: %w", start, end, err)
			}
		}

		if combined == nil {
			combined = &types.EmbeddingsResponse{Model: request.Model, Created: time.Now()}
		}
		combined.Model = request.Model
		combined.Embeddings = out
		combined.Usage = usage
		return combined, nil
	})
	if err != nil {
		return nil, err
	}
	return encodeEmbeddingsResponse(response, request.EncodingFormat), nil
}

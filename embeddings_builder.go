package wormhole

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// EmbeddingsRequestBuilder builds embeddings requests.
//
// Thread Safety: Each builder instance should be used by a single goroutine.
// The client.Embeddings() method creates a new builder instance for each call,
// making concurrent usage safe when each goroutine creates its own builder.
// Do NOT reuse the same builder instance across multiple goroutines.
type EmbeddingsRequestBuilder struct {
	CommonBuilder
	request *types.EmbeddingsRequest
}

// Using sets the provider to use
func (b *EmbeddingsRequestBuilder) Using(provider string) *EmbeddingsRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *EmbeddingsRequestBuilder) BaseURL(url string) *EmbeddingsRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the model to use.
// Returns the builder for chaining. Validation errors are returned by Generate().
func (b *EmbeddingsRequestBuilder) Model(model string) *EmbeddingsRequestBuilder {
	b.request.Model = model
	return b
}

// Input sets the input text(s) to generate embeddings for.
// Returns the builder for chaining. Validation errors are returned by Generate().
func (b *EmbeddingsRequestBuilder) Input(inputs ...string) *EmbeddingsRequestBuilder {
	b.request.Input = inputs
	return b
}

// AddInput adds input text to generate embeddings for.
// Returns the builder for chaining. Validation errors are returned by Generate().
func (b *EmbeddingsRequestBuilder) AddInput(input string) *EmbeddingsRequestBuilder {
	b.request.Input = append(b.request.Input, input)
	return b
}

// Dimensions sets the desired dimensions for the embeddings.
// Returns the builder for chaining. Validation errors are returned by Generate().
func (b *EmbeddingsRequestBuilder) Dimensions(dims int) *EmbeddingsRequestBuilder {
	b.request.Dimensions = &dims
	return b
}

// EncodingFormat controls whether returned embeddings are numeric vectors or
// OpenAI-compatible base64 strings containing little-endian float32 values.
func (b *EmbeddingsRequestBuilder) EncodingFormat(format types.EmbeddingEncodingFormat) *EmbeddingsRequestBuilder {
	b.request.EncodingFormat = format
	return b
}

// ProviderOptions sets provider-specific options
func (b *EmbeddingsRequestBuilder) ProviderOptions(options map[string]any) *EmbeddingsRequestBuilder {
	b.request.ProviderOptions = options
	return b
}

// Clone creates a deep copy of the builder with all settings preserved.
// This allows you to create variations from a base configuration.
//
// Example:
//
//	base := client.Embeddings().Model("text-embedding-3-small").Dimensions(512)
//	resp1, _ := base.Clone().Input("First text").Generate(ctx)
//	resp2, _ := base.Clone().Input("Second text").Generate(ctx)
func (b *EmbeddingsRequestBuilder) Clone() *EmbeddingsRequestBuilder {
	clonedRequest := cloneEmbeddingsRequest(b.request)

	return &EmbeddingsRequestBuilder{
		CommonBuilder: CommonBuilder{
			wormhole: b.wormhole,
			provider: b.provider,
			baseURL:  b.baseURL,
		},
		request: clonedRequest,
	}
}

// Validate checks the request configuration for errors before calling Generate().
// This enables fail-fast behavior to catch configuration issues early.
//
// Validates:
//   - Model is specified
//   - Input is provided
//   - Dimensions is positive if specified
//
// Example:
//
//	builder := client.Embeddings().Model("text-embedding-3-small").Input("text")
//	if err := builder.Validate(); err != nil {
//	    log.Fatal("Invalid configuration:", err)
//	}
func (b *EmbeddingsRequestBuilder) Validate() error {
	var errs types.ValidationErrors

	if b.request.Model == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}

	if len(b.request.Input) == 0 {
		errs.Add("input", "required", nil, "at least one input text must be provided")
	}

	if b.request.Dimensions != nil && *b.request.Dimensions <= 0 {
		errs.Add("dimensions", "positive", *b.request.Dimensions, "must be a positive integer")
	}
	if format := b.request.EncodingFormat; !validEmbeddingEncodingFormat(format) {
		errs.Add("encoding_format", "enum", format, "must be float or base64")
	}

	return errs.Error()
}

// MustValidate calls Validate() and panics if validation fails.
func (b *EmbeddingsRequestBuilder) MustValidate() *EmbeddingsRequestBuilder {
	if err := b.Validate(); err != nil {
		panic(err)
	}
	return b
}

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

	response, err := executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("embeddings.generate_batched"), request, func(ctx context.Context) (*types.EmbeddingsResponse, error) {
		out := make([]types.Embedding, len(request.Input))
		var combined *types.EmbeddingsResponse
		var usage *types.Usage

		for start := 0; start < len(request.Input); start += batchSize {
			end := start + batchSize
			if end > len(request.Input) {
				end = len(request.Input)
			}
			batchRequest := cloneEmbeddingsRequest(request)
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

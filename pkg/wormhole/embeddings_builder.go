package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
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
	clonedRequest := getEmbeddingsRequest()
	clonedRequest.Model = b.request.Model

	// Clone pointer fields
	if b.request.Dimensions != nil {
		dims := *b.request.Dimensions
		clonedRequest.Dimensions = &dims
	}

	// Clone slices
	if len(b.request.Input) > 0 {
		clonedRequest.Input = make([]string, len(b.request.Input))
		copy(clonedRequest.Input, b.request.Input)
	}
	if len(b.request.ProviderOptions) > 0 {
		clonedRequest.ProviderOptions = make(map[string]any)
		for k, v := range b.request.ProviderOptions {
			clonedRequest.ProviderOptions[k] = v
		}
	}

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
	// CRITICAL: Return request to pool to prevent memory leak
	defer putEmbeddingsRequest(b.request)

	provider, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}

	// Validate request
	if len(b.request.Input) == 0 {
		return nil, types.NewValidationError("input", "required", nil, "no input provided")
	}
	if b.request.Model == "" {
		return nil, types.NewValidationError("model", "required", nil, "no model specified")
	}

	// Apply type-safe middleware chain if configured
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyEmbeddings(provider.Embeddings)
		return handler(ctx, *b.request)
	}

	// Fallback to legacy middleware if configured
	if b.getWormhole().middlewareChain != nil {
		handler := b.getWormhole().middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			// Safe type assertion with error handling
			embeddingsReq, ok := req.(*types.EmbeddingsRequest)
			if !ok {
				return nil, fmt.Errorf("invalid request type: expected *EmbeddingsRequest, got %T", req)
			}
			return provider.Embeddings(ctx, *embeddingsReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		// Safe type assertion with error handling
		embeddingsResp, ok := resp.(*types.EmbeddingsResponse)
		if !ok {
			return nil, fmt.Errorf("invalid response type: expected *EmbeddingsResponse, got %T", resp)
		}
		return embeddingsResp, nil
	}

	return provider.Embeddings(ctx, *b.request)
}

package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// EmbeddingsRequestBuilder builds embeddings requests
type EmbeddingsRequestBuilder struct {
	CommonBuilder
	request *types.EmbeddingsRequest
}

// Using sets the provider to use
func (b *EmbeddingsRequestBuilder) Using(provider string) *EmbeddingsRequestBuilder {
	b.setProvider(provider)
	return b
}

// Provider sets the provider to use (alias for Using)
func (b *EmbeddingsRequestBuilder) Provider(provider string) *EmbeddingsRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *EmbeddingsRequestBuilder) BaseURL(url string) *EmbeddingsRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the model to use
func (b *EmbeddingsRequestBuilder) Model(model string) *EmbeddingsRequestBuilder {
	b.request.Model = model
	return b
}

// Input sets the input text(s) to generate embeddings for
func (b *EmbeddingsRequestBuilder) Input(inputs ...string) *EmbeddingsRequestBuilder {
	b.request.Input = inputs
	return b
}

// AddInput adds input text to generate embeddings for
func (b *EmbeddingsRequestBuilder) AddInput(input string) *EmbeddingsRequestBuilder {
	b.request.Input = append(b.request.Input, input)
	return b
}

// Dimensions sets the desired dimensions for the embeddings
func (b *EmbeddingsRequestBuilder) Dimensions(dims int) *EmbeddingsRequestBuilder {
	b.request.Dimensions = &dims
	return b
}

// Generate executes the request and returns embeddings
func (b *EmbeddingsRequestBuilder) Generate(ctx context.Context) (*types.EmbeddingsResponse, error) {
	provider, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}

	// Validate request
	if len(b.request.Input) == 0 {
		return nil, fmt.Errorf("no input provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}

	// Ensure we have an EmbeddingsProvider
	embeddingsProvider, ok := provider.(types.EmbeddingsProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support embeddings", provider.Name())
	}

	// Apply middleware chain if configured
	if b.getWormhole().middlewareChain != nil {
		handler := b.getWormhole().middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			embeddingsReq := req.(*types.EmbeddingsRequest)
			return embeddingsProvider.Embeddings(ctx, *embeddingsReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.EmbeddingsResponse), nil
	}

	return embeddingsProvider.Embeddings(ctx, *b.request)
}

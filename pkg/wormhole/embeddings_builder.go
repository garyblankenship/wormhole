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
	// Early validation for better error detection
	if model == "" {
		panic("embeddings model cannot be empty")
	}
	b.request.Model = model
	return b
}

// Input sets the input text(s) to generate embeddings for
func (b *EmbeddingsRequestBuilder) Input(inputs ...string) *EmbeddingsRequestBuilder {
	// Early validation for better error detection
	if len(inputs) == 0 {
		panic("embeddings input cannot be empty")
	}
	for i, input := range inputs {
		if input == "" {
			panic(fmt.Sprintf("embeddings input[%d] cannot be empty string", i))
		}
	}
	b.request.Input = inputs
	return b
}

// AddInput adds input text to generate embeddings for
func (b *EmbeddingsRequestBuilder) AddInput(input string) *EmbeddingsRequestBuilder {
	// Early validation for better error detection
	if input == "" {
		panic("embeddings input cannot be empty string")
	}
	b.request.Input = append(b.request.Input, input)
	return b
}

// Dimensions sets the desired dimensions for the embeddings
func (b *EmbeddingsRequestBuilder) Dimensions(dims int) *EmbeddingsRequestBuilder {
	// Early validation for better error detection
	if dims <= 0 {
		panic("embeddings dimensions must be positive")
	}
	if dims > 10000 {
		panic("embeddings dimensions too large (>10000)")
	}
	b.request.Dimensions = &dims
	return b
}

// ProviderOptions sets provider-specific options
func (b *EmbeddingsRequestBuilder) ProviderOptions(options map[string]any) *EmbeddingsRequestBuilder {
	b.request.ProviderOptions = options
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
		return nil, fmt.Errorf("no input provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
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

package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ImageRequestBuilder builds image generation requests
type ImageRequestBuilder struct {
	CommonBuilder
	request *types.ImageRequest
}

// Using sets the provider to use
func (b *ImageRequestBuilder) Using(provider string) *ImageRequestBuilder {
	b.setProvider(provider)
	return b
}

// Provider sets the provider to use (alias for Using)
func (b *ImageRequestBuilder) Provider(provider string) *ImageRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *ImageRequestBuilder) BaseURL(url string) *ImageRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the model to use
func (b *ImageRequestBuilder) Model(model string) *ImageRequestBuilder {
	b.request.Model = model
	return b
}

// Prompt sets the prompt for image generation
func (b *ImageRequestBuilder) Prompt(prompt string) *ImageRequestBuilder {
	b.request.Prompt = prompt
	return b
}

// Size sets the size of the generated image
func (b *ImageRequestBuilder) Size(size string) *ImageRequestBuilder {
	b.request.Size = size
	return b
}

// Quality sets the quality of the generated image
func (b *ImageRequestBuilder) Quality(quality string) *ImageRequestBuilder {
	b.request.Quality = quality
	return b
}

// Style sets the style of the generated image
func (b *ImageRequestBuilder) Style(style string) *ImageRequestBuilder {
	b.request.Style = style
	return b
}

// N sets the number of images to generate
func (b *ImageRequestBuilder) N(n int) *ImageRequestBuilder {
	b.request.N = n
	return b
}

// ResponseFormat sets the response format (url or b64_json)
func (b *ImageRequestBuilder) ResponseFormat(format string) *ImageRequestBuilder {
	b.request.ResponseFormat = format
	return b
}

// Generate executes the request and returns generated images
func (b *ImageRequestBuilder) Generate(ctx context.Context) (*types.ImageResponse, error) {
	provider, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}

	// Validate request
	if b.request.Prompt == "" {
		return nil, fmt.Errorf("no prompt provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}

	// Set defaults
	if b.request.N == 0 {
		b.request.N = 1
	}

	// Apply type-safe middleware chain if configured
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyImage(provider.GenerateImage)
		return handler(ctx, *b.request)
	}

	// Fallback to legacy middleware if configured
	if b.getWormhole().middlewareChain != nil {
		handler := b.getWormhole().middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			imageReq := req.(*types.ImageRequest)
			return provider.GenerateImage(ctx, *imageReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.ImageResponse), nil
	}

	return provider.GenerateImage(ctx, *b.request)
}

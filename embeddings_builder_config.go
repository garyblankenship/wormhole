package wormhole

import (
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

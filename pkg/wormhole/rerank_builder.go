package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// RerankRequestBuilder builds rerank requests.
//
// Thread Safety: Each builder instance should be used by a single goroutine.
// The client.Rerank() method creates a new builder instance for each call,
// making concurrent usage safe when each goroutine creates its own builder.
type RerankRequestBuilder struct {
	CommonBuilder
	request *types.RerankRequest
}

// Using sets the provider to use.
func (b *RerankRequestBuilder) Using(provider string) *RerankRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs.
func (b *RerankRequestBuilder) BaseURL(url string) *RerankRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the rerank model to use.
func (b *RerankRequestBuilder) Model(model string) *RerankRequestBuilder {
	b.request.Model = model
	return b
}

// Query sets the search query to rank documents against.
func (b *RerankRequestBuilder) Query(query string) *RerankRequestBuilder {
	b.request.Query = query
	return b
}

// Documents sets the documents to rerank.
func (b *RerankRequestBuilder) Documents(documents ...string) *RerankRequestBuilder {
	b.request.Documents = documents
	return b
}

// AddDocument appends a document to rerank.
func (b *RerankRequestBuilder) AddDocument(document string) *RerankRequestBuilder {
	b.request.Documents = append(b.request.Documents, document)
	return b
}

// TopN limits the response to the N most relevant documents.
func (b *RerankRequestBuilder) TopN(n int) *RerankRequestBuilder {
	b.request.TopN = &n
	return b
}

// ProviderOptions sets provider-specific options.
func (b *RerankRequestBuilder) ProviderOptions(options map[string]any) *RerankRequestBuilder {
	b.request.ProviderOptions = options
	return b
}

// Validate checks the request configuration for errors before calling Generate().
func (b *RerankRequestBuilder) Validate() error {
	var errs types.ValidationErrors

	if b.request.Model == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}
	if b.request.Query == "" {
		errs.Add("query", "required", nil, "query must be specified")
	}
	if len(b.request.Documents) == 0 {
		errs.Add("documents", "required", nil, "at least one document must be provided")
	}

	return errs.Error()
}

// Generate executes the request and returns reranked results. Routes through
// the provider middleware chain and in-flight request tracking (trackRequest/
// Shutdown, idempotency) exactly like Text/Embeddings, so Shutdown can no
// longer tear down connections out from under an in-flight rerank call.
func (b *RerankRequestBuilder) Generate(ctx context.Context) (*types.RerankResponse, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	request := b.request
	return executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("rerank.generate"), request, func(ctx context.Context) (*types.RerankResponse, error) {
		return b.executeRerank(ctx, request)
	})
}

// executeRerank resolves the provider and routes the call through the
// middleware chain, mirroring EmbeddingsRequestBuilder.executeEmbeddings.
func (b *RerankRequestBuilder) executeRerank(ctx context.Context, request *types.RerankRequest) (*types.RerankResponse, error) {
	provider, release, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}
	defer release()

	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyRerank(provider.Rerank)
		return handler(ctx, *request)
	}

	return provider.Rerank(ctx, *request)
}

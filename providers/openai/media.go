package openai

import (
	"context"
	"net/http"

	"github.com/garyblankenship/wormhole/v3/types"
)

// Embeddings generates embeddings
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	payload := map[string]any{
		"model": request.Model,
		"input": request.Input,
	}

	if request.Dimensions != nil {
		payload["dimensions"] = *request.Dimensions
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.GetBaseURL() + "/embeddings"

	var response embeddingsResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformEmbeddingsResponse(&response, request.Model)
	resp.ResponseMetadata = metadata
	resp.Provider = p.Name()
	return resp, nil
}

// Rerank reranks documents by relevance to a query (OpenAI-compatible /rerank).
func (p *Provider) Rerank(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
	payload := map[string]any{
		"model":     request.Model,
		"query":     request.Query,
		"documents": request.Documents,
	}

	if request.TopN != nil {
		payload["top_n"] = *request.TopN
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.GetBaseURL() + "/rerank"

	var response rerankResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformRerankResponse(&response, request.Model)
	resp.ResponseMetadata = metadata
	resp.Provider = p.Name()
	return resp, nil
}

// Images generates images
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}

	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if request.N > 0 {
		payload["n"] = request.N
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.imagesURL()

	var response imageResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	result := p.transformImageResponse(&response)
	result.ResponseMetadata = metadata
	return result, nil
}

// GenerateImage generates images through the unified image-generation interface.
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return p.Images(ctx, request)
}

// Temporarily disabled until request types are defined
// These methods will be automatically provided by embedded BaseProvider with NotImplementedError

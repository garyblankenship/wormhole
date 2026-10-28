package fetchers

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

// OpenAICompatibleFetcher fetches models from providers that expose the
// OpenAI-compatible GET /models shape.
type OpenAICompatibleFetcher struct {
	name    string
	baseURL string
	apiKey  string
	headers map[string]string
}

// NewOpenAICompatibleFetcher creates a model fetcher for an OpenAI-compatible provider.
func NewOpenAICompatibleFetcher(name, baseURL, apiKey string, headers map[string]string) discovery.ModelFetcher {
	return &OpenAICompatibleFetcher{
		name:    name,
		baseURL: normalizedEndpoint(baseURL),
		apiKey:  apiKey,
		headers: canonicalHeaders(headers),
	}
}

// Name returns the configured provider name.
func (f *OpenAICompatibleFetcher) Name() string {
	return f.name
}

// AccountDiscriminator hashes the endpoint and effective authentication/header
// scope so distinct catalogs cannot share a cache identity.
func (f *OpenAICompatibleFetcher) AccountDiscriminator() string {
	effectiveHeaders := map[string]string{}
	if f.apiKey != "" {
		effectiveHeaders["Authorization"] = "Bearer " + f.apiKey
	}
	for key, value := range f.headers {
		effectiveHeaders[key] = value
	}
	return configuredAccountDiscriminator(f.baseURL, false, "", effectiveHeaders)
}

// FetchModels retrieves all available models from an OpenAI-compatible provider.
func (f *OpenAICompatibleFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if f.baseURL == "" {
		return nil, fmt.Errorf("%s base URL not configured", f.name)
	}

	req, err := newGetRequest(ctx, f.baseURL+"/models")
	if err != nil {
		return nil, err
	}
	if f.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+f.apiKey)
	}
	applyHeaders(req, f.headers)

	return fetchOpenAICompatibleModels(req, f.name, false)
}

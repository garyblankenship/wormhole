package fetchers

import (
	"context"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/discovery"
	"github.com/garyblankenship/wormhole/pkg/types"
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
	copiedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		copiedHeaders[key] = value
	}
	return &OpenAICompatibleFetcher{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		headers: copiedHeaders,
	}
}

// Name returns the configured provider name.
func (f *OpenAICompatibleFetcher) Name() string {
	return f.name
}

// AccountDiscriminator scopes the model cache per API key so different
// accounts on the same OpenAI-compatible endpoint don't collide on the
// same cache file.
func (f *OpenAICompatibleFetcher) AccountDiscriminator() string {
	return accountKeyDiscriminator(f.apiKey)
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
	for key, value := range f.headers {
		req.Header.Set(key, value)
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := fetchJSON(req, &response); err != nil {
		return nil, err
	}

	models := make([]*types.ModelInfo, 0, len(response.Data))
	for _, model := range response.Data {
		models = append(models, &types.ModelInfo{
			ID:           model.ID,
			Name:         formatModelName(model.ID),
			Provider:     f.name,
			Capabilities: inferOpenAICapabilities(model.ID),
		})
	}
	return models, nil
}

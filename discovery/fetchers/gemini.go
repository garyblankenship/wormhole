package fetchers

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GeminiFetcher fetches models from Google's Gemini API.
type GeminiFetcher struct {
	apiKey  string
	baseURL string
}

// NewGeminiFetcher creates a Gemini model fetcher.
func NewGeminiFetcher(baseURL, apiKey string) *GeminiFetcher {
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	return &GeminiFetcher{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// Name returns the provider name.
func (f *GeminiFetcher) Name() string {
	return "gemini"
}

// AccountDiscriminator scopes the model cache per API key so different
// Gemini accounts don't collide on the same cache file.
func (f *GeminiFetcher) AccountDiscriminator() string {
	return accountKeyDiscriminator(f.apiKey)
}

// FetchModels retrieves all available Gemini models.
func (f *GeminiFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("gemini API key not configured")
	}

	req, err := newGetRequest(ctx, f.baseURL+"/models?key="+url.QueryEscape(f.apiKey))
	if err != nil {
		return nil, err
	}

	var response struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := fetchJSON(req, &response); err != nil {
		return nil, err
	}

	models := make([]*types.ModelInfo, 0, len(response.Models))
	for _, model := range response.Models {
		id := strings.TrimPrefix(model.Name, "models/")
		name := model.DisplayName
		if name == "" {
			name = formatModelName(id)
		}
		models = append(models, &types.ModelInfo{
			ID:           id,
			Name:         name,
			Provider:     "gemini",
			Capabilities: inferGeminiCapabilities(model.SupportedGenerationMethods),
			MaxTokens:    model.InputTokenLimit,
		})
	}

	return models, nil
}

func inferGeminiCapabilities(methods []string) []types.ModelCapability {
	capabilities := []types.ModelCapability{}
	for _, method := range methods {
		switch method {
		case "generateContent":
			capabilities = append(capabilities,
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityStream,
				types.CapabilityFunctions,
				types.CapabilityStructured,
				types.CapabilityVision,
			)
		case "embedContent", "batchEmbedContents":
			capabilities = append(capabilities, types.CapabilityEmbeddings)
		}
	}
	if len(capabilities) == 0 {
		return []types.ModelCapability{types.CapabilityText}
	}
	return capabilities
}

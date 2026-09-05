package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// OpenAIFetcher fetches models from OpenAI API
type OpenAIFetcher struct {
	apiKey  string
	baseURL string
}

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type openAIModel struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created json.RawMessage `json:"created"`
	OwnedBy string          `json:"owned_by"`
}

func fetchOpenAICompatibleModels(req *http.Request, provider string, includeMetadata bool) ([]*types.ModelInfo, error) {
	var response openAIModelsResponse
	if err := fetchJSON(req, &response); err != nil {
		return nil, err
	}

	models := make([]*types.ModelInfo, 0, len(response.Data))
	for _, model := range response.Data {
		info := &types.ModelInfo{
			ID:           model.ID,
			Name:         formatModelName(model.ID),
			Provider:     provider,
			Capabilities: inferOpenAICapabilities(model.ID),
		}
		if includeMetadata {
			if len(model.Created) > 0 {
				if err := json.Unmarshal(model.Created, &info.Created); err != nil {
					return nil, err
				}
			}
			info.OwnedBy = model.OwnedBy
		}
		models = append(models, info)
	}
	return models, nil
}

// NewOpenAIFetcher creates a new OpenAI model fetcher
func NewOpenAIFetcher(apiKey string) *OpenAIFetcher {
	return &OpenAIFetcher{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1",
	}
}

// Name returns the provider name
func (f *OpenAIFetcher) Name() string {
	return "openai"
}

// AccountDiscriminator scopes the model cache per API key so different
// OpenAI accounts don't collide on the same cache file.
func (f *OpenAIFetcher) AccountDiscriminator() string {
	return accountKeyDiscriminator(f.apiKey)
}

// FetchModels retrieves all available models from OpenAI
func (f *OpenAIFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	req, err := newGetRequest(ctx, f.baseURL+"/models")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	return fetchOpenAICompatibleModels(req, f.Name(), true)
}

// inferOpenAICapabilities determines capabilities from model ID
func inferOpenAICapabilities(modelID string) []types.ModelCapability {
	switch {
	case strings.HasPrefix(modelID, "text-embedding-"):
		return []types.ModelCapability{types.CapabilityEmbeddings}
	case strings.HasPrefix(modelID, "dall-e-"), strings.HasPrefix(modelID, "gpt-image-"), strings.HasPrefix(modelID, "sora-"):
		return []types.ModelCapability{types.CapabilityImages}
	case strings.HasPrefix(modelID, "whisper-"), strings.HasPrefix(modelID, "tts-"), strings.HasPrefix(modelID, "gpt-audio"), strings.HasPrefix(modelID, "gpt-realtime"):
		return []types.ModelCapability{types.CapabilityAudio}
	case strings.HasPrefix(modelID, "gpt-"), strings.HasPrefix(modelID, "o1"), strings.HasPrefix(modelID, "o3"), strings.HasPrefix(modelID, "o4"):
		return []types.ModelCapability{
			types.CapabilityText,
			types.CapabilityChat,
			types.CapabilityStream,
		}
	default:
		return []types.ModelCapability{types.CapabilityText}
	}
}

// formatModelName creates a human-readable name from model ID
func formatModelName(modelID string) string {
	// Simple formatting: "gpt-5" -> "GPT-5"
	parts := strings.Split(modelID, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

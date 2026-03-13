package fetchers

import (
	"context"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// OpenAIFetcher fetches models from OpenAI API
type OpenAIFetcher struct {
	apiKey  string
	baseURL string
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

	var response struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := fetchJSON(ctx, req, &response); err != nil {
		return nil, err
	}

	// Convert to ModelInfo
	models := make([]*types.ModelInfo, 0, len(response.Data))
	for _, m := range response.Data {
		models = append(models, &types.ModelInfo{
			ID:           m.ID,
			Name:         formatModelName(m.ID),
			Provider:     "openai",
			Capabilities: inferOpenAICapabilities(m.ID),
		})
	}

	return models, nil
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

package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// OpenAIFetcher fetches models from OpenAI API
type OpenAIFetcher struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIFetcher creates a new OpenAI model fetcher
func NewOpenAIFetcher(apiKey string) *OpenAIFetcher {
	return &OpenAIFetcher{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1",
		client:  &http.Client{},
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

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth header
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	// Execute request
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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

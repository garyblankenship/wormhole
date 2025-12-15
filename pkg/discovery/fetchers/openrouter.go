package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Provider name constant
const providerOpenRouter = "openrouter"

// OpenRouterFetcher fetches models from OpenRouter API
type OpenRouterFetcher struct {
	baseURL string
	client  *http.Client
}

// NewOpenRouterFetcher creates a new OpenRouter model fetcher
func NewOpenRouterFetcher() *OpenRouterFetcher {
	return &OpenRouterFetcher{
		baseURL: "https://openrouter.ai/api/v1",
		client:  &http.Client{},
	}
}

// Name returns the provider name
func (f *OpenRouterFetcher) Name() string {
	return providerOpenRouter
}

// FetchModels retrieves all available models from OpenRouter
func (f *OpenRouterFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	// Create request (no auth required for model listing)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Created       int64  `json:"created"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Architecture struct {
				Modality  string `json:"modality"`
				Tokenizer string `json:"tokenizer"`
			} `json:"architecture"`
			Moderation struct {
				Illicit bool `json:"illicit"`
			} `json:"moderation"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to ModelInfo
	models := make([]*types.ModelInfo, 0, len(response.Data))
	for _, m := range response.Data {
		// Filter out models with illicit moderation flags
		if m.Moderation.Illicit {
			continue
		}

		// Infer capabilities from modality
		capabilities := inferCapabilitiesFromModality(m.Architecture.Modality)

		// Extract provider from model ID (e.g., "openai/gpt-5" -> "openai")
		provider := extractProvider(m.ID)

		models = append(models, &types.ModelInfo{
			ID:           m.ID,
			Name:         m.Name,
			Provider:     provider,
			Capabilities: capabilities,
			MaxTokens:    m.ContextLength,
		})
	}

	return models, nil
}

// inferCapabilitiesFromModality determines capabilities from OpenRouter modality string
func inferCapabilitiesFromModality(modality string) []types.ModelCapability {
	capabilities := []types.ModelCapability{}

	// Modality examples:
	// - "text->text" (text generation)
	// - "text+image->text" (vision models)
	// - "text->embedding" (embedding models)
	// - "text->audio" (text-to-speech)
	// - "audio->text" (speech-to-text)

	if strings.Contains(modality, "text->text") || strings.Contains(modality, "->text") {
		capabilities = append(capabilities, types.CapabilityText, types.CapabilityChat)
		// Most text models support functions and structured output
		capabilities = append(capabilities, types.CapabilityFunctions, types.CapabilityStructured)
	}

	if strings.Contains(modality, "image") {
		if strings.Contains(modality, "text+image->") {
			// Vision model (input)
			capabilities = append(capabilities, types.CapabilityVision)
		} else if strings.Contains(modality, "->image") {
			// Image generation (output)
			capabilities = append(capabilities, types.CapabilityImages)
		}
	}

	if strings.Contains(modality, "embedding") {
		capabilities = []types.ModelCapability{types.CapabilityEmbeddings}
	}

	if strings.Contains(modality, "audio") {
		capabilities = append(capabilities, types.CapabilityAudio)
	}

	// Fallback
	if len(capabilities) == 0 {
		capabilities = []types.ModelCapability{types.CapabilityText}
	}

	return capabilities
}

// extractProvider extracts provider name from model ID
func extractProvider(modelID string) string {
	// OpenRouter model IDs are formatted as "provider/model-name"
	// e.g., "openai/gpt-5", "anthropic/claude-sonnet-4-5", "google/gemini-pro"
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return providerOpenRouter // Fallback
}

package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// AnthropicFetcher fetches models from Anthropic API
type AnthropicFetcher struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropicFetcher creates a new Anthropic model fetcher
func NewAnthropicFetcher(apiKey string) *AnthropicFetcher {
	return &AnthropicFetcher{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com/v1",
		client:  &http.Client{},
	}
}

// Name returns the provider name
func (f *AnthropicFetcher) Name() string {
	return "anthropic"
}

// FetchModels retrieves all available models from Anthropic
func (f *AnthropicFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("anthropic API key not configured")
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add required headers
	req.Header.Set("x-api-key", f.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
			Type        string `json:"type"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to ModelInfo
	models := make([]*types.ModelInfo, 0, len(response.Data))
	for _, m := range response.Data {
		// All Claude models have the same capabilities
		capabilities := []types.ModelCapability{
			types.CapabilityText,
			types.CapabilityChat,
			types.CapabilityFunctions,
			types.CapabilityStructured,
			types.CapabilityVision,
		}

		name := m.DisplayName
		if name == "" {
			name = formatClaudeName(m.ID)
		}

		models = append(models, &types.ModelInfo{
			ID:           m.ID,
			Name:         name,
			Provider:     "anthropic",
			Capabilities: capabilities,
			MaxTokens:    200000, // All Claude models have 200k context
		})
	}

	return models, nil
}

// formatClaudeName creates a human-readable name from model ID
func formatClaudeName(modelID string) string {
	// Simple formatting: "claude-sonnet-4-5" -> "Claude Sonnet 4.5"
	switch modelID {
	case "claude-sonnet-4-5":
		return "Claude Sonnet 4.5"
	case "claude-haiku-4-5":
		return "Claude Haiku 4.5"
	case "claude-opus-4":
		return "Claude Opus 4"
	default:
		// Generic formatting
		return "Claude " + modelID
	}
}

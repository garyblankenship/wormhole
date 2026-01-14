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

// OllamaFetcher fetches locally available models from Ollama
type OllamaFetcher struct {
	baseURL string
	client  *http.Client
}

// NewOllamaFetcher creates a new Ollama model fetcher
func NewOllamaFetcher(baseURL string) *OllamaFetcher {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaFetcher{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name
func (f *OllamaFetcher) Name() string {
	return "ollama"
}

// FetchModels retrieves all locally available models from Ollama
func (f *OllamaFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	// Create request (no auth required for local service)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models (is Ollama running?): %w", err)
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
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
			Digest     string `json:"digest"`
			Details    struct {
				Format        string `json:"format"`
				Family        string `json:"family"`
				ParameterSize string `json:"parameter_size"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to ModelInfo
	models := make([]*types.ModelInfo, 0, len(response.Models))
	for _, m := range response.Models {
		// Most Ollama models are chat models
		capabilities := []types.ModelCapability{
			types.CapabilityText,
			types.CapabilityChat,
		}

		// Some models support vision (llava, bakllava, etc.)
		if isVisionModel(m.Name) {
			capabilities = append(capabilities, types.CapabilityVision)
		}

		// Some models are embedding models
		if isEmbeddingModel(m.Name) {
			capabilities = []types.ModelCapability{types.CapabilityEmbeddings}
		}

		// Infer max tokens from parameter size or use default
		maxTokens := inferMaxTokensFromSize(m.Details.ParameterSize)

		// Format name (remove :tag if present)
		displayName := formatOllamaName(m.Name)

		models = append(models, &types.ModelInfo{
			ID:           m.Name,
			Name:         displayName,
			Provider:     "ollama",
			Capabilities: capabilities,
			MaxTokens:    maxTokens,
		})
	}

	return models, nil
}

// isVisionModel checks if model supports vision based on name
func isVisionModel(modelName string) bool {
	visionModels := []string{"llava", "bakllava", "vision"}
	lowerName := strings.ToLower(modelName)
	for _, vm := range visionModels {
		if strings.Contains(lowerName, vm) {
			return true
		}
	}
	return false
}

// isEmbeddingModel checks if model is an embedding model
func isEmbeddingModel(modelName string) bool {
	embeddingModels := []string{"nomic-embed", "mxbai-embed", "all-minilm", "embedding"}
	lowerName := strings.ToLower(modelName)
	for _, em := range embeddingModels {
		if strings.Contains(lowerName, em) {
			return true
		}
	}
	return false
}

// inferMaxTokensFromSize estimates context length from parameter size
func inferMaxTokensFromSize(paramSize string) int {
	// Parameter size examples: "7B", "13B", "70B"
	switch {
	case strings.Contains(paramSize, "70B"), strings.Contains(paramSize, "65B"):
		return 32768 // Larger models typically have larger context
	case strings.Contains(paramSize, "13B"), strings.Contains(paramSize, "34B"):
		return 16384
	case strings.Contains(paramSize, "7B"):
		return 8192
	default:
		return 4096 // Conservative default
	}
}

// formatOllamaName creates display name from model ID
func formatOllamaName(modelID string) string {
	// Remove tag suffix (e.g., "llama2:latest" -> "llama2")
	name := strings.Split(modelID, ":")[0]

	// Capitalize first letter
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}

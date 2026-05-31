package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Embeddings generates embeddings using Gemini models
func (g *Gemini) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	// More flexible model validation - check for known embedding models or "embedding" in name
	isEmbeddingModel := strings.Contains(request.Model, "embedding") ||
		request.Model == "models/gemini-embedding-001" ||
		request.Model == "gemini-embedding-001" ||
		request.Model == "models/embedding-001" ||
		request.Model == "embedding-001" ||
		strings.HasSuffix(request.Model, ":embedding")

	if !isEmbeddingModel {
		return nil, g.ModelErrorf("model '%s' does not appear to be an embedding model", request.Model)
	}

	payload := g.buildEmbeddingsPayload(request)
	modelName := normalizeModelResource(request.Model)

	endpoint := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s",
		g.GetBaseURL(),
		modelName,
		g.apiKey,
	)

	var response geminiEmbeddingsResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	resp, err := g.transformEmbeddingsResponse(&response)
	if err != nil {
		return nil, err
	}
	resp.Provider = g.Name()
	return resp, nil
}

// buildEmbeddingsPayload builds the request payload for embeddings
func (g *Gemini) buildEmbeddingsPayload(request types.EmbeddingsRequest) map[string]any {
	requests := make([]map[string]any, len(request.Input))
	modelName := "models/" + normalizeModelResource(request.Model)

	for i, input := range request.Input {
		requests[i] = map[string]any{
			"model": modelName,
			"content": map[string]any{
				"parts": []map[string]any{
					{"text": input},
				},
			},
		}

		// Add task type if specified
		if request.ProviderOptions != nil {
			if taskType, ok := request.ProviderOptions["taskType"].(string); ok {
				requests[i]["taskType"] = taskType
			}
			if title, ok := request.ProviderOptions["title"].(string); ok {
				requests[i]["title"] = title
			}
		}
	}

	return map[string]any{
		"requests": requests,
	}
}

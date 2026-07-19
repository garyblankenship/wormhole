package openai

import (
	"encoding/json"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// transformTools converts internal tools to OpenAI format
func (p *Provider) transformTools(tools []types.Tool) []map[string]any {
	// Use shared RequestBuilder for common tool transformation
	baseTools := p.requestBuilder.TransformTools(tools)

	// Adapt to OpenAI-specific format (json.RawMessage for parameters)
	for _, baseTool := range baseTools {
		if toolFunc, ok := baseTool["function"].(map[string]any); ok {
			if params, ok := toolFunc["parameters"].(map[string]any); ok {
				// Convert map to json.RawMessage
				parameters, _ := json.Marshal(params)
				toolFunc["parameters"] = json.RawMessage(parameters)
			}
		}
		// Ensure type field is present (OpenAI requires "type": "function")
		if _, hasType := baseTool["type"]; !hasType {
			baseTool["type"] = "function"
		}
	}

	return baseTools
}

// cleanJSONResponse removes markdown code blocks from JSON responses
func cleanJSONResponse(content string) string {
	return extractJSONFromMarkdown(content)
}

// transformTextResponse converts OpenAI response to internal format
func (p *Provider) transformTextResponse(response *chatCompletionResponse) *types.TextResponse {
	if len(response.Choices) == 0 {
		return &types.TextResponse{
			ID:      response.ID,
			Model:   response.Model,
			Created: time.Unix(response.Created, 0),
		}
	}

	choice := response.Choices[0]
	content := choice.Message.Content

	// Strip markdown code fences from JSON responses regardless of model.
	// cleanJSONResponse is a no-op when there are no backticks and only
	// strips when the extracted content is valid-looking JSON, so this is
	// safe for every provider/model and avoids brittle model-name sniffing.
	content = cleanJSONResponse(content)

	resp := &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         content,
		Refusal:      choice.Message.Refusal,
		ToolCalls:    p.convertToolCalls(choice.Message.ToolCalls),
		FinishReason: p.mapFinishReason(choice.FinishReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Unix(response.Created, 0),
	}

	if choice.Message.ReasoningContent != "" {
		resp.Thinking = &types.Thinking{Content: choice.Message.ReasoningContent}
	}

	return resp
}

// transformEmbeddingsResponse converts OpenAI embeddings response
func (p *Provider) transformEmbeddingsResponse(response *embeddingsResponse, requestModel string) *types.EmbeddingsResponse {
	embeddings := make([]types.Embedding, len(response.Data))

	for i, data := range response.Data {
		// Convert []float32 to []float64
		embedding := make([]float64, len(data.Embedding))
		for j, v := range data.Embedding {
			embedding[j] = float64(v)
		}
		embeddings[i] = types.Embedding{
			Index:     data.Index,
			Embedding: embedding,
		}
	}

	model := response.Model
	if model == "" {
		model = requestModel
	}

	return &types.EmbeddingsResponse{
		Model:      model,
		Embeddings: embeddings,
		Usage:      p.convertUsage(response.Usage),
		Created:    time.Now(),
	}
}

// transformRerankResponse converts an OpenAI-compatible rerank response.
func (p *Provider) transformRerankResponse(response *rerankResponse, requestModel string) *types.RerankResponse {
	results := make([]types.RerankResult, len(response.Results))
	for i, r := range response.Results {
		results[i] = types.RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
			Document:       r.Document.Text,
		}
	}

	model := response.Model
	if model == "" {
		model = requestModel
	}

	return &types.RerankResponse{
		ID:      response.ID,
		Model:   model,
		Results: results,
		Usage:   &types.Usage{TotalTokens: response.Usage.TotalTokens},
		Created: time.Now(),
	}
}

// transformImageResponse converts OpenAI image response
func (p *Provider) transformImageResponse(response *imageResponse) *types.ImagesResponse {
	images := make([]types.GeneratedImage, len(response.Data))

	for i, data := range response.Data {
		images[i] = types.GeneratedImage{
			URL:     data.URL,
			B64JSON: data.B64JSON,
		}
	}

	return &types.ImagesResponse{
		Images:  images,
		Created: time.Unix(response.Created, 0),
	}
}
